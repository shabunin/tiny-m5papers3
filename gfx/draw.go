// Package gfx draws anti-aliased text and shapes on a 16-level gray canvas
// such as the epd.Display. Shapes are rendered from signed distance
// functions: every pixel's coverage becomes a gray level blended with what is
// already there, which gives smooth edges on the e-paper panel.
package gfx

import "math"

// Canvas is a gray-level drawing surface (0 = black ... 15 = white).
// *epd.Display implements it.
type Canvas interface {
	Size() (w, h int16)
	SetGray(x, y int16, level uint8)
	Gray(x, y int16) uint8
	FillRect(x, y, w, h int16, level uint8)
}

// Gray levels.
const (
	Black uint8 = 0
	Dark  uint8 = 5
	Mid   uint8 = 9
	Light uint8 = 13
	White uint8 = 15
)

// Blend mixes level into the pixel at (x, y) with coverage a (0..15).
func Blend(c Canvas, x, y int, level, a uint8) {
	if a == 0 {
		return
	}
	if a >= 15 {
		c.SetGray(int16(x), int16(y), level)
		return
	}
	bg := int(c.Gray(int16(x), int16(y)))
	v := (bg*(15-int(a)) + int(level)*int(a) + 7) / 15
	c.SetGray(int16(x), int16(y), uint8(v))
}

// Rect fills a rectangle.
func Rect(c Canvas, x, y, w, h int, level uint8) {
	c.FillRect(int16(x), int16(y), int16(w), int16(h), level)
}

// Frame draws a rectangle outline of the given thickness inside the rectangle.
func Frame(c Canvas, x, y, w, h, t int, level uint8) {
	Rect(c, x, y, w, t, level)
	Rect(c, x, y+h-t, w, t, level)
	Rect(c, x, y, t, h, level)
	Rect(c, x+w-t, y, t, h, level)
}

// sdf shades the pixels of a bounding box by a signed distance function
// (negative inside): coverage = clamp(0.5 - d).
func sdf(c Canvas, x0, y0, x1, y1 float32, level uint8, d func(px, py float32) float32) {
	sdfPx(c, int(floor(x0))-1, int(floor(y0))-1, int(ceil(x1))+1, int(ceil(y1))+1, level, d)
}

// sdfPx is sdf for an inclusive pixel box.
func sdfPx(c Canvas, ix0, iy0, ix1, iy1 int, level uint8, d func(px, py float32) float32) {
	w, h := c.Size()
	ix0, iy0 = max(ix0, 0), max(iy0, 0)
	ix1, iy1 = min(ix1, int(w)-1), min(iy1, int(h)-1)
	for y := iy0; y <= iy1; y++ {
		py := float32(y) + 0.5
		for x := ix0; x <= ix1; x++ {
			v := 0.5 - d(float32(x)+0.5, py)
			if v <= 0 {
				continue
			}
			a := uint8(15)
			if v < 1 {
				a = uint8(v*15 + 0.5)
			}
			Blend(c, x, y, level, a)
		}
	}
}

func floor(v float32) float32 { return float32(math.Floor(float64(v))) }
func ceil(v float32) float32  { return float32(math.Ceil(float64(v))) }
func sqrt(v float32) float32  { return float32(math.Sqrt(float64(v))) }
func abs(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
func clamp(v, lo, hi float32) float32 { return min(max(v, lo), hi) }

// FillCircle draws a filled circle (center and radius in pixels; pixel
// centers are at +0.5).
func FillCircle(c Canvas, cx, cy, r float32, level uint8) {
	sdf(c, cx-r, cy-r, cx+r, cy+r, level, func(px, py float32) float32 {
		dx, dy := px-cx, py-cy
		return sqrt(dx*dx+dy*dy) - r
	})
}

// Ring draws a circle outline of width t centered on radius r.
func Ring(c Canvas, cx, cy, r, t float32, level uint8) {
	o := r + t/2
	sdf(c, cx-o, cy-o, cx+o, cy+o, level, func(px, py float32) float32 {
		dx, dy := px-cx, py-cy
		return abs(sqrt(dx*dx+dy*dy)-r) - t/2
	})
}

// Line draws a line of width t with round caps.
func Line(c Canvas, x0, y0, x1, y1, t float32, level uint8) {
	bx, by := x1-x0, y1-y0
	bb := bx*bx + by*by
	r := t / 2
	sdf(c, min(x0, x1)-r, min(y0, y1)-r, max(x0, x1)+r, max(y0, y1)+r, level, func(px, py float32) float32 {
		ax, ay := px-x0, py-y0
		h := float32(0)
		if bb > 0 {
			h = clamp((ax*bx+ay*by)/bb, 0, 1)
		}
		dx, dy := ax-bx*h, ay-by*h
		return sqrt(dx*dx+dy*dy) - r
	})
}

// roundBox is the signed distance to a rounded rectangle.
func roundBox(px, py, cx, cy, hw, hh, r float32) float32 {
	qx := abs(px-cx) - (hw - r)
	qy := abs(py-cy) - (hh - r)
	ox, oy := max(qx, 0), max(qy, 0)
	return sqrt(ox*ox+oy*oy) + min(max(qx, qy), 0) - r
}

// FillRoundRect draws a filled rectangle with corner radius r.
func FillRoundRect(c Canvas, x, y, w, h, r int, level uint8) {
	if r <= 0 {
		Rect(c, x, y, w, h, level)
		return
	}
	fx, fy, fw, fh := float32(x), float32(y), float32(w), float32(h)
	cx, cy, hw, hh, fr := fx+fw/2, fy+fh/2, fw/2, fh/2, float32(r)
	// Interior without the corners: plain fill.
	Rect(c, x+r, y, w-2*r, h, level)
	Rect(c, x, y+r, r, h-2*r, level)
	Rect(c, x+w-r, y+r, r, h-2*r, level)
	for _, k := range [4][2]int{{x, y}, {x + w - r, y}, {x, y + h - r}, {x + w - r, y + h - r}} {
		sdfPx(c, k[0], k[1], k[0]+r-1, k[1]+r-1, level, func(px, py float32) float32 {
			return roundBox(px, py, cx, cy, hw, hh, fr)
		})
	}
}

// RoundRect draws the outline (thickness t, inside the rectangle) of a
// rectangle with corner radius r.
func RoundRect(c Canvas, x, y, w, h, r, t int, level uint8) {
	fx, fy, fw, fh := float32(x), float32(y), float32(w), float32(h)
	cx, cy, hw, hh, fr, ft := fx+fw/2, fy+fh/2, fw/2, fh/2, float32(r), float32(t)
	// Straight parts.
	Rect(c, x+r, y, w-2*r, t, level)
	Rect(c, x+r, y+h-t, w-2*r, t, level)
	Rect(c, x, y+r, t, h-2*r, level)
	Rect(c, x+w-t, y+r, t, h-2*r, level)
	if r <= 0 {
		return
	}
	for _, k := range [4][2]int{{x, y}, {x + w - r, y}, {x, y + h - r}, {x + w - r, y + h - r}} {
		sdfPx(c, k[0], k[1], k[0]+r-1, k[1]+r-1, level, func(px, py float32) float32 {
			d := roundBox(px, py, cx, cy, hw, hh, fr)
			return max(d, -d-ft)
		})
	}
}

// FillTriangle draws a filled triangle.
func FillTriangle(c Canvas, x0, y0, x1, y1, x2, y2 float32, level uint8) {
	// Orient counter-clockwise (in screen coordinates, y down).
	if (x1-x0)*(y2-y0)-(y1-y0)*(x2-x0) < 0 {
		x1, y1, x2, y2 = x2, y2, x1, y1
	}
	edge := func(px, py, ax, ay, bx, by float32) float32 {
		// Signed distance to the line through a and b, positive outside.
		ex, ey := bx-ax, by-ay
		l := sqrt(ex*ex + ey*ey)
		if l == 0 {
			return 0
		}
		return ((px-ax)*ey - (py-ay)*ex) / l
	}
	sdf(c, min(x0, x1, x2), min(y0, y1, y2), max(x0, x1, x2), max(y0, y1, y2), level, func(px, py float32) float32 {
		return max(edge(px, py, x0, y0, x1, y1), edge(px, py, x1, y1, x2, y2), edge(px, py, x2, y2, x0, y0))
	})
}

// bayer4 is a 4x4 ordered dithering matrix.
var bayer4 = [16]uint8{0, 8, 2, 10, 12, 4, 14, 6, 3, 11, 1, 9, 15, 7, 13, 5}

// Dither returns black or white for an 8-bit gray value at (x, y), so that an
// area looks gray even with the black/white fast update modes.
func Dither(x, y int, v uint8) uint8 {
	if int(v) > int(bayer4[(y&3)<<2|x&3])*16+8 {
		return White
	}
	return Black
}

// Gray8 returns one of the 16 levels for an 8-bit gray value at (x, y), with
// ordered dithering between levels.
func Gray8(x, y int, v uint8) uint8 {
	l := (int(v) + int(bayer4[(y&3)<<2|x&3]) - 8) >> 4
	return uint8(min(max(l, 0), 15))
}

// FillDither fills a rectangle with a black/white dither pattern for gray v.
func FillDither(c Canvas, x, y, w, h int, v uint8) {
	for j := y; j < y+h; j++ {
		for i := x; i < x+w; i++ {
			c.SetGray(int16(i), int16(j), Dither(i, j, v))
		}
	}
}
