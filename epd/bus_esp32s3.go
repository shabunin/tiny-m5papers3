//go:build esp32s3

package epd

import (
	"device"
	"device/esp"
	"errors"
	"machine"
	"runtime"
	"runtime/interrupt"
	"runtime/volatile"
	"unsafe"

	"github.com/shabunin/tiny-m5papers3/psram"
)

// M5PaperS3 wiring (M5GFX board data, "papers3").
var dataPins = [8]machine.Pin{
	machine.GPIO6, machine.GPIO14, machine.GPIO7, machine.GPIO12,
	machine.GPIO9, machine.GPIO11, machine.GPIO8, machine.GPIO10,
}

const (
	pinPWR = machine.GPIO46 // panel high-voltage supply enable
	pinSPV = machine.GPIO17 // gate driver start pulse
	pinCKV = machine.GPIO18 // gate driver clock
	pinSPH = machine.GPIO13 // source driver start pulse, active low during a row
	pinOE  = machine.GPIO45 // source driver output enable
	pinLE  = machine.GPIO15 // source driver latch enable
	pinCL  = machine.GPIO16 // source driver clock = LCD_CAM PCLK
)

// GPIO matrix signals and GDMA peripheral number (ESP-IDF gpio_sig_map.h,
// soc/gdma_channel.h).
const (
	sigLCDCS    = 132 // LCD_CS_IDX: low while a transfer runs
	sigLCDData0 = 133 // LCD_DATA_OUT0_IDX .. LCD_DATA_OUT7_IDX = 140
	sigLCDPCLK  = 154 // LCD_PCLK_IDX
	sigGPIO     = 256 // plain GPIO output
	gdmaPeriLCD = 5   // SOC_GDMA_TRIG_PERIPH_LCD0
)

// LCD_CAM register bits (ESP32-S3 TRM / ESP-IDF lcd_ll.h).
const (
	lcdClkEn       = 1 << 31
	lcdClkSelPos   = 29 // 3 = PLL_F160M
	lcdDivNumPos   = 9  // LCD_CLK = src / div_num
	lcdClkcntNPos  = 0  // PCLK = LCD_CLK / (clkcnt_n + 1)
	lcdAlwaysOut   = 1 << 13
	lcd2ByteEn     = 1 << 23
	lcdUpdate      = 1 << 20
	lcdDout        = 1 << 24
	lcdStart       = 1 << 27
	lcdReset       = 1 << 28
	lcdBkEn        = 1 << 26
	lcdAFIFOReset  = 1 << 27
	lcdVfkMask     = 0x3f << 6
	lcdVbkMask     = 0x1fff << 12
	lcdNextFrameEn = 1 << 25
	lcdRGBModeEn   = 1 << 31
	lcdTransDone   = 1 << 1
)

// GDMA bits (same as TinyGo's machine_esp32xx_i2s.go).
const (
	gdmaConf0Rst      = 1 << 0
	gdmaOutConf0Burst = 1<<4 | 1<<5
	gdmaLinkAddrMask  = 0xfffff
	gdmaOutLinkStop   = 1 << 20
	gdmaOutLinkStart  = 1 << 21

	descOwnerDMA = 1 << 31
	descSucEOF   = 1 << 30
	descLenPos   = 12
)

// dmaDesc is a GDMA link descriptor (ESP-IDF hal/dma_types.h).
type dmaDesc struct {
	dw0  uint32
	buf  uint32
	next uint32
}

var (
	errTimeout = errors.New("epd: LCD transfer timeout")
)

// Timing (microseconds), mirroring M5GFX Bus_EPD.
var (
	// RowGapUs is how long CKV stays low (and LE high) between two rows.
	// In M5GFX this gap is the time its CPU needs to prepare the next row.
	RowGapUs uint32 = 3
	// FrameGapUs is the shortest pause after the last row of a frame (M5GFX:
	// one FreeRTOS tick). Other goroutines run in it.
	FrameGapUs uint32 = 1000
)

type bus struct {
	lines       [2][lineWords]uint32
	desc        [2]dmaDesc
	cyclesPerUs uint32
	userVal     uint32
	powered     bool

	// DryRun keeps the panel supply off: all signals are generated but the
	// panel is never driven. Useful for testing the bus timing.
	DryRun bool

	// LateRows counts rows (since the last ResetStats) whose data was not
	// ready when the previous row finished shifting out. These rows were
	// driven longer than M5GFX would; it should stay 0.
	LateRows int
	// FillMaxUs is the longest time spent preparing one row.
	FillMaxUs uint32
}

// ResetStats clears LateRows and FillMaxUs.
func (b *bus) ResetStats() { b.LateRows, b.FillMaxUs = 0, 0 }

func ccount() uint32 {
	return uint32(device.AsmFull("rsr {}, ccount", nil))
}

func (b *bus) delayUs(us uint32) {
	start := ccount()
	n := us * b.cyclesPerUs
	for ccount()-start < n {
	}
}

// Micros returns a free-running microsecond counter. It wraps every ~17.9 s
// (2^32 CPU cycles), so differences are only meaningful over short spans;
// use package time for longer ones.
func (b *bus) Micros() uint32 {
	return ccount() / b.cyclesPerUs
}

func gpioPinReg(p machine.Pin) *volatile.Register32 {
	return (*volatile.Register32)(unsafe.Add(unsafe.Pointer(&esp.GPIO.PIN0), uintptr(p)*4))
}

func gpioOutSel(p machine.Pin) *volatile.Register32 {
	return (*volatile.Register32)(unsafe.Add(unsafe.Pointer(&esp.GPIO.FUNC0_OUT_SEL_CFG), uintptr(p)*4))
}

func configOut(p machine.Pin, high bool) {
	p.Configure(machine.PinConfig{Mode: machine.PinOutput})
	gpioPinReg(p).ClearBits(esp.GPIO_PIN_PAD_DRIVER) // push-pull, not open drain
	p.Set(high)
}

// routeCS connects a pin to the LCD_CS signal (optionally inverted), so the
// LCD_CAM hardware drives it in step with the row transfers.
func routeCS(p machine.Pin, invert bool) {
	v := uint32(sigLCDCS) | esp.GPIO_FUNC_OUT_SEL_CFG_OEN_SEL
	if invert {
		v |= esp.GPIO_FUNC_OUT_SEL_CFG_INV_SEL
	}
	gpioOutSel(p).Set(v)
}

func routeGPIO(p machine.Pin) { gpioOutSel(p).Set(sigGPIO) }

func (b *bus) init() {
	f, err := machine.GetCPUFrequency()
	if err != nil || f == 0 {
		f = 240_000000
	}
	b.cyclesPerUs = f / 1_000_000

	// Control lines. Idle levels as in M5GFX: everything low, SPH (an
	// active-low "chip select") high.
	configOut(pinPWR, false)
	configOut(pinOE, false)
	configOut(pinSPV, false)
	configOut(pinCKV, false)
	configOut(pinLE, false)
	configOut(pinSPH, true)

	// Peripheral clocks.
	esp.SYSTEM.SetPERIP_CLK_EN1_LCD_CAM_CLK_EN(1)
	esp.SYSTEM.SetPERIP_RST_EN1_LCD_CAM_RST(1)
	esp.SYSTEM.SetPERIP_RST_EN1_LCD_CAM_RST(0)
	if esp.SYSTEM.GetPERIP_CLK_EN1_DMA_CLK_EN() == 0 {
		esp.SYSTEM.SetPERIP_CLK_EN1_DMA_CLK_EN(1)
		esp.SYSTEM.SetPERIP_RST_EN1_DMA_RST(1)
		esp.SYSTEM.SetPERIP_RST_EN1_DMA_RST(0)
	}
	esp.DMA.MISC_CONF.SetBits(esp.DMA_MISC_CONF_CLK_EN)

	lcd := esp.LCD_CAM
	// PLL_F160M / 2 = 80 MHz LCD clock, PCLK = 80 / (4+1) = 16 MHz, PCLK idle
	// low, data launched on the falling edge (pclk_idle_low=1,
	// pclk_active_neg=0 in M5GFX's esp_lcd config).
	lcd.LCD_CLOCK.Set(lcdClkEn | 3<<lcdClkSelPos | 2<<lcdDivNumPos | 4<<lcdClkcntNPos)
	lcd.LCD_USER.SetBits(lcdReset)
	lcd.LCD_MISC.SetBits(lcdAFIFOReset)
	lcd.LC_DMA_INT_ENA.ClearBits(3)
	lcd.LC_DMA_INT_CLR.Set(3)
	lcd.LCD_CTRL.ClearBits(lcdRGBModeEn) // i8080 mode
	// Data phase only; its length is set by the DMA buffer (always-out).
	b.userVal = lcdDout | lcdAlwaysOut // 8-bit bus: 2BYTE_EN = 0
	lcd.LCD_USER.Set(b.userVal)
	misc := lcd.LCD_MISC.Get()
	misc &^= lcdVfkMask | lcdVbkMask | lcdNextFrameEn | 0xf<<28 // also DC levels: never toggled
	misc |= lcdBkEn                                             // 1 blank cycle front/back, as IDF
	lcd.LCD_MISC.Set(misc)

	// Route the LCD data lines and PCLK to the panel pins.
	for i, p := range dataPins {
		configOut(p, false)
		gpioOutSel(p).Set(uint32(sigLCDData0+i) | esp.GPIO_FUNC_OUT_SEL_CFG_OEN_SEL)
	}
	configOut(pinCL, false)
	gpioOutSel(pinCL).Set(sigLCDPCLK | esp.GPIO_FUNC_OUT_SEL_CFG_OEN_SEL)
	// SPH is the source driver "chip select": exactly LCD_CS, as in M5GFX.
	routeCS(pinSPH, false)

	// GDMA channel 4 (TX) -> LCD_CAM.
	esp.DMA.OUT_CONF0_CH4.Set(gdmaConf0Rst)
	esp.DMA.OUT_CONF0_CH4.Set(0)
	esp.DMA.OUT_CONF0_CH4.Set(gdmaOutConf0Burst)
	esp.DMA.OUT_PERI_SEL_CH4.Set(gdmaPeriLCD)
	esp.DMA.OUT_INT_ENA_CH4.Set(0)
	esp.DMA.OUT_INT_CLR_CH4.Set(0xff)

	for i := range b.desc {
		b.desc[i] = dmaDesc{
			dw0:  descOwnerDMA | descSucEOF | lineBytes<<descLenPos | lineBytes,
			buf:  uint32(uintptr(unsafe.Pointer(&b.lines[i][0]))),
			next: 0,
		}
	}
}

func (b *bus) powerOn() {
	if b.powered {
		return
	}
	b.powered = true
	if !b.DryRun {
		pinOE.High()
		b.delayUs(100)
		pinPWR.High()
		b.delayUs(100)
	}
	pinSPV.High()
	b.delayUs(1000)
}

func (b *bus) powerOff() {
	if !b.powered {
		return
	}
	b.powered = false
	b.delayUs(1000)
	pinPWR.Low()
	b.delayUs(10)
	pinOE.Low()
	b.delayUs(100)
	pinSPV.Low()
}

// beginFrame resets the gate driver to the first row (M5GFX
// Bus_EPD::beginTransaction).
func (b *bus) beginFrame() {
	pinSPV.Low()
	b.delayUs(1)
	pinCKV.Low()
	b.delayUs(3)
	pinCKV.High()
	b.delayUs(1)
	pinSPV.High()
	for i := 0; i < 3; i++ {
		b.delayUs(3)
		pinCKV.Low()
		b.delayUs(3)
		pinCKV.High()
	}
}

// startRow starts shifting line buffer i out to the source drivers.
func (b *bus) startRow(i int) {
	d := &b.desc[i]
	volatile.StoreUint32(&d.dw0, descOwnerDMA|descSucEOF|lineBytes<<descLenPos|lineBytes)
	lcd := esp.LCD_CAM
	lcd.LC_DMA_INT_CLR.Set(lcdTransDone)
	lcd.LCD_MISC.SetBits(lcdAFIFOReset)
	device.Asm("memw")
	esp.DMA.OUT_LINK_CH4.Set(uint32(uintptr(unsafe.Pointer(d)))&gdmaLinkAddrMask | gdmaOutLinkStart)
	b.delayUs(1) // let GDMA fill the LCD FIFO (ESP-IDF does the same)
	lcd.LCD_USER.Set(b.userVal | lcdUpdate)
	lcd.LCD_USER.Set(b.userVal | lcdUpdate | lcdStart)
}

// rowDone reports whether the current row transfer has finished.
func rowDone() bool { return esp.LCD_CAM.LC_DMA_INT_RAW.Get()&lcdTransDone != 0 }

// waitRow waits for the current row transfer to finish.
func (b *bus) waitRow() error {
	start := ccount()
	limit := 1000 * b.cyclesPerUs
	for !rowDone() {
		if ccount()-start > limit {
			return errTimeout
		}
	}
	esp.LCD_CAM.LC_DMA_INT_CLR.Set(lcdTransDone)
	return nil
}

// rowSignalsByHardware hands CKV and LE to the LCD_CS signal: during a row
// transfer CKV is high and LE low; between rows CKV is low and LE high (the
// latch pulse). This is the timing M5GFX produces in software (CKV high and
// LE low before starting a row, CKV low and LE high in the transfer-done
// interrupt), but exact and independent of how long the CPU needs to
// prepare the next row.
func rowSignalsByHardware(on bool) {
	if on {
		routeCS(pinCKV, true)
		routeCS(pinLE, false)
		return
	}
	// Back to GPIO, with the levels the hardware had between rows.
	pinCKV.Low()
	pinLE.High()
	routeGPIO(pinCKV)
	routeGPIO(pinLE)
}

// endFrame is M5GFX's vTaskDelay(1) + Bus_EPD::endTransaction. Like the
// FreeRTOS delay, the pause lets other goroutines run: an update takes up to
// a second, and the radio drivers miss their deadlines if they cannot run
// for that long (WiFi handshakes time out).
func (b *bus) endFrame() {
	start := ccount()
	runtime.Gosched()
	if dt := (ccount() - start) / b.cyclesPerUs; dt < FrameGapUs {
		b.delayUs(FrameGapUs - dt)
	}
	pinLE.Low()
	pinCKV.High()
}

// frame scans one full frame with interrupts disabled. fill(y, line) must
// fill line with the bus data for row y (240 bytes, the 8 padding bytes stay
// zero); it runs while the previous row is being shifted out.
func (b *bus) frame(fill func(y int, line *[lineWords]uint32)) error {
	st := interrupt.Disable()
	fill(0, &b.lines[0])
	fill(1, &b.lines[1])
	b.beginFrame() // leaves CKV high

	// Row 0 exactly like M5GFX: no CKV edge (it is already high), LE low.
	pinLE.Low()
	b.startRow(0)
	err := b.waitRow()
	rowSignalsByHardware(true) // CKV falls, LE rises: row 0 latched
	for y := 1; y < Height && err == nil; y++ {
		b.delayUs(RowGapUs)
		b.startRow(y & 1)
		if y+1 < Height {
			t0 := ccount()
			fill(y+1, &b.lines[(y+1)&1]) // buffer of row y-1, already sent
			if dt := (ccount() - t0) / b.cyclesPerUs; dt > b.FillMaxUs {
				b.FillMaxUs = dt
			}
			if rowDone() {
				b.LateRows++
			}
		}
		err = b.waitRow()
	}
	rowSignalsByHardware(false)
	interrupt.Restore(st)
	b.endFrame()
	return err
}

// ProbeRowSignals runs one dry row transfer and samples CKV, LE and SPH
// during and after it (bring-up check of the LCD_CS routing). The panel
// supply must be off.
func (b *bus) ProbeRowSignals() (during, after [3]bool) {
	sample := func() [3]bool {
		in := esp.GPIO.IN.Get()
		return [3]bool{in&(1<<pinCKV) != 0, in&(1<<pinLE) != 0, in&(1<<pinSPH) != 0}
	}
	st := interrupt.Disable()
	for i := range b.lines[0] {
		b.lines[0][i] = 0
	}
	rowSignalsByHardware(true)
	b.startRow(0)
	b.delayUs(4) // well inside the ~15 us transfer
	during = sample()
	b.waitRow()
	b.delayUs(1)
	after = sample()
	rowSignalsByHardware(false)
	interrupt.Restore(st)
	return
}

// DebugRegs prints the LCD/DMA state, for bring-up.
func (b *bus) DebugRegs() {
	println("LCD_CLOCK", hex(esp.LCD_CAM.LCD_CLOCK.Get()),
		"LCD_USER", hex(esp.LCD_CAM.LCD_USER.Get()),
		"LCD_MISC", hex(esp.LCD_CAM.LCD_MISC.Get()),
		"INT_RAW", hex(esp.LCD_CAM.LC_DMA_INT_RAW.Get()))
	println("DMA OUT_CONF0", hex(esp.DMA.OUT_CONF0_CH4.Get()),
		"OUT_LINK", hex(esp.DMA.OUT_LINK_CH4.Get()),
		"OUT_INT_RAW", hex(esp.DMA.OUT_INT_RAW_CH4.Get()),
		"PERI_SEL", hex(esp.DMA.OUT_PERI_SEL_CH4.Get()))
}

func hex(v uint32) string {
	const digits = "0123456789abcdef"
	var buf [10]byte
	buf[0], buf[1] = '0', 'x'
	for i := 0; i < 8; i++ {
		buf[9-i] = digits[v&0xf]
		v >>= 4
	}
	return string(buf[:])
}

// allocBuffers puts the frame buffer (259 KB) and the dirty map (65 KB) in
// PSRAM, so that the internal RAM stays free for the program and the radio
// (WiFi and Bluetooth together need about 160 KB of it). PSRAM is slow
// (~32 MB/s at 40 MHz), so fillRow has the cache preload the next frame
// buffer row in the background (prefetch). Measured against internal RAM,
// with the slower frame loop of that time (many late rows): full quality
// refresh 706 instead of 698 ms, a text refresh of most of the
// screen 692 instead of 640 ms (the dirty map rows are not prefetched), Paint
// strokes the same (110 ms). Without PSRAM both come from the heap.
func allocBuffers() (*[Width * Height / 8]uint32, *[Width * Height / 32]uint32) {
	if psram.Init() == nil {
		fbMem := psram.Alloc(Width * Height / 2)
		dMem := psram.Alloc(Width * Height / 8)
		if fbMem != nil && dMem != nil {
			return (*[Width * Height / 8]uint32)(unsafe.Pointer(&fbMem[0])),
				(*[Width * Height / 32]uint32)(unsafe.Pointer(&dMem[0]))
		}
	}
	return new([Width * Height / 8]uint32), new([Width * Height / 32]uint32)
}

// Data cache manual preload registers (EXTMEM_DCACHE_PRELOAD_*, ESP32-S3
// TRM, chapter 4 Cache).
var (
	preloadCtrl = (*volatile.Register32)(unsafe.Pointer(uintptr(0x600c4040)))
	preloadAddr = (*volatile.Register32)(unsafe.Pointer(uintptr(0x600c4044)))
	preloadSize = (*volatile.Register32)(unsafe.Pointer(uintptr(0x600c4048)))
)

const (
	preloadEna  = 1 << 0
	preloadDone = 1 << 1
)

// prefetch has the data cache load [p, p+n) from PSRAM in the background,
// so that the refresh loop finds the next row in the cache. It does nothing
// for internal RAM or while the previous preload is still running.
func prefetch(p unsafe.Pointer, n int) {
	a := uintptr(p)
	if a < 0x3c000000 || a >= 0x3e000000 {
		return
	}
	c := preloadCtrl.Get()
	if c&preloadEna != 0 && c&preloadDone == 0 {
		return
	}
	preloadCtrl.Set(0)
	preloadAddr.Set(uint32(a))
	preloadSize.Set(uint32(n))
	preloadCtrl.Set(preloadEna)
}
