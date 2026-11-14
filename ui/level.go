package ui

import (
	"math"
	"time"

	"tiny-m5papers3/epd"
	"tiny-m5papers3/fonts"
	"tiny-m5papers3/gfx"
	"tiny-m5papers3/papers3"
)

// Level is a bubble level using the BMI270 accelerometer.
type Level struct {
	fx, fy, fz  float32 // filtered acceleration (g)
	bx, by      int     // bubble position as drawn
	ax, ay      int     // angles as drawn (0.1 degree)
	last        time.Time
	init        bool
	ok          bool
	sharpenedAt time.Time
	movedAt     time.Time
}

const (
	levelR   = 210 // radius of the dial
	levelCY  = HeaderH + 40 + levelR
	bubbleR  = 34
	levelMax = levelR - bubbleR - 6
)

// Title implements App.
func (l *Level) Title() string { return "Level" }

// Icon implements App.
func (l *Level) Icon(c gfx.Canvas, cx, cy, size int) {
	r := float32(size) * 0.45
	fx, fy := float32(cx), float32(cy)
	gfx.Ring(c, fx, fy, r, 4, gfx.Black)
	gfx.Line(c, fx-r, fy, fx+r, fy, 2, gfx.Mid)
	gfx.Line(c, fx, fy-r, fx, fy+r, 2, gfx.Mid)
	gfx.Ring(c, fx, fy, r*0.3, 2, gfx.Mid)
	gfx.FillCircle(c, fx+r*0.35, fy-r*0.2, r*0.26, gfx.Black)
}

// Start implements App.
func (l *Level) Start(s *System) {
	l.init = false
	l.ax, l.ay = 9999, 9999
	l.sample()
	l.draw(s)
	w, h := s.Size()
	gfx.DrawTextCentered(s.D, fonts.Sans22, w/2, h-40, "lay the device on a surface", gfx.Mid)
}

func (l *Level) sample() {
	x, y, z, ok := papers3.ReadAccel()
	l.ok = ok
	if !ok {
		return
	}
	if !l.init {
		l.fx, l.fy, l.fz, l.init = x, y, z, true
		return
	}
	const k = 0.3 // low-pass
	l.fx += (x - l.fx) * k
	l.fy += (y - l.fy) * k
	l.fz += (z - l.fz) * k
}

// angles returns the tilt around the screen's two axes in 0.1 degree. The
// BMI270's X axis runs along the short side of the panel, Y along the long
// side (portrait), as on other M5Stack devices.
func (l *Level) angles() (ax, ay int) {
	deg := func(a, b float32) int {
		return int(math.Round(math.Atan2(float64(a), float64(b)) * 1800 / math.Pi))
	}
	z := float32(math.Sqrt(float64(l.fz*l.fz))) + 1e-6
	return deg(l.fx, z), deg(l.fy, z)
}

func (l *Level) draw(s *System) {
	d := s.D
	w, _ := s.Size()
	cx := w / 2
	ax, ay := l.angles()
	// Bubble: moves towards the high side, like a real bubble.
	bx := cx - int(float32(levelMax)*clampf(l.fx/0.5, -1, 1))
	by := levelCY + int(float32(levelMax)*clampf(l.fy/0.5, -1, 1))
	if s.Flipped() {
		bx, by = 2*cx-bx, 2*levelCY-by
	}
	l.bx, l.by, l.ax, l.ay = bx, by, ax, ay

	gfx.Rect(d, 0, HeaderH, w, 2*levelR+300, gfx.White)
	fcx, fcy := float32(cx), float32(levelCY)
	gfx.Ring(d, fcx, fcy, levelR, 5, gfx.Black)
	gfx.Ring(d, fcx, fcy, levelR*2/3, 2, gfx.Light)
	gfx.Ring(d, fcx, fcy, levelR/3, 2, gfx.Light)
	gfx.Line(d, fcx-levelR, fcy, fcx+levelR, fcy, 2, gfx.Light)
	gfx.Line(d, fcx, fcy-levelR, fcx, fcy+levelR, 2, gfx.Light)
	gfx.Ring(d, fcx, fcy, bubbleR+4, 3, gfx.Black) // target
	level := abs(ax) <= 5 && abs(ay) <= 5
	if l.ok {
		gfx.FillCircle(d, float32(bx), float32(by), bubbleR, gfx.Black)
		if level {
			gfx.FillCircle(d, float32(bx), float32(by), bubbleR-10, gfx.White)
		}
	}
	y := levelCY + levelR + 110
	if !l.ok {
		gfx.DrawTextCentered(d, fonts.SansBold28, cx, y, "IMU not found", gfx.Black)
		return
	}
	gfx.DrawTextCentered(d, fonts.Digits64, cx/2, y, tenths(ax)+"°", gfx.Black)
	gfx.DrawTextCentered(d, fonts.Digits64, cx+cx/2, y, tenths(ay)+"°", gfx.Black)
	gfx.DrawTextCentered(d, fonts.Sans22, cx/2, y+36, "left/right", gfx.Dark)
	gfx.DrawTextCentered(d, fonts.Sans22, cx+cx/2, y+36, "up/down", gfx.Dark)
	if level {
		gfx.DrawTextCentered(d, fonts.SansBold28, cx, y+90, "LEVEL", gfx.Black)
	}
}

// tenths formats a value in tenths with a sign: "+1.5", "-0.3", "0.0".
func tenths(v int) string {
	sign := "+"
	if v < 0 {
		sign, v = "-", -v
	} else if v == 0 {
		sign = ""
	}
	return sign + ftoa(float32(v)/10, 1)
}

func clampf(v, lo, hi float32) float32 { return min(max(v, lo), hi) }

// Event implements App.
func (l *Level) Event(s *System, e Event) {}

// Busy implements Busy: the bubble follows the board all the time.
func (l *Level) Busy() bool { return true }

// Tick implements App: sample at 20 Hz, redraw when something visibly
// changed, and do a clean update once the bubble rests.
func (l *Level) Tick(s *System) {
	if s.Now.Sub(l.last) < 50*time.Millisecond {
		return
	}
	l.last = s.Now
	l.sample()
	w, _ := s.Size()
	cx := w / 2
	ax, ay := l.angles()
	bx := cx - int(float32(levelMax)*clampf(l.fx/0.5, -1, 1))
	by := levelCY + int(float32(levelMax)*clampf(l.fy/0.5, -1, 1))
	if s.Flipped() {
		bx, by = 2*cx-bx, 2*levelCY-by
	}
	if abs(bx-l.bx) >= 3 || abs(by-l.by) >= 3 || abs(ax-l.ax) >= 3 || abs(ay-l.ay) >= 3 {
		l.draw(s)
		s.Refresh(epd.ModeFastest)
		l.movedAt = s.Now
		return
	}
	// At rest for 2 s after moving: one quality pass of the dial area.
	if l.movedAt.After(l.sharpenedAt) && s.Now.Sub(l.movedAt) > 2*time.Second {
		l.sharpenedAt = s.Now
		w, _ := s.Size()
		s.D.MarkDirty(0, HeaderH, int16(w), int16(2*levelR+300))
		s.Refresh(epd.ModeText)
	}
}
