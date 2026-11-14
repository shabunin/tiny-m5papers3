package ui

import (
	"strconv"

	"github.com/shabunin/tiny-m5papers3/epd"
	"github.com/shabunin/tiny-m5papers3/fonts"
	"github.com/shabunin/tiny-m5papers3/gfx"
	"github.com/shabunin/tiny-m5papers3/papers3"
)

// TouchTest shows every finger the GT911 reports (up to 5) with its track
// ID and coordinates, and leaves a trail.
type TouchTest struct {
	prev    [5]papers3.Touch
	nprev   int
	buttons []*Button
	maxSeen int
}

const touchBar = 76

// Title implements App.
func (t *TouchTest) Title() string { return "Touch" }

// Icon implements App: rings around a fingertip.
func (t *TouchTest) Icon(c gfx.Canvas, cx, cy, size int) {
	fx, fy := float32(cx), float32(cy)
	r := float32(size) / 2
	gfx.Ring(c, fx, fy, r*0.9, 2, gfx.Light)
	gfx.Ring(c, fx, fy, r*0.62, 3, gfx.Mid)
	gfx.Ring(c, fx, fy, r*0.35, 4, gfx.Black)
	gfx.FillCircle(c, fx, fy, r*0.14, gfx.Black)
}

// Start implements App.
func (t *TouchTest) Start(s *System) {
	w, h := s.Size()
	t.buttons = Row(w-170, w-12, h-touchBar+12, 52, 0, "Clear")
	t.clear(s)
}

func (t *TouchTest) clear(s *System) {
	w, h := s.Size()
	gfx.Rect(s.D, 0, HeaderH, w, h-HeaderH, gfx.White)
	// Grid every 60 px for orientation.
	for x := 60; x < w; x += 60 {
		gfx.Rect(s.D, x, HeaderH, 1, h-HeaderH-touchBar, gfx.Light)
	}
	for y := HeaderH + 60; y < h-touchBar; y += 60 {
		gfx.Rect(s.D, 0, y, w, 1, gfx.Light)
	}
	gfx.Rect(s.D, 0, h-touchBar, w, 3, gfx.Black)
	DrawAll(s.D, t.buttons)
	t.info(s, "touch with up to 5 fingers")
	t.nprev = 0
}

func (t *TouchTest) info(s *System, msg string) {
	w, h := s.Size()
	gfx.Rect(s.D, 0, h-touchBar+4, w-180, touchBar-4, gfx.White)
	gfx.DrawText(s.D, fonts.Sans22, 12, h-touchBar+32, msg, gfx.Black)
	st := papers3.GetStatus()
	gfx.DrawText(s.D, fonts.Sans18, 12, h-touchBar+60,
		"GT911 fw 0x"+strconv.FormatUint(uint64(st.TouchFirmware), 16)+
			" · cfg "+strconv.Itoa(int(st.TouchConfig))+
			" · max "+strconv.Itoa(t.maxSeen)+" fingers", gfx.Dark)
}

// erase clears a box (clipped to the touch area) and restores the grid.
func (t *TouchTest) erase(s *System, x0, y0, x1, y1 int) {
	w, h := s.Size()
	x0, y0 = max(x0, 0), max(y0, HeaderH)
	x1, y1 = min(x1, w), min(y1, h-touchBar)
	if x0 >= x1 || y0 >= y1 {
		return
	}
	gfx.Rect(s.D, x0, y0, x1-x0, y1-y0, gfx.White)
	for x := max((x0+59)/60*60, 60); x < x1; x += 60 {
		gfx.Rect(s.D, x, y0, 1, y1-y0, gfx.Light)
	}
	for y := HeaderH + max((y0-HeaderH+59)/60*60, 60); y < y1; y += 60 {
		gfx.Rect(s.D, x0, y, x1-x0, 1, gfx.Light)
	}
}

// Event implements App.
func (t *TouchTest) Event(s *System, e Event) {
	if e.Kind == Tap && Hit(t.buttons, e.X, e.Y) == 0 {
		t.clear(s)
		s.Refresh(epd.ModeText)
	}
}

// Tick implements App: draw the fingers whenever they move.
func (t *TouchTest) Tick(s *System) {
	_, h := s.Size()
	cur := s.Touches
	changed := len(cur) != t.nprev
	for i := 0; i < len(cur) && !changed; i++ {
		changed = cur[i] != t.prev[i]
	}
	if !changed {
		return
	}
	// Old markers become trail dots.
	for i := 0; i < t.nprev; i++ {
		p := t.prev[i]
		t.erase(s, int(p.X)-38, int(p.Y)-44, int(p.X)+90, int(p.Y)+38)
	}
	for i := 0; i < t.nprev; i++ {
		p := t.prev[i]
		gfx.FillCircle(s.D, float32(p.X), float32(p.Y), 3, gfx.Dark)
	}
	t.nprev = 0
	msg := "fingers: " + strconv.Itoa(len(cur))
	for i, p := range cur {
		if i >= len(t.prev) || int(p.Y) >= h-touchBar {
			continue
		}
		fx, fy := float32(p.X), float32(p.Y)
		gfx.Ring(s.D, fx, fy, 30, 5, gfx.Black)
		gfx.Line(s.D, fx-34, fy, fx+34, fy, 2, gfx.Black)
		gfx.Line(s.D, fx, fy-34, fx, fy+34, 2, gfx.Black)
		gfx.DrawText(s.D, fonts.SansBold28, int(p.X)+36, int(p.Y)-14, strconv.Itoa(int(p.ID)), gfx.Black)
		t.prev[t.nprev] = p
		t.nprev++
		if i == 0 {
			msg += " · " + strconv.Itoa(int(p.X)) + "," + strconv.Itoa(int(p.Y))
		}
	}
	t.maxSeen = max(t.maxSeen, len(cur))
	t.info(s, msg)
	s.Refresh(epd.ModeFastest)
}
