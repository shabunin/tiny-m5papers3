package ui

import (
	"math"
	"strconv"

	"tiny-m5papers3/epd"
	"tiny-m5papers3/fonts"
	"tiny-m5papers3/gfx"
)

// Clock is an analog clock face with the date, updated every minute.
type Clock struct {
	minute int
}

const (
	clockR  = 230
	clockCY = HeaderH + 40 + clockR
)

// Title implements App.
func (c *Clock) Title() string { return "Clock" }

// Icon implements App.
func (c *Clock) Icon(cv gfx.Canvas, cx, cy, size int) {
	r := float32(size) * 0.46
	fx, fy := float32(cx), float32(cy)
	gfx.Ring(cv, fx, fy, r, 4, gfx.Black)
	for i := 0; i < 12; i++ {
		a := float64(i) * math.Pi / 6
		sx, sy := float32(math.Sin(a)), float32(-math.Cos(a))
		gfx.Line(cv, fx+sx*r*0.78, fy+sy*r*0.78, fx+sx*r*0.9, fy+sy*r*0.9, 2, gfx.Black)
	}
	gfx.Line(cv, fx, fy, fx, fy-r*0.7, 4, gfx.Black)
	gfx.Line(cv, fx, fy, fx+r*0.45, fy+r*0.15, 5, gfx.Black)
	gfx.FillCircle(cv, fx, fy, 5, gfx.Black)
}

// Start implements App.
func (c *Clock) Start(s *System) { c.draw(s) }

func (c *Clock) draw(s *System) {
	d := s.D
	w, h := s.Size()
	t := s.Now
	c.minute = t.Hour()*60 + t.Minute()
	gfx.Rect(d, 0, HeaderH, w, h-HeaderH, gfx.White)
	cx, cy := float32(w/2), float32(clockCY)
	gfx.Ring(d, cx, cy, clockR, 8, gfx.Black)
	for i := 0; i < 60; i++ {
		a := float64(i) * math.Pi / 30
		sx, sy := float32(math.Sin(a)), float32(-math.Cos(a))
		if i%5 == 0 {
			gfx.Line(d, cx+sx*(clockR-42), cy+sy*(clockR-42), cx+sx*(clockR-16), cy+sy*(clockR-16), 7, gfx.Black)
			n := strconv.Itoa(i / 5)
			if i == 0 {
				n = "12"
			}
			f := fonts.SansBold40
			gfx.DrawTextCentered(d, f, int(cx+sx*(clockR-78)), int(cy+sy*(clockR-78))+f.Ascent()*7/20, n, gfx.Black)
		} else {
			gfx.Line(d, cx+sx*(clockR-28), cy+sy*(clockR-28), cx+sx*(clockR-16), cy+sy*(clockR-16), 2, gfx.Dark)
		}
	}
	// Hands.
	hour := (float64(t.Hour()%12) + float64(t.Minute())/60) * math.Pi / 6
	minute := float64(t.Minute()) * math.Pi / 30
	hand := func(a float64, length, width float32) {
		sx, sy := float32(math.Sin(a)), float32(-math.Cos(a))
		gfx.Line(d, cx-sx*24, cy-sy*24, cx+sx*length, cy+sy*length, width, gfx.Black)
	}
	hand(hour, clockR*0.52, 16)
	hand(minute, clockR*0.8, 10)
	gfx.FillCircle(d, cx, cy, 14, gfx.Black)
	gfx.FillCircle(d, cx, cy, 5, gfx.White)

	// Digital time and date below.
	y := clockCY + clockR + 40 + fonts.Digits64.Ascent()
	gfx.DrawTextCentered(d, fonts.Digits64, w/2, y, HHMM(t), gfx.Black)
	gfx.DrawTextCentered(d, fonts.Serif30, w/2, y+52, t.Weekday().String(), gfx.Black)
	gfx.DrawTextCentered(d, fonts.Sans28, w/2, y+92,
		strconv.Itoa(t.Day())+" "+t.Month().String()+" "+strconv.Itoa(t.Year()), gfx.Dark)
}

// Event implements App.
func (c *Clock) Event(s *System, e Event) {}

// Tick implements App.
func (c *Clock) Tick(s *System) {
	if s.Now.Hour()*60+s.Now.Minute() != c.minute {
		c.draw(s)
		s.Refresh(epd.ModeText)
	}
}
