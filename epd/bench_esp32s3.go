//go:build esp32s3

package epd

import (
	"runtime/interrupt"
	"unsafe"
)

var benchSink uint32

// BenchFill measures (in CPU cycles) the row fill loop running from flash,
// the same loop running from IRAM, and plain loads of one row.
func (d *Display) BenchFill() (flash, iram, loads uint32) {
	d.rw0, d.rw1, d.ry0, d.ry1 = 0, rowBytes/4, 0, Height
	d.MarkAllDirty()
	d.setLUT(lutQuality[6]) // a frame with different actions per level
	d.full = true
	defer func() { d.full = false }()
	st := interrupt.Disable()
	for range 2 { // second pass: warm caches
		t0 := ccount()
		d.fillRow(10, &d.lines[0])
		t1 := ccount()
		fillIRAM(&d.t2, 10, &d.lines[1])
		t2 := ccount()
		var s uint32
		for i := range Width / 8 {
			s += fbWords[10*(Width/8)+i]
		}
		t3 := ccount()
		benchSink += s
		flash, iram, loads = t1-t0, t2-t1, t3-t2
	}
	interrupt.Restore(st)
	return
}

//go:section .iram1.epdbench
//go:noinline
func fillIRAM(t *[256]uint8, y int, line *[lineWords]uint32) {
	src := unsafe.Pointer(&fbWords[y*(Width/8)])
	drow := unsafe.Pointer(&dirty[y*(Width/8)])
	lp := unsafe.Pointer(line)
	for i := 0; i < rowBytes/4; i++ {
		dm := *(*uint16)(unsafe.Add(drow, 2*i))
		var out uint32
		if dm != 0 {
			a := *(*uint32)(unsafe.Add(src, 8*i))
			b := *(*uint32)(unsafe.Add(src, 8*i+4))
			out = uint32(t[a&0xff])<<4 | uint32(t[a>>8&0xff]) |
				uint32(t[a>>16&0xff])<<12 | uint32(t[a>>24])<<8 |
				uint32(t[b&0xff])<<20 | uint32(t[b>>8&0xff])<<16 |
				uint32(t[b>>16&0xff])<<28 | uint32(t[b>>24])<<24
			if dm != 0xffff {
				out &= uint32(expand8[dm&0xff]) | uint32(expand8[dm>>8])<<16
			}
		}
		*(*uint32)(unsafe.Add(lp, 4*i)) = out
	}
}
