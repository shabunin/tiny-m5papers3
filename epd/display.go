package epd

import (
	"image/color"
	"time"
	"unsafe"
)

// Gray levels: 0 is black, 15 is white.
const (
	Black uint8 = 0
	White uint8 = 15
)

// Rotation of the drawing coordinates.
type Rotation uint8

const (
	Portrait         Rotation = iota // 540x960, M5Stack's default orientation
	Landscape                        // 960x540, native panel orientation
	PortraitFlipped                  // 540x960, upside down
	LandscapeFlipped                 // 960x540, upside down
)

// Mode selects the waveform used for an update. Every mode only drives
// pixels that changed since they were last sent to the panel (see Refresh);
// Update and UpdateRect first mark their whole area as changed.
type Mode uint8

const (
	// ModeQuality shows all 16 gray levels. The changed pixels flash
	// black/white first (~0.6 s).
	ModeQuality Mode = iota
	// ModeText is like ModeQuality, tuned for crisp black text (~0.56 s).
	ModeText
	// ModeFast shows black and white only (levels < 8 become black), without
	// flashing (~0.1 s). Leaves a little ghosting behind, which the next
	// ModeQuality/ModeText update of that area removes.
	ModeFast
	// ModeFastest is ModeFast with fewer frames (~65 ms) and more ghosting.
	ModeFastest
)

// fb is the frame buffer: 4 bits per pixel, two pixels per byte (even x in the
// high nibble), rows of Width pixels in native panel orientation. fbWords is
// the same memory as words, so that rows can be read 32 bits at a time.
//
// dirty has one bit per pixel, set when the frame buffer pixel changed since
// it was last driven. Rows of Width bits; the MSB of each byte is its leftmost
// pixel, matching the bus order.
//
// allocBuffers (platform specific) provides the memory: on the ESP32-S3 it is
// PSRAM, which leaves the internal RAM free for other things (WiFi).
var (
	fbWords    *[Width * Height / 8]uint32
	fb         *[Width * Height / 2]byte
	dirtyWords *[Width * Height / 32]uint32
	dirty      *[Width * Height / 8]byte
)

func init() {
	fbWords, dirtyWords = allocBuffers()
	fb = (*[Width * Height / 2]byte)(unsafe.Pointer(fbWords))
	dirty = (*[Width * Height / 8]byte)(unsafe.Pointer(dirtyWords))
}

// expand4 turns 4 dirty bits (MSB = first pixel) into an 8-bit bus mask.
var expand4 = func() (t [16]uint8) {
	for n := range 16 {
		for i := range 4 {
			if n>>i&1 != 0 {
				t[n] |= 3 << (2 * i)
			}
		}
	}
	return
}()

// expand8 turns 8 dirty bits into a 16-bit mask for two bus bytes (little
// endian: first 4 pixels in the low byte).
var expand8 = func() (t [256]uint16) {
	for n := range 256 {
		t[n] = uint16(expand4[n>>4]) | uint16(expand4[n&15])<<8
	}
	return
}()

// bayer is a 4x4 ordered dithering matrix (as in M5GFX).
var bayer = [16]uint8{0, 8, 2, 10, 12, 4, 14, 6, 3, 11, 1, 9, 15, 7, 13, 5}

// Display is the M5PaperS3 e-paper display. It implements tinygo.org/x/drivers
// Displayer, so it works with tinyfont, tinydraw etc.
type Display struct {
	bus

	rot Rotation
	t2  [256]uint8 // per frame: frame buffer byte (2 px) -> 2x2-bit bus codes
	t2h [256]uint8 // t2 << 4

	// full is set while updating the whole screen with every pixel marked
	// changed: the fill loop can then skip the dirty bits.
	full bool

	// uniform is set when every level gets the same action this frame (flash
	// and eraser frames); ucode is then that action for 16 pixels.
	uniform bool
	ucode   uint32

	// update region: words of 16 pixels [rw0,rw1), rows [ry0,ry1)
	rw0, rw1, ry0, ry1 int
	fill               func(y int, line *[lineWords]uint32)

	// Waveform tuning.
	ClearCycles int // black/white flash cycles in ModeQuality/ModeText
	ClearFrames int // frames per flash phase

	// Statistics of the last update.
	Frames   int    // frames driven
	UpdateUs uint32 // total time
}

// New returns the display. Call Configure before use.
func New() *Display {
	return &Display{
		ClearCycles: 2,
		ClearFrames: 8,
	}
}

// Configure sets up the pins, LCD_CAM and GDMA and clears the frame buffer to
// white. It does not drive the panel: call Update to show the frame buffer.
func (d *Display) Configure() {
	d.bus.init()
	d.fill = d.fillRow
	d.FillScreen(White)
	d.MarkAllDirty()
}

// SetRotation sets the drawing orientation.
func (d *Display) SetRotation(r Rotation) { d.rot = r & 3 }

// Rotation returns the drawing orientation.
func (d *Display) Rotation() Rotation { return d.rot }

// Size returns the drawing area size for the current rotation.
func (d *Display) Size() (x, y int16) {
	if d.rot&1 == 0 {
		return Height, Width
	}
	return Width, Height
}

// toNative maps drawing coordinates to native panel coordinates (same
// convention as M5GFX with its panel rotation offset of 3).
func (d *Display) toNative(x, y int) (mx, my int) {
	switch d.rot {
	case Portrait:
		return y, Height - 1 - x
	case Landscape:
		return x, y
	case PortraitFlipped:
		return Width - 1 - y, x
	default:
		return Width - 1 - x, Height - 1 - y
	}
}

// FromPortrait converts coordinates given in the Portrait orientation (such
// as raw touch coordinates of the GT911) to the current rotation.
func (d *Display) FromPortrait(x, y int16) (int16, int16) {
	mx, my := y, Height-1-x // native
	switch d.rot {
	case Portrait:
		return x, y
	case Landscape:
		return mx, my
	case PortraitFlipped:
		return my, Width - 1 - mx
	default:
		return Width - 1 - mx, Height - 1 - my
	}
}

// clip clips a rectangle to the drawing area; the result is inclusive.
func (d *Display) clip(x, y, w, h int16) (x0, y0, x1, y1 int, ok bool) {
	sw, sh := d.Size()
	x0, y0 = max(int(x), 0), max(int(y), 0)
	x1, y1 = min(int(x)+int(w), int(sw))-1, min(int(y)+int(h), int(sh))-1
	return x0, y0, x1, y1, x0 <= x1 && y0 <= y1
}

// nativeRect converts an inclusive drawing rectangle to an inclusive native one.
func (d *Display) nativeRect(x0, y0, x1, y1 int) (int, int, int, int) {
	ax, ay := d.toNative(x0, y0)
	bx, by := d.toNative(x1, y1)
	return min(ax, bx), min(ay, by), max(ax, bx), max(ay, by)
}

func setNative(mx, my int, level uint8) {
	i := my*(Width/2) + mx>>1
	old := fb[i]
	v := old&0xf0 | level
	if mx&1 == 0 {
		v = old&0x0f | level<<4
	}
	if v != old {
		fb[i] = v
		dirty[my*(Width/8)+mx>>3] |= 0x80 >> (mx & 7)
	}
}

// SetGray sets a pixel to a gray level (0 = black ... 15 = white).
func (d *Display) SetGray(x, y int16, level uint8) {
	w, h := d.Size()
	if x < 0 || y < 0 || x >= w || y >= h {
		return
	}
	mx, my := d.toNative(int(x), int(y))
	setNative(mx, my, level&15)
}

// SetGray8 sets a pixel from an 8-bit gray value (0 = black ... 255 =
// white), with ordered dithering between the 16 levels.
func (d *Display) SetGray8(x, y int16, v uint8) {
	w, h := d.Size()
	if x < 0 || y < 0 || x >= w || y >= h {
		return
	}
	mx, my := d.toNative(int(x), int(y))
	l := (int(v) + int(bayer[(my&3)<<2|mx&3]) - 8) >> 4
	setNative(mx, my, uint8(min(max(l, 0), 15)))
}

// Gray returns the gray level of a pixel in the frame buffer.
func (d *Display) Gray(x, y int16) uint8 {
	w, h := d.Size()
	if x < 0 || y < 0 || x >= w || y >= h {
		return White
	}
	mx, my := d.toNative(int(x), int(y))
	v := fb[my*(Width/2)+mx>>1]
	if mx&1 == 0 {
		return v >> 4
	}
	return v & 0x0f
}

// Luma converts a color to an 8-bit gray value.
func Luma(c color.RGBA) uint8 {
	return uint8((uint32(c.R)*77 + uint32(c.G)*150 + uint32(c.B)*29) >> 8)
}

// SetPixel implements drivers.Displayer (with dithering).
func (d *Display) SetPixel(x, y int16, c color.RGBA) { d.SetGray8(x, y, Luma(c)) }

// FillRect fills a rectangle with a gray level.
func (d *Display) FillRect(x, y, w, h int16, level uint8) {
	x0, y0, x1, y1, ok := d.clip(x, y, w, h)
	if !ok {
		return
	}
	nx0, ny0, nx1, ny1 := d.nativeRect(x0, y0, x1, y1)
	for my := ny0; my <= ny1; my++ {
		for mx := nx0; mx <= nx1; mx++ {
			setNative(mx, my, level&15)
		}
	}
}

// FillScreen fills the whole frame buffer with a gray level.
func (d *Display) FillScreen(level uint8) {
	level &= 15
	v := level<<4 | level
	for my := range Height {
		row := fb[my*(Width/2) : (my+1)*(Width/2)]
		drow := dirty[my*(Width/8) : (my+1)*(Width/8)]
		for i, old := range row {
			if diff := old ^ v; diff != 0 {
				row[i] = v
				var m byte
				if diff&0xf0 != 0 {
					m |= 2
				}
				if diff&0x0f != 0 {
					m |= 1
				}
				px := 2 * i
				drow[px>>3] |= m << (6 - px&7)
			}
		}
	}
}

// FrameBuffer returns the frame buffer: 4 bits per pixel (0 black ... 15
// white), two pixels per byte with the even x in the high nibble, rows of
// Width pixels in native orientation. Writing to it bypasses the change
// tracking (use MarkDirty).
func (d *Display) FrameBuffer() []byte { return fb[:] }

// MarkAllDirty marks every pixel as changed, so that the next Refresh
// redraws the whole screen.
func (d *Display) MarkAllDirty() {
	for i := range dirtyWords {
		dirtyWords[i] = 0xffffffff
	}
}

// MarkClean declares that the panel already shows the frame buffer, so the
// next Refresh only drives pixels drawn after this call. A program that
// starts after DeepSleep (the RAM was lost, the panel kept its image) redraws
// what was on the screen, calls MarkClean and then draws the changes.
func (d *Display) MarkClean() {
	for i := range dirtyWords {
		dirtyWords[i] = 0
	}
}

// MarkDirty marks a rectangle as changed.
func (d *Display) MarkDirty(x, y, w, h int16) {
	x0, y0, x1, y1, ok := d.clip(x, y, w, h)
	if !ok {
		return
	}
	nx0, ny0, nx1, ny1 := d.nativeRect(x0, y0, x1, y1)
	markNative(nx0, ny0, nx1+1, ny1+1)
}

func markNative(x0, y0, x1, y1 int) {
	for my := y0; my < y1; my++ {
		row := dirty[my*(Width/8):]
		for mx := x0; mx < x1; mx++ {
			row[mx>>3] |= 0x80 >> (mx & 7)
		}
	}
}

// Dirty reports whether any pixel changed since it was last sent to the panel.
func (d *Display) Dirty() bool {
	for _, w := range dirtyWords {
		if w != 0 {
			return true
		}
	}
	return false
}

// dirtyBounds returns the native bounding box (exclusive end) of the changed
// pixels, rounded to 32 pixels horizontally.
func dirtyBounds() (x0, y0, x1, y1 int, ok bool) {
	const wpr = Width / 32 // words per row
	k0, k1 := wpr, -1
	y0, y1 = Height, -1
	for y := range Height {
		row := dirtyWords[y*wpr : (y+1)*wpr]
		for k, w := range row {
			if w != 0 {
				k0, k1 = min(k0, k), max(k1, k)
				y0, y1 = min(y0, y), y
			}
		}
	}
	if k1 < 0 {
		return 0, 0, 0, 0, false
	}
	return 32 * k0, y0, 32 * (k1 + 1), y1 + 1, true
}

// Display redraws the whole screen in ModeQuality. It implements
// drivers.Displayer.
func (d *Display) Display() error { return d.Update(ModeQuality) }

// Update redraws the whole screen.
func (d *Display) Update(mode Mode) error {
	d.MarkAllDirty()
	d.full = true
	err := d.updateNative(0, 0, Width, Height, mode)
	d.full = false
	return err
}

// UpdateRect redraws a rectangle (in drawing coordinates).
func (d *Display) UpdateRect(x, y, w, h int16, mode Mode) error {
	x0, y0, x1, y1, ok := d.clip(x, y, w, h)
	if !ok {
		return nil
	}
	nx0, ny0, nx1, ny1 := d.nativeRect(x0, y0, x1, y1)
	markNative(nx0, ny0, nx1+1, ny1+1)
	return d.updateNative(nx0, ny0, nx1+1, ny1+1, mode)
}

// Refresh sends all changed pixels to the panel (and nothing else).
func (d *Display) Refresh(mode Mode) error {
	x0, y0, x1, y1, ok := dirtyBounds()
	if !ok {
		return nil
	}
	return d.updateNative(x0, y0, x1, y1, mode)
}

// Clear sets the frame buffer to white and redraws the whole screen.
func (d *Display) Clear() error {
	d.FillScreen(White)
	return d.Update(ModeText)
}

// updateNative drives the changed pixels in a native rectangle (exclusive
// end) and marks them clean.
func (d *Display) updateNative(x0, y0, x1, y1 int, mode Mode) error {
	d.rw0, d.rw1 = x0/16, (x1+15)/16
	d.ry0, d.ry1 = y0, y1
	err := d.run(func() error {
		switch mode {
		case ModeFast:
			return d.waveform(lutFast, true)
		case ModeFastest:
			return d.waveform(lutFastest, true)
		}
		if err := d.flash(); err != nil {
			return err
		}
		// The pixels are white now: run M5GFX's eraser for old level 15,
		// then the target waveform.
		for _, e := range lutEraser {
			if err := d.frames(allLevels(e>>30&3), 1); err != nil {
				return err
			}
		}
		if mode == ModeText {
			return d.waveform(lutText, false)
		}
		return d.waveform(lutQuality, false)
	})
	// Clean the region, whether or not the update went through: on error
	// the panel is in an unknown state anyway.
	for y := d.ry0; y < d.ry1; y++ {
		row := (*[Width / 16]uint16)(unsafe.Pointer(&dirty[y*(Width/8)]))
		for i := d.rw0; i < d.rw1; i++ {
			row[i] = 0
		}
	}
	return err
}

func (d *Display) waveform(table []uint32, blackWhite bool) error {
	for _, lu := range table {
		if blackWhite {
			lu = bw(lu)
		}
		if err := d.frames(lu, 1); err != nil {
			return err
		}
	}
	return nil
}

// bw makes levels 0-7 follow level 0 (black) and 8-15 follow level 15.
func bw(lu uint32) uint32 {
	return (lu&3)*0x5555 | (lu>>30&3)*0x55550000
}

// flash drives the changed pixels black and white a few times, ending white.
func (d *Display) flash() error {
	for range d.ClearCycles {
		if err := d.frames(allLevels(actBlack), d.ClearFrames); err != nil {
			return err
		}
		if err := d.frames(allLevels(actWhite), d.ClearFrames); err != nil {
			return err
		}
	}
	return nil
}

// run powers the panel for the duration of an update.
func (d *Display) run(f func() error) error {
	start := time.Now()
	d.Frames = 0
	d.powerOn()
	err := f()
	d.powerOff()
	d.UpdateUs = uint32(time.Since(start).Microseconds())
	return err
}

// frames drives n frames with the per-level actions in lu.
func (d *Display) frames(lu uint32, n int) error {
	d.setLUT(lu)
	for range n {
		err := d.frame(d.fill)
		d.Frames++
		if err != nil {
			return err
		}
	}
	return nil
}

// setLUT prepares the lookup tables for a frame with per-level actions lu.
func (d *Display) setLUT(lu uint32) {
	d.uniform = lu == allLevels(lu&3)
	d.ucode = allLevels(lu & 3)
	for v := range 256 {
		hi := lu >> ((v >> 4) * 2) & 3
		lo := lu >> ((v & 15) * 2) & 3
		d.t2[v] = uint8(hi<<2 | lo)
		d.t2h[v] = uint8(hi<<6 | lo<<4)
	}
}

// fillRow builds the bus data for row y: 4 pixels per byte, first pixel in
// the top bits. Only changed pixels inside the update region are driven; all
// others get "no-op" (00).
//
// This is the hot loop: on the ESP32-S3 it should finish before the previous
// row is shifted out (~15 us), so it keeps few values live (the Xtensa
// register window is small) and works on 8 pixels at a time.
func (d *Display) fillRow(y int, line *[lineWords]uint32) {
	// Have the next row's pixels loaded into the cache while this one is
	// being converted (the frame buffer may be in slow PSRAM).
	if next := y + 1; next >= d.ry0 && next < d.ry1 && !d.uniform && d.rw1 > d.rw0 {
		prefetch(unsafe.Pointer(&fbWords[next*(Width/8)+2*d.rw0]), 8*(d.rw1-d.rw0))
	}
	if y < d.ry0 || y >= d.ry1 {
		for i := range rowBytes / 4 {
			line[i] = 0
		}
		return
	}
	rw0, rw1 := d.rw0, d.rw1
	for i := 0; i < rw0; i++ {
		line[i] = 0
	}
	for i := rw1; i < rowBytes/4; i++ {
		line[i] = 0
	}
	// From here on: groups of 8 pixels, [2*rw0, 2*rw1).
	n := 2 * (rw1 - rw0)
	src := unsafe.Pointer(&fbWords[y*(Width/8)+2*rw0])
	dmask := unsafe.Pointer(&dirty[y*(Width/8)+2*rw0])
	dst := unsafe.Add(unsafe.Pointer(line), 4*rw0)
	switch {
	case d.uniform:
		fillUniform(dst, dmask, n, d.ucode)
	case d.full:
		fillLUTFull(dst, src, n, &d.t2, &d.t2h)
	default:
		fillLUT(dst, src, dmask, n, &d.t2, &d.t2h)
	}
}

// fillLUTFull is fillLUT for rows where every pixel is to be driven: 16
// pixels (two frame buffer words, one bus word) per iteration, no branches.
func fillLUTFull(dst, src unsafe.Pointer, n int, t, th *[256]uint8) {
	for range n / 2 {
		a := *(*uint32)(src)
		b := *(*uint32)(unsafe.Add(src, 4))
		*(*uint32)(dst) = uint32(th[a&0xff]) | uint32(t[a>>8&0xff]) |
			(uint32(th[a>>16&0xff])|uint32(t[a>>24]))<<8 |
			(uint32(th[b&0xff])|uint32(t[b>>8&0xff]))<<16 |
			(uint32(th[b>>16&0xff])|uint32(t[b>>24]))<<24
		dst = unsafe.Add(dst, 4)
		src = unsafe.Add(src, 8)
	}
}

// fillLUT converts n groups of 8 pixels (one frame buffer word each) to 2 bus
// bytes each, keeping only pixels whose dirty bit is set.
func fillLUT(dst, src, dmask unsafe.Pointer, n int, t, th *[256]uint8) {
	for range n {
		m := *(*uint8)(dmask)
		var o uint32
		if m != 0 {
			a := *(*uint32)(src)
			o = uint32(th[a&0xff]) | uint32(t[a>>8&0xff]) |
				(uint32(th[a>>16&0xff])|uint32(t[a>>24]))<<8
			if m != 0xff {
				o &= uint32(expand8[m])
			}
		}
		*(*uint16)(dst) = uint16(o)
		dst = unsafe.Add(dst, 2)
		src = unsafe.Add(src, 4)
		dmask = unsafe.Add(dmask, 1)
	}
}

// fillUniform is fillLUT for frames where every level gets the same action:
// code holds that action for 8 pixels.
func fillUniform(dst, dmask unsafe.Pointer, n int, code uint32) {
	c := uint16(code)
	for range n {
		m := *(*uint8)(dmask)
		o := c
		if m != 0xff {
			o &= expand8[m]
		}
		*(*uint16)(dst) = o
		dst = unsafe.Add(dst, 2)
		dmask = unsafe.Add(dmask, 1)
	}
}
