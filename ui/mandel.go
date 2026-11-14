package ui

import (
	"math"
	"strconv"
	"time"

	"github.com/shabunin/tiny-m5papers3/epd"
	"github.com/shabunin/tiny-m5papers3/fonts"
	"github.com/shabunin/tiny-m5papers3/gfx"
)

// Mandel explores the Mandelbrot set in 16 gray levels: tap to zoom in.
type Mandel struct {
	cx, cy  float32 // center
	scale   float32 // units per pixel
	zoom    int
	buttons []*Button
	took    time.Duration
}

const mandelBar = 76 // bottom bar height

// Title implements App.
func (m *Mandel) Title() string { return "Fractal" }

// Icon implements App.
func (m *Mandel) Icon(c gfx.Canvas, cx, cy, size int) {
	n := size * 9 / 10
	x0, y0 := cx-n/2, cy-n/2
	sc := 3.0 / float32(n)
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			nu := mandelNu(-0.65+(float32(x)-float32(n)/2)*sc, (float32(y)-float32(n)/2)*sc, 48)
			c.SetGray(int16(x0+x), int16(y0+y), gfx.Gray8(x, y, mandelShade(nu, 0, 30)))
		}
	}
}

func (m *Mandel) reset() {
	m.cx, m.cy, m.zoom = -0.65, 0, 0
	m.scale = 3.2 / 540
}

// Start implements App.
func (m *Mandel) Start(s *System) {
	if m.scale == 0 {
		m.reset()
	}
	w, h := s.Size()
	m.buttons = Row(250, w-12, h-mandelBar+12, 52, 12, "Out", "Reset")
	m.render(s)
}

func (m *Mandel) render(s *System) {
	w, h := s.Size()
	top, bottom := HeaderH, h-mandelBar
	iter := 64 + 48*m.zoom
	start := time.Now()
	// A coarse pass finds the fastest escape in view, so that the gradient
	// starts there at any zoom level.
	nuMin := float32(iter)
	for y := top; y < bottom; y += 16 {
		ci := m.cy + float32(y-(top+bottom)/2)*m.scale
		for x := 0; x < w; x += 16 {
			if nu := mandelNu(m.cx+float32(x-w/2)*m.scale, ci, iter); nu >= 0 {
				nuMin = min(nuMin, nu)
			}
		}
	}
	span := float32(30 + 6*m.zoom)
	for y := top; y < bottom; y++ {
		ci := m.cy + float32(y-(top+bottom)/2)*m.scale
		for x := 0; x < w; x++ {
			cr := m.cx + float32(x-w/2)*m.scale
			v := mandelShade(mandelNu(cr, ci, iter), nuMin, span)
			s.D.SetGray(int16(x), int16(y), gfx.Gray8(x, y, v))
		}
	}
	m.took = time.Since(start)
	// Bottom bar.
	gfx.Rect(s.D, 0, bottom, w, mandelBar, gfx.White)
	gfx.Rect(s.D, 0, bottom, w, 3, gfx.Black)
	z := "zoom 1"
	if m.zoom > 0 {
		z = "zoom 3^" + strconv.Itoa(m.zoom)
	}
	gfx.DrawText(s.D, fonts.SansBold28, 14, bottom+36, z, gfx.Black)
	gfx.DrawText(s.D, fonts.Sans18, 14, bottom+62, "tap to zoom · "+strconv.Itoa(int(m.took/time.Millisecond))+" ms", gfx.Dark)
	DrawAll(s.D, m.buttons)
}

// mandelShade maps a smooth iteration count to an 8-bit gray: black inside
// the set, a gradient from white (fast escape) to dark (near the set).
func mandelShade(nu, nuMin, span float32) uint8 {
	if nu < 0 {
		return 0
	}
	t := 1 - (nu-nuMin)/span
	t = min(max(t, 0), 1)
	t = t * t * t
	return uint8(30 + 225*t)
}

// mandelNu returns the smooth iteration count at which point c escapes, or
// -1 if it seems to be in the set.
func mandelNu(cr, ci float32, maxIter int) float32 {
	// Main cardioid and period-2 bulb: inside, skip the iteration.
	q := (cr-0.25)*(cr-0.25) + ci*ci
	if q*(q+(cr-0.25)) <= 0.25*ci*ci || (cr+1)*(cr+1)+ci*ci <= 1.0/16 {
		return -1
	}
	var zr, zi float32
	n := 0
	var m float32
	for ; n < maxIter; n++ {
		r2, i2 := zr*zr, zi*zi
		m = r2 + i2
		if m > 256 {
			break
		}
		zi = 2*zr*zi + ci
		zr = r2 - i2 + cr
	}
	if n == maxIter {
		return -1
	}
	return float32(n) + 1 - log2(log2(m)*0.5)
}

// log2 is a fast approximation (float32 bits), good enough for shading.
func log2(x float32) float32 {
	b := math.Float32bits(x)
	e := float32(int(b>>23&0xff) - 127)
	f := math.Float32frombits(b&0x007fffff | 0x3f800000) // 1..2
	return e + (f-1)*(1.8854-0.4427*f)                   // quadratic fit
}

// Event implements App.
func (m *Mandel) Event(s *System, e Event) {
	if e.Kind != Tap {
		return
	}
	w, h := s.Size()
	switch Hit(m.buttons, e.X, e.Y) {
	case 0:
		if m.zoom > 0 {
			m.zoom--
			m.scale *= 3
		}
	case 1:
		m.reset()
	default:
		if e.Y >= h-mandelBar {
			return
		}
		if m.zoom >= 8 { // float32 runs out of precision
			return
		}
		top, bottom := HeaderH, h-mandelBar
		m.cx += float32(e.X-w/2) * m.scale
		m.cy += float32(e.Y-(top+bottom)/2) * m.scale
		m.scale /= 3
		m.zoom++
		// Show where we zoom while computing.
		gfx.Ring(s.D, float32(e.X), float32(e.Y), 40, 6, gfx.Black)
		gfx.Ring(s.D, float32(e.X), float32(e.Y), 46, 4, gfx.White)
		s.Refresh(epd.ModeFastest)
		s.Flush()
	}
	m.render(s)
	s.Refresh(epd.ModeQuality)
}

// Tick implements App.
func (m *Mandel) Tick(s *System) {}
