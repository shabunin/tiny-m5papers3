package ui

import (
	"strconv"

	"tiny-m5papers3/epd"
	"tiny-m5papers3/fonts"
	"tiny-m5papers3/gfx"
	"tiny-m5papers3/papers3"
)

// Home is the launcher: a big clock, the board status and a grid of apps.
type Home struct {
	minute int
	info   string
}

// Title implements App.
func (h *Home) Title() string { return "Home" }

// Icon implements App.
func (h *Home) Icon(c gfx.Canvas, cx, cy, size int) {}

// Grid geometry: 3 columns for up to 9 apps, 4 for more.
const (
	gridTop  = 344
	tileH    = 186
	tileGapX = 10
	tileGapY = 12
	gridLeft = 20
	gridW    = 540 - 2*gridLeft
)

func gridCols(apps int) int {
	if apps > 9 {
		return 4
	}
	return 3
}

// tileW returns the tile width for a grid of apps.
func tileW(apps int) int {
	c := gridCols(apps)
	return (gridW - (c-1)*tileGapX) / c
}

// tileRect returns the top left corner of tile i of apps.
func tileRect(i, apps int) (x, y int) {
	c := gridCols(apps)
	return gridLeft + (i%c)*(tileW(apps)+tileGapX), gridTop + (i/c)*(tileH+tileGapY)
}

// TileCenter returns the center of app i's tile on the Home screen (in
// portrait drawing coordinates), with apps tiles in the grid.
func TileCenter(i, apps int) (x, y int) {
	x, y = tileRect(i, apps)
	return x + tileW(apps)/2, y + tileH/2
}

// Start implements App.
func (h *Home) Start(s *System) {
	h.drawClock(s)
	n := len(s.Apps)
	tw := tileW(n)
	for i, a := range s.Apps {
		x, y := tileRect(i, n)
		gfx.RoundRect(s.D, x, y, tw, tileH, 20, 3, gfx.Black)
		a.Icon(s.D, x+tw/2, y+76, min(96, tw-24))
		f := fonts.SansBold28
		if f.Width(a.Title()) > tw-12 {
			f = fonts.Sans22
		}
		gfx.DrawTextCentered(s.D, f, x+tw/2, y+163, a.Title(), gfx.Black)
	}
}

func (h *Home) drawClock(s *System) {
	d := s.D
	w, _ := s.Size()
	t := s.Now
	h.minute = t.Hour()*60 + t.Minute()
	gfx.Rect(d, 0, HeaderH, w, gridTop-HeaderH-8, gfx.White)
	base := HeaderH + 12 + fonts.Digits180.Ascent()
	gfx.DrawTextCentered(d, fonts.Digits180, w/2, base, HHMM(t), gfx.Black)
	date := t.Weekday().String() + ", " + strconv.Itoa(t.Day()) + " " + t.Month().String() + " " + strconv.Itoa(t.Year())
	gfx.DrawTextCentered(d, fonts.Sans28, w/2, base+52, date, gfx.Black)
	h.info = statusLine()
	gfx.DrawTextCentered(d, fonts.Sans22, w/2, base+92, h.info, gfx.Dark)
}

// statusLine is the board status under the date.
func statusLine() string {
	b := papers3.ReadBattery()
	s := ftoa(papers3.ChipTemperature(), 1) + " °C chip · " + ftoa(b.Volts, 2) + " V"
	if b.Charging {
		s += " · charging"
	}
	return s
}

// Event implements App.
func (h *Home) Event(s *System, e Event) {
	if e.Kind != Tap {
		return
	}
	n := len(s.Apps)
	tw := tileW(n)
	for i, a := range s.Apps {
		x, y := tileRect(i, n)
		if e.X >= x && e.X < x+tw && e.Y >= y && e.Y < y+tileH {
			// Show the touch: invert the tile frame while the app loads.
			gfx.RoundRect(s.D, x+3, y+3, tw-6, tileH-6, 17, 4, gfx.Black)
			s.Refresh(epd.ModeFastest)
			s.Flush()
			s.Open(a)
			return
		}
	}
}

// Tick implements App: once a minute, redraw the clock.
func (h *Home) Tick(s *System) {
	if s.Now.Hour()*60+s.Now.Minute() != h.minute {
		h.drawClock(s)
		s.Refresh(epd.ModeText)
	}
}

// ftoa formats v with the given number of decimals (no fmt on the device).
func ftoa(v float32, decimals int) string {
	neg := v < 0
	if neg {
		v = -v
	}
	scale := 1
	for range decimals {
		scale *= 10
	}
	n := int(v*float32(scale) + 0.5)
	s := strconv.Itoa(n / scale)
	if decimals > 0 {
		f := strconv.Itoa(n % scale)
		for len(f) < decimals {
			f = "0" + f
		}
		s += "." + f
	}
	if neg && n != 0 {
		s = "-" + s
	}
	return s
}
