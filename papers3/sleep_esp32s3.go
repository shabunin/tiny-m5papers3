//go:build esp32s3

package papers3

// Sleep modes of the ESP32-S3, written from the register sequences in
// ESP-IDF (components/esp_hw_support/port/esp32s3/rtc_sleep.c and
// sleep_modes.c), reduced to what this board needs:
//
//   - Light sleep stops the CPU clock. RAM, registers and GPIO outputs are
//     kept and the program continues after LightSleep returns. The 40 MHz
//     crystal and the PLL stay powered, which costs a little current but
//     keeps the wake-up simple (no clock switching, no re-calibration).
//   - Deep sleep powers down everything except the RTC domain. The chip
//     restarts from the beginning when the RTC timer fires.
//
// TinyGo's clock (TIMG0) stops in light sleep; LightSleep measures the time
// it slept with the RTC timer and moves the clock forward.

/*
#include <stdint.h>

// ROM functions (addresses from ESP-IDF esp32s3.rom.ld) for the internal
// analog I2C bus that sets the regulator voltages.
static uint8_t papers3_regi2c_read_mask(uint8_t block, uint8_t host, uint8_t reg, uint8_t msb, uint8_t lsb) {
	return ((uint8_t (*)(uint8_t, uint8_t, uint8_t, uint8_t, uint8_t))0x40005d54)(block, host, reg, msb, lsb);
}
static void papers3_regi2c_write_mask(uint8_t block, uint8_t host, uint8_t reg, uint8_t msb, uint8_t lsb, uint8_t data) {
	((void (*)(uint8_t, uint8_t, uint8_t, uint8_t, uint8_t, uint8_t))0x40005d6c)(block, host, reg, msb, lsb, data);
}

#define PAPERS3_REG(a) (*(volatile uint32_t *)(a))
#define PAPERS3_RTC_CNTL 0x60008000u
#define PAPERS3_CACHE_STATE 0x600c4130u // EXTMEM_CACHE_STATE_REG, 1 = idle
#define PAPERS3_SYSCLK_CONF 0x600c0060u // SYSTEM_SYSCLK_CONF_REG

// papers3_sleep_start starts the sleep and waits for the wake-up (light
// sleep) or never returns (deep sleep), like rtc_sleep_start in ESP-IDF. It
// runs from internal RAM and suspends the flash caches (ROM functions) for
// light sleep: nothing may be fetched from flash while the chip falls asleep
// and wakes up.
__attribute__((section(".iram1.papers3_sleep"), noinline))
static uint32_t papers3_sleep_start(uint32_t wake, uint32_t reject, uint32_t light) {
	uint32_t ic = 0, dc = 0;
	if (light) {
		ic = ((uint32_t (*)(void))0x4000189c)(); // rom_Cache_Suspend_ICache
		while ((PAPERS3_REG(PAPERS3_CACHE_STATE) & 0xfff) != 1) {
		}
		dc = ((uint32_t (*)(void))0x400018b4)(); // rom_Cache_Suspend_DCache
		while (((PAPERS3_REG(PAPERS3_CACHE_STATE) >> 12) & 0xfff) != 1) {
		}
	}
	// Run the CPU from the 40 MHz crystal while falling asleep and waking up
	// (SOC_CLK_SEL = XTAL, no divider): at 240 MHz the CPU fails when the
	// supply comes back.
	uint32_t sysclk = PAPERS3_REG(PAPERS3_SYSCLK_CONF);
	PAPERS3_REG(PAPERS3_SYSCLK_CONF) = sysclk & ~(0x3ffu | (3u << 10));
	// WAKEUP_ENA, SLEEP_REJECT_ENA, clear SLP_REJECT/SLP_WAKEUP, SLEEP_EN.
	PAPERS3_REG(PAPERS3_RTC_CNTL + 0x3c) = (PAPERS3_REG(PAPERS3_RTC_CNTL + 0x3c) & ~(0x1ffffu << 15)) | (wake << 15);
	PAPERS3_REG(PAPERS3_RTC_CNTL + 0x68) = (PAPERS3_REG(PAPERS3_RTC_CNTL + 0x68) & ~(0x3ffffu << 12)) | (reject << 12);
	PAPERS3_REG(PAPERS3_RTC_CNTL + 0x4c) = 3;
	PAPERS3_REG(PAPERS3_RTC_CNTL + 0x18) |= 1u << 31;
	while ((PAPERS3_REG(PAPERS3_RTC_CNTL + 0x44) & 3) == 0) {
	}
	uint32_t rejected = (PAPERS3_REG(PAPERS3_RTC_CNTL + 0x44) >> 1) & 1;
	PAPERS3_REG(PAPERS3_RTC_CNTL + 0x4c) = 3;
	if (light) {
		// Let the supply settle (~50 us at 40 MHz), then back to the PLL.
		for (volatile uint32_t i = 0; i < 500; i++) {
		}
		PAPERS3_REG(PAPERS3_SYSCLK_CONF) = sysclk;
		for (volatile uint32_t i = 0; i < 100; i++) {
		}
		((void (*)(uint32_t))0x400018c0)(dc); // Cache_Resume_DCache
		((void (*)(uint32_t))0x400018a8)(ic); // Cache_Resume_ICache
	}
	return rejected;
}
*/
import "C"

import (
	"device/esp"
	"machine"
	"runtime/interrupt"
	"time"
)

// Wake-up sources (RTC_CNTL_WAKEUP_ENA bits).
const (
	trigGPIO  = 1 << 2 // any GPIO, light sleep only
	trigTimer = 1 << 3 // RTC timer
)

// Analog I2C registers for the sleep voltages (soc/regi2c_dig_reg.h).
const (
	regi2cDig           = 0x6d
	regi2cDigHost       = 1
	regi2cRTCDreg       = 4 // bits 4:0
	regi2cDigDreg       = 6 // bits 4:0
	regi2cRTCDregSleep  = 5 // bits 4:0
	regi2cDigDregSleep  = 7 // bits 4:0
	dbias1V10           = 23
	rtcResetDeepSleep   = 5 // RTC_CNTL_RESET_CAUSE_PROCPU after deep sleep
	gpioIntLowLevel     = 4
	slowCalCycles       = 1024 // RTC slow clock cycles per calibration (~7 ms)
	xtalHz              = 40_000_000
	pinTouchIntNumber   = 48
	rtcTimerHighMask    = 0xffff
	rtcSleepRejectMask  = 0x3ffff
	rtcWakeupEnableMask = 0x1ffff
)

var (
	// slowPeriod is the RTC slow clock period in XTAL cycles, << 16.
	slowPeriod uint32
	lastSlept  time.Duration
)

// The touch panel pulls its INT line low when it has new data and keeps it
// low until the data is read (ReadTouches).
const pinTouchInt = machine.GPIO48

func init() {
	pinTouchInt.Configure(machine.PinConfig{Mode: machine.PinInput})
}

// Retained returns word i (0 or 1) of the memory that survives DeepSleep.
func Retained(i int) uint32 {
	switch i {
	case 0:
		return esp.RTC_CNTL.STORE2.Get()
	case 1:
		return esp.RTC_CNTL.STORE3.Get()
	}
	return 0
}

// SetRetained sets word i (0 or 1) of the memory that survives DeepSleep.
// (RTC_CNTL_STORE2/3, the boot-time words of ESP-IDF, which this program
// does not use otherwise.)
func SetRetained(i int, v uint32) {
	switch i {
	case 0:
		esp.RTC_CNTL.STORE2.Set(v)
	case 1:
		esp.RTC_CNTL.STORE3.Set(v)
	}
}

// startReason works out why the program started (called by Configure, after
// the I2C bus is up).
func startReason() Wake {
	alarm := rtcTimerFired() // always: clears the RTC flags
	switch {
	case esp.RTC_CNTL.GetRESET_STATE_RESET_CAUSE_PROCPU() == rtcResetDeepSleep:
		return WakeTimer
	case alarm:
		return WakeAlarm
	}
	return WakeNone
}

// ResetCause returns the chip's reset reason code (ESP-IDF soc/reset_reasons.h:
// 1 power-on, 3 software, 5 deep sleep, 7/8 watchdog, 15 brown-out, 21/22 USB).
func ResetCause() int { return int(esp.RTC_CNTL.GetRESET_STATE_RESET_CAUSE_PROCPU()) }

// USBHost reports whether a computer is talking to the USB port: the USB
// serial/JTAG controller receives a start-of-frame packet every millisecond
// while a host is connected. A charger alone sends none. Sleeping while a
// host is connected drops the serial port (and flashing needs it).
func USBHost() bool {
	f := esp.USB_DEVICE.FRAM_NUM.Get() & 0x7ff
	start := time.Now()
	for time.Since(start) < 3*time.Millisecond {
		if esp.USB_DEVICE.FRAM_NUM.Get()&0x7ff != f {
			return true
		}
	}
	return false
}

// rtcTicks reads the RTC timer (48 bits, counting RTC slow clock cycles in
// all power modes). TIME_UPDATE latches the counter; the value is accepted
// once a second read a little later shows it moving forward.
func rtcTicks() uint64 {
	read := func() uint64 {
		esp.RTC_CNTL.TIME_UPDATE.SetBits(esp.RTC_CNTL_TIME_UPDATE_TIME_UPDATE)
		lo := esp.RTC_CNTL.TIME_LOW0.Get()
		hi := esp.RTC_CNTL.TIME_HIGH0.Get() & rtcTimerHighMask
		return uint64(hi)<<32 | uint64(lo)
	}
	a := read()
	for range 8 {
		spinMicros(20) // ~3 slow clock cycles
		b := read()
		if b > a && b-a < 1000 {
			return b
		}
		a = b
	}
	return a
}

// spinMicros busy-waits on TinyGo's clock (works with interrupts off).
func spinMicros(us uint64) {
	start := timerTicks()
	for timerTicks()-start < us*40 {
	}
}

// calibrate measures the RTC slow clock (the internal ~150 kHz RC
// oscillator, which drifts with temperature) against the crystal, using the
// calibration counter in timer group 0 (rtc_clk_cal in ESP-IDF). A result
// outside 100-250 kHz is ignored.
func calibrate() {
	t := esp.TIMG0
	const thresMsk = esp.TIMG_RTCCALICFG2_RTC_CALI_TIMEOUT_THRES_Msk
	const thresPos = esp.TIMG_RTCCALICFG2_RTC_CALI_TIMEOUT_THRES_Pos
	const rdy = esp.TIMG_RTCCALICFG_RTC_CALI_RDY
	const timeout = esp.TIMG_RTCCALICFG2_RTC_CALI_TIMEOUT
	if t.RTCCALICFG.Get()&esp.TIMG_RTCCALICFG_RTC_CALI_START_CYCLING != 0 {
		// Stop the periodic calibration that runs after reset.
		t.RTCCALICFG2.Set(t.RTCCALICFG2.Get()&^thresMsk | 1<<thresPos)
		start := timerTicks()
		for t.RTCCALICFG.Get()&rdy == 0 && t.RTCCALICFG2.Get()&timeout == 0 &&
			timerTicks()-start < 50_000*40 {
		}
	}
	cfg := t.RTCCALICFG.Get()
	cfg &^= esp.TIMG_RTCCALICFG_RTC_CALI_START | esp.TIMG_RTCCALICFG_RTC_CALI_START_CYCLING |
		esp.TIMG_RTCCALICFG_RTC_CALI_CLK_SEL_Msk | esp.TIMG_RTCCALICFG_RTC_CALI_MAX_Msk
	cfg |= slowCalCycles << esp.TIMG_RTCCALICFG_RTC_CALI_MAX_Pos // CLK_SEL 0: RTC slow clock
	t.RTCCALICFG.Set(cfg)
	// Time-out threshold in XTAL cycles: 1024 per slow cycle (ESP-IDF).
	t.RTCCALICFG2.Set(t.RTCCALICFG2.Get()&^thresMsk | (slowCalCycles<<10)<<thresPos&thresMsk)
	t.RTCCALICFG.SetBits(esp.TIMG_RTCCALICFG_RTC_CALI_START)
	// The ready flag of the previous run may still be set: first wait for
	// the expected time (1024 cycles at 150 kHz), like ESP-IDF.
	spinMicros(slowCalCycles * 1_000_000 / 150_000)
	start := timerTicks()
	for t.RTCCALICFG.Get()&rdy == 0 {
		if t.RTCCALICFG2.Get()&timeout != 0 || timerTicks()-start > 50_000*40 {
			t.RTCCALICFG.ClearBits(esp.TIMG_RTCCALICFG_RTC_CALI_START)
			return // keep the previous value
		}
	}
	v := t.RTCCALICFG1.Get() >> esp.TIMG_RTCCALICFG1_RTC_CALI_VALUE_Pos // XTAL cycles
	t.RTCCALICFG.ClearBits(esp.TIMG_RTCCALICFG_RTC_CALI_START)
	// 250 kHz .. 100 kHz: 164 .. 410 XTAL cycles per slow cycle.
	if v < slowCalCycles*160 || v > slowCalCycles*410 {
		return
	}
	slowPeriod = uint32(uint64(v) << 16 / slowCalCycles)
}

// SlowClockHz returns the measured frequency of the RTC slow clock.
func SlowClockHz() int {
	if slowPeriod == 0 {
		calibrate()
	}
	if slowPeriod == 0 {
		return 0
	}
	return int(uint64(xtalHz) << 16 / uint64(slowPeriod))
}

func durationToSlow(d time.Duration) uint64 {
	xtal := uint64(d) / (1e9 / xtalHz) // 25 ns per XTAL cycle
	return xtal << 16 / uint64(slowPeriod)
}

func slowToDuration(ticks uint64) time.Duration {
	return time.Duration(ticks * uint64(slowPeriod) >> 16 * (1e9 / xtalHz))
}

func setSleepTimer(d time.Duration) (ticks uint64) {
	if slowPeriod == 0 {
		calibrate()
	}
	if slowPeriod == 0 {
		slowPeriod = uint32(uint64(xtalHz) << 16 / 150_000)
	}
	ticks = durationToSlow(d)
	// Never trust a conversion far from the nominal 150 kHz: a wrong wake-up
	// time could leave the chip asleep for days.
	nominal := uint64(d) * 150_000 / 1e9
	if ticks < nominal*2/3 || ticks > nominal*3/2 {
		ticks = nominal
	}
	if ticks < 20 {
		ticks = 20
	}
	t := rtcTicks() + ticks
	esp.RTC_CNTL.SLP_TIMER0.Set(uint32(t))
	esp.RTC_CNTL.SLP_TIMER1.Set(uint32(t>>32) & rtcTimerHighMask)
	esp.RTC_CNTL.INT_CLR_RTC.Set(esp.RTC_CNTL_INT_CLR_RTC_MAIN_TIMER_INT_CLR)
	esp.RTC_CNTL.SLP_TIMER1.SetBits(esp.RTC_CNTL_SLP_TIMER1_MAIN_TIMER_ALARM_EN)
	return ticks
}

// RTC watchdog (RWDT) settings.
const (
	wdtKey        = 0x50d83aa1 // WDTWPROTECT: unlocks the watchdog registers
	wdtResetRTC   = 4          // stage action: reset the whole chip, RTC included
	wdtResetWidth = 7          // reset pulse length: 3.2 us
)

// armWatchdog starts the RTC watchdog as a safety net for a sleep of the
// given number of RTC slow clock cycles: if the chip has not woken up (and
// disarmed it, or restarted) by then plus a margin, the watchdog resets the
// chip. The watchdog counts the same clock as the sleep timer and keeps
// counting while the chip sleeps. TinyGo's runtime switches it off at
// start-up, so a normal wake from deep sleep needs no extra step.
func armWatchdog(ticks uint64) {
	ticks += ticks/8 + 2*150_000 // + 12% + about 2 s
	// The hardware doubles the stage 0 time (see rwdt_ll_config_stage in
	// ESP-IDF); longer sleeps (> 15 h) are not covered.
	hold := ticks / 2
	if hold > 0xffffffff {
		return
	}
	r := esp.RTC_CNTL
	r.WDTWPROTECT.Set(wdtKey)
	r.WDTCONFIG0.Set(0)
	r.WDTCONFIG1.Set(uint32(hold))
	r.WDTCONFIG0.Set(esp.RTC_CNTL_WDTCONFIG0_WDT_EN |
		wdtResetRTC<<esp.RTC_CNTL_WDTCONFIG0_WDT_STG0_Pos |
		wdtResetWidth<<esp.RTC_CNTL_WDTCONFIG0_WDT_CPU_RESET_LENGTH_Pos |
		wdtResetWidth<<esp.RTC_CNTL_WDTCONFIG0_WDT_SYS_RESET_LENGTH_Pos) // PAUSE_IN_SLP off
	r.WDTFEED.Set(esp.RTC_CNTL_WDTFEED_WDT_FEED)
	r.WDTWPROTECT.Set(0)
}

// disarmWatchdog stops the RTC watchdog again.
func disarmWatchdog() {
	r := esp.RTC_CNTL
	r.WDTWPROTECT.Set(wdtKey)
	r.WDTCONFIG0.Set(0)
	r.WDTWPROTECT.Set(0)
}

// sleepVoltage sets the regulator targets used while asleep.
func sleepVoltage(rtc, dig uint8) {
	C.papers3_regi2c_write_mask(regi2cDig, regi2cDigHost, regi2cRTCDregSleep, 4, 0, C.uint8_t(rtc))
	C.papers3_regi2c_write_mask(regi2cDig, regi2cDigHost, regi2cDigDregSleep, 4, 0, C.uint8_t(dig))
}

// SleepVoltages returns the analog regulator settings (for debugging): the
// RTC and digital targets while running and while asleep.
func SleepVoltages() (rtc, dig, rtcSleep, digSleep uint8) {
	rd := func(reg uint8) uint8 {
		return uint8(C.papers3_regi2c_read_mask(regi2cDig, regi2cDigHost, C.uint8_t(reg), 4, 0))
	}
	return rd(regi2cRTCDreg), rd(regi2cDigDreg), rd(regi2cRTCDregSleep), rd(regi2cDigDregSleep)
}

// LightSleep stops the CPU until d has passed or the touch panel reports a
// touch, and returns which one it was. The display, RAM and all settings are
// kept. The system clock (time.Now) is corrected for the time spent
// sleeping.
//
// The USB serial port does not work while the chip sleeps: a connected
// computer sees it hang (see USBHost).
func LightSleep(d time.Duration) Wake {
	if d <= 0 {
		return WakeTimer
	}
	w := lightSleep(d)
	keepClock()
	return w
}

// keepClock corrects the system clock from the RTC chip once it is outside
// the second the RTC shows. The time spent in light sleep is measured with
// the chip's RC oscillator, which drifts with temperature (~1%); the RTC
// runs from a crystal.
func keepClock() {
	if !status.RTC || !status.RTCValid {
		return
	}
	var r [7]byte
	if i2c.Tx(rtcAddress, []byte{rtcSeconds}, r[:]) != nil {
		return
	}
	rtc, valid := decodeRTC(r[:])
	if !valid {
		return
	}
	switch now := Now(); {
	case now.Before(rtc):
		setSystemTime(rtc)
	case !now.Before(rtc.Add(time.Second)):
		setSystemTime(rtc.Add(time.Second - time.Millisecond))
	}
}

func lightSleep(d time.Duration) Wake {
	if !pinTouchInt.Get() {
		return WakeTouch // unread touch data
	}
	machine.FlushSerial()
	r := esp.RTC_CNTL
	// No interrupts: their handlers run from flash, and the flash cache is
	// suspended around the sleep.
	state := interrupt.Disable()
	defer interrupt.Restore(state)

	// Wake on a low level of the touch INT pin.
	pin := &esp.GPIO.PIN48
	pinCfg := pin.Get()
	pin.Set(pinCfg&^(esp.GPIO_PIN_INT_TYPE_Msk|esp.GPIO_PIN_INT_ENA_Msk) |
		gpioIntLowLevel<<esp.GPIO_PIN_INT_TYPE_Pos | esp.GPIO_PIN_WAKEUP_ENABLE)

	ticks := setSleepTimer(d)

	// Power domains: keep everything on (rtc_sleep_init with no PD flags).
	r.SetDIG_PWC_WIFI_PD_EN(0)
	r.SetDIG_PWC_CPU_TOP_PD_EN(0)
	r.SetDIG_PWC_DG_PERI_PD_EN(0)
	r.SetDIG_PWC_DG_WRAP_PD_EN(0)
	r.SetDIG_PWC_LSLP_MEM_FORCE_PU(1)
	r.SetPWC_PD_EN(0)
	r.SetPWC_FASTMEM_FORCE_LPU(1)
	r.SetPWC_SLOWMEM_FORCE_LPU(1)
	r.SetREGULATOR_DRV_CTRL_DG_VDD_DRV_B_SLP(0xf)
	// No voltage drop: sleep at the running voltages.
	rtcV, digV, _, _ := SleepVoltages()
	sleepVoltage(rtcV, digV)
	r.SetBIAS_CONF_DBG_ATTEN_DEEP_SLP(0)
	r.SetBIAS_CONF_BIAS_SLEEP_DEEP_SLP(0)
	r.SetBIAS_CONF_PD_CUR_DEEP_SLP(0)
	r.SetBIAS_CONF_DBG_ATTEN_MONITOR(0)
	r.SetBIAS_CONF_BIAS_SLEEP_MONITOR(0)
	r.SetBIAS_CONF_PD_CUR_MONITOR(0)
	r.SetRTC_REGULATOR_FORCE_PU(1)
	clk := r.CLK_CONF.Get()
	r.SetCLK_CONF_CK8M_FORCE_PU(0)
	r.SetSDIO_CONF_SDIO_FORCE(0)
	r.SetSDIO_CONF_SDIO_REG_PD_EN(0) // keep the flash powered
	r.SetSLP_REJECT_CONF_LIGHT_SLP_REJECT_EN(1)
	r.SetSLP_REJECT_CONF_DEEP_SLP_REJECT_EN(1)
	r.SetTIMER2_ULPCP_TOUCH_START_WAIT(0xff)
	// Crystal and PLL keep running; the CPU clock is only gated.
	opts := r.OPTIONS0.Get()
	r.SetOPTIONS0_XTL_FORCE_PU(1)
	r.SetOPTIONS0_BBPLL_FORCE_PU(1)
	r.SetOPTIONS0_BBPLL_I2C_FORCE_PU(1)
	r.SetOPTIONS0_BB_I2C_FORCE_PU(1)
	r.SetCLK_CONF_XTAL_GLOBAL_FORCE_NOGATING(1)
	// rtc_sleep_low_init: wake-up wait times.
	r.SetTIMER1_PLL_BUF_WAIT(1)
	r.SetTIMER1_XTL_BUF_WAIT(uint32(durationToSlow(250*time.Microsecond)) + 1)
	r.SetTIMER1_CK8M_WAIT(4)

	armWatchdog(ticks)
	t0, c0 := rtcTicks(), timerTicks()
	rejected := C.papers3_sleep_start(trigGPIO|trigTimer, trigGPIO|trigTimer, 1) != 0
	t1, c1 := rtcTicks(), timerTicks()
	disarmWatchdog()
	cause := r.SLP_WAKEUP_CAUSE.Get() & rtcWakeupEnableMask

	r.SetTIMER2_ULPCP_TOUCH_START_WAIT(0x10)
	r.OPTIONS0.Set(opts)
	r.CLK_CONF.Set(clk)
	r.SLP_TIMER1.ClearBits(esp.RTC_CNTL_SLP_TIMER1_MAIN_TIMER_ALARM_EN)
	pin.Set(pinCfg)
	esp.GPIO.STATUS1_W1TC.Set(1 << (pinTouchIntNumber - 32))

	// Move TinyGo's clock forward by the time the timer group was stopped.
	slept := slowToDuration(t1 - t0)
	counted := time.Duration(c1-c0) * 25
	if slept > counted {
		advanceTimer(uint64((slept - counted) / 25))
	}
	lastSlept = slept

	switch {
	case rejected:
		if !pinTouchInt.Get() {
			return WakeTouch
		}
		return WakeNone
	case cause&trigGPIO != 0:
		return WakeTouch
	}
	return WakeTimer
}

// LastSleep returns how long the last LightSleep lasted (RTC timer).
func LastSleep() time.Duration { return lastSlept }

// timerTicks reads timer 0 of timer group 0, TinyGo's clock (40 MHz).
func timerTicks() uint64 {
	t := esp.TIMG0
	t.T0UPDATE.Set(1)
	for t.T0UPDATE.Get() != 0 {
	}
	return uint64(t.T0LO.Get()) | uint64(t.T0HI.Get())<<32
}

// advanceTimer adds n ticks to TinyGo's clock.
func advanceTimer(n uint64) {
	t := esp.TIMG0
	v := timerTicks() + n
	t.T0LOADLO.Set(uint32(v))
	t.T0LOADHI.Set(uint32(v >> 32))
	t.T0LOAD.Set(1)
}

// DeepSleep powers down the chip for d. Then the program starts again from
// the beginning, with GetStatus().Wake == WakeTimer. RAM is lost (use
// SetRetained for a few words), the e-paper keeps its image, the RTC keeps
// the time. The USB port disappears while the chip sleeps.
func DeepSleep(d time.Duration) {
	machine.FlushSerial()
	r := esp.RTC_CNTL
	interrupt.Disable()
	calibrate()
	ticks := setSleepTimer(d)
	esp.RTC_CNTL.STORE6.Set(0) // no wake stub: boot normally

	// rtc_sleep_init for deep sleep: digital domain, crystal, RC_FAST and the
	// flash supply off; RTC voltage reduced (the "default" deep sleep mode).
	r.SetDIG_PWC_DG_WRAP_PD_EN(1)
	r.SetANA_CONF_CKGEN_I2C_PU(0)
	r.SetANA_CONF_PLL_I2C_PU(0)
	r.SetANA_CONF_RFRX_PBUS_PU(0)
	r.SetANA_CONF_TXRF_I2C_PU(0)
	r.SetOPTIONS0_BB_I2C_FORCE_PU(0)
	r.SetDIG_PWC_LSLP_MEM_FORCE_PU(1)
	r.SetPWC_PD_EN(1)
	r.SetPWC_FORCE_PU(0)
	r.SetPWC_FORCE_NOISO(0)
	r.SetPWC_FORCE_ISO(0)
	r.SetPWC_FASTMEM_FORCE_LPU(1)
	r.SetPWC_SLOWMEM_FORCE_LPU(1)
	sleepVoltage(dbias1V10, 0)
	r.SetBIAS_CONF_DBG_ATTEN_DEEP_SLP(14)
	r.SetBIAS_CONF_BIAS_SLEEP_DEEP_SLP(1)
	r.SetBIAS_CONF_PD_CUR_DEEP_SLP(1)
	r.SetBIAS_CONF_DBG_ATTEN_MONITOR(0)
	r.SetBIAS_CONF_BIAS_SLEEP_MONITOR(1)
	r.SetBIAS_CONF_PD_CUR_MONITOR(1)
	r.SetRTC_REGULATOR_FORCE_PU(1)
	r.SetCLK_CONF_CK8M_FORCE_PU(0)
	r.SetSDIO_CONF_SDIO_FORCE(0)
	r.SetSDIO_CONF_SDIO_REG_PD_EN(1)
	r.SetSLP_REJECT_CONF_DEEP_SLP_REJECT_EN(0)
	r.SetTIMER2_ULPCP_TOUCH_START_WAIT(0xff)
	r.SetOPTIONS0_XTL_FORCE_PU(0)
	r.SetCLK_CONF_XTAL_GLOBAL_FORCE_NOGATING(0)

	// Hand the power switches of the digital domains to the sleep state
	// machine, as rtc_init does at start-up in ESP-IDF. TinyGo leaves the
	// reset values, which force the domains on and unisolated, so deep sleep
	// could not cut their power.
	r.DIG_PWC.ClearBits(esp.RTC_CNTL_DIG_PWC_DG_WRAP_FORCE_PU | esp.RTC_CNTL_DIG_PWC_WIFI_FORCE_PU |
		esp.RTC_CNTL_DIG_PWC_BT_FORCE_PU | esp.RTC_CNTL_DIG_PWC_CPU_TOP_FORCE_PU |
		esp.RTC_CNTL_DIG_PWC_DG_PERI_FORCE_PU)
	r.DIG_ISO.ClearBits(esp.RTC_CNTL_DIG_ISO_DG_WRAP_FORCE_NOISO | esp.RTC_CNTL_DIG_ISO_DG_WRAP_FORCE_ISO |
		esp.RTC_CNTL_DIG_ISO_WIFI_FORCE_NOISO | esp.RTC_CNTL_DIG_ISO_WIFI_FORCE_ISO |
		esp.RTC_CNTL_DIG_ISO_BT_FORCE_NOISO | esp.RTC_CNTL_DIG_ISO_BT_FORCE_ISO |
		esp.RTC_CNTL_DIG_ISO_CPU_TOP_FORCE_NOISO | esp.RTC_CNTL_DIG_ISO_CPU_TOP_FORCE_ISO |
		esp.RTC_CNTL_DIG_ISO_DG_PERI_FORCE_NOISO | esp.RTC_CNTL_DIG_ISO_DG_PERI_FORCE_ISO |
		esp.RTC_CNTL_DIG_ISO_DG_PAD_FORCE_UNHOLD | esp.RTC_CNTL_DIG_ISO_DG_PAD_FORCE_NOISO)
	r.PWC.ClearBits(esp.RTC_CNTL_PWC_FASTMEM_FORCE_NOISO | esp.RTC_CNTL_PWC_SLOWMEM_FORCE_NOISO)
	r.RTC.ClearBits(esp.RTC_CNTL_RTC_DBOOST_FORCE_PU)

	// If the timer does not wake the chip, the watchdog resets it.
	armWatchdog(ticks)
	C.papers3_sleep_start(trigTimer, 0, 0)
	for {
	}
}
