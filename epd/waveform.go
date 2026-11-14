package epd

// Waveform tables, taken from M5GFX's Panel_EPD (LovyanGFX, FreeBSD license,
// https://github.com/m5stack/M5GFX; see LICENSE-LovyanGFX).
//
// Each uint32 is one frame. It holds a 2-bit action for each of the 16 target
// gray levels (level 0 = black in bits 0-1 ... level 15 = white in bits
// 30-31). Actions are also the raw 2-bit codes sent on the bus:
//
//	1 = drive towards black, 2 = drive towards white, 0/3 = leave alone
const (
	actNone  = 0
	actBlack = 1
	actWhite = 2
	actNop   = 3
)

func lut(d ...uint32) uint32 {
	var v uint32
	for i, a := range d {
		v |= a << (2 * i)
	}
	return v
}

// allLevels returns a frame that applies the same action to every level.
func allLevels(act uint32) uint32 { return act * 0x55555555 }

// lutEraser pushes every pixel from its *old* level towards mid gray. It is
// indexed by the previous level, not the target.
var lutEraser = []uint32{
	lut(2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 3, 3, 3, 1, 1),
	lut(2, 2, 3, 3, 3, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1),
}

// lutQuality: full 16-level grayscale, flashing.
var lutQuality = []uint32{
	lut(1, 1, 1, 1, 1, 1, 1, 2, 1, 2, 2, 1, 1, 1, 1, 1),
	lut(2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2),
	lut(2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2),
	lut(1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1),
	lut(1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1),
	lut(1, 1, 1, 1, 1, 1, 1, 1, 2, 1, 1, 1, 1, 1, 1, 1),
	lut(1, 1, 2, 2, 1, 1, 1, 2, 1, 2, 1, 1, 1, 1, 1, 3),
	lut(1, 1, 1, 1, 1, 2, 1, 1, 2, 2, 1, 2, 1, 2, 2, 2),
	lut(1, 1, 3, 2, 2, 1, 2, 2, 2, 2, 2, 2, 1, 1, 2, 2),
	lut(3, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2),
	lut(3, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2),
	lut(1, 1, 1, 1, 3, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2),
	lut(1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 3, 3, 2, 2, 2, 2),
	lut(3, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 3),
	lut(3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 2, 2, 2, 3),
}

// lutText: fewer gray levels, less flashing.
var lutText = []uint32{
	lut(2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 1),
	lut(2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 1),
	lut(2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 1),
	lut(2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 1, 2, 2, 2, 1),
	lut(2, 2, 2, 2, 1, 2, 2, 2, 2, 2, 2, 1, 2, 2, 2, 1),
	lut(1, 2, 2, 1, 1, 1, 1, 1, 3, 3, 1, 1, 3, 3, 1, 2),
	lut(1, 3, 3, 1, 1, 1, 1, 3, 3, 1, 1, 1, 1, 3, 1, 2),
	lut(1, 3, 3, 1, 2, 2, 1, 1, 1, 1, 2, 1, 1, 1, 1, 2),
	lut(3, 1, 3, 2, 2, 2, 1, 1, 1, 2, 2, 1, 1, 1, 2, 3),
	lut(1, 1, 1, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2),
	lut(1, 1, 1, 1, 1, 1, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2),
	lut(1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 3, 3, 3, 3, 2),
}

// lutFast: black and white only (levels 0 and 15); other levels untouched.
var lutFast = []uint32{
	lut(2, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 1),
	lut(2, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 1),
	lut(1, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 2),
	lut(1, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 2),
	lut(1, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 2),
	lut(1, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 2),
	lut(1, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 2),
	lut(1, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 2),
}

// lutFastest: like lutFast with fewer frames.
var lutFastest = []uint32{
	lut(2, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 1),
	lut(1, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 2),
	lut(1, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 2),
	lut(1, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 2),
	lut(1, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 2),
}
