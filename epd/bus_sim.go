//go:build !esp32s3

package epd

import (
	"errors"
	"image"
	"image/color"
	"time"
	"unsafe"
)

var errNotPowered = errors.New("epd: frame sent while panel power is off")

// InkStep is how far one frame moves the simulated ink (0 = black, 255 =
// white). Real ink is non-linear; this is only good enough to see where and
// in which direction pixels were driven.
var InkStep = 64

// Timing knobs exist on the hardware; kept for API compatibility.
var (
	RowGapUs   uint32 = 3
	FrameGapUs uint32 = 1000
)

type bus struct {
	lines   [2][lineWords]uint32
	powered bool
	start   time.Time

	DryRun    bool
	LateRows  int
	FillMaxUs uint32

	// Ink is the simulated panel content in native orientation.
	Ink []uint8
	// Driven counts, per pixel, the frames that drove it (black or white)
	// since the last ResetStats.
	Driven []uint16
	// SimFrames counts frames sent.
	SimFrames int
}

func (b *bus) ResetStats() {
	b.LateRows, b.FillMaxUs = 0, 0
	for i := range b.Driven {
		b.Driven[i] = 0
	}
}

func (b *bus) init() {
	b.start = time.Now()
	b.Ink = make([]uint8, Width*Height)
	b.Driven = make([]uint16, Width*Height)
	for i := range b.Ink {
		b.Ink[i] = 160 // unknown state: light gray
	}
}

func (b *bus) powerOn()  { b.powered = true }
func (b *bus) powerOff() { b.powered = false }

// Micros returns a microsecond counter.
func (b *bus) Micros() uint32 { return uint32(time.Since(b.start).Microseconds()) }

// DebugRegs does nothing in the simulation.
func (b *bus) DebugRegs() {}

func (b *bus) frame(fill func(y int, line *[lineWords]uint32)) error {
	if !b.powered {
		return errNotPowered
	}
	b.SimFrames++
	for y := 0; y < Height; y++ {
		line := &b.lines[y&1]
		for i := range line {
			line[i] = 0xdeadbeef // fill must overwrite all row bytes
		}
		fill(y, line)
		for j := 0; j < rowBytes; j++ {
			v := byte(line[j/4] >> (8 * (j % 4))) // little endian, as GDMA reads it
			for k := 0; k < 4; k++ {
				code := v >> (6 - 2*k) & 3
				i := y*Width + 4*j + k
				switch code {
				case actBlack:
					b.Driven[i]++
					if !b.DryRun {
						b.Ink[i] = uint8(max(int(b.Ink[i])-InkStep, 0))
					}
				case actWhite:
					b.Driven[i]++
					if !b.DryRun {
						b.Ink[i] = uint8(min(int(b.Ink[i])+InkStep, 255))
					}
				}
			}
		}
	}
	return nil
}

// InkImage returns the simulated panel content in the current drawing
// orientation.
func (d *Display) InkImage() *image.Gray {
	return d.nativeImage(func(mx, my int) uint8 { return d.Ink[my*Width+mx] })
}

// FrameBufferImage returns the frame buffer (what should be on screen after
// the next update) in the current drawing orientation.
func (d *Display) FrameBufferImage() *image.Gray {
	return d.nativeImage(func(mx, my int) uint8 {
		v := fb[my*(Width/2)+mx>>1]
		if mx&1 == 0 {
			v >>= 4
		}
		return (v & 15) * 17
	})
}

// DrivenImage shows which pixels were driven since the last ResetStats:
// white = untouched, darker = more frames.
func (d *Display) DrivenImage() *image.Gray {
	return d.nativeImage(func(mx, my int) uint8 {
		n := d.Driven[my*Width+mx]
		return uint8(255 - min(int(n)*8, 255))
	})
}

func (d *Display) nativeImage(px func(mx, my int) uint8) *image.Gray {
	w, h := d.Size()
	img := image.NewGray(image.Rect(0, 0, int(w), int(h)))
	for y := 0; y < int(h); y++ {
		for x := 0; x < int(w); x++ {
			mx, my := d.toNative(x, y)
			img.SetGray(x, y, color.Gray{px(mx, my)})
		}
	}
	return img
}

// allocBuffers returns heap memory for the frame buffer and dirty map.
func allocBuffers() (*[Width * Height / 8]uint32, *[Width * Height / 32]uint32) {
	return new([Width * Height / 8]uint32), new([Width * Height / 32]uint32)
}

// prefetch does nothing on other platforms.
func prefetch(p unsafe.Pointer, n int) {}
