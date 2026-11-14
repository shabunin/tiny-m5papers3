package ui

import (
	"tiny-m5papers3/epd"
	"tiny-m5papers3/fonts"
	"tiny-m5papers3/gfx"
)

// Paint is finger painting with fast partial updates.
type Paint struct {
	actions []*Button // Clear, Sharpen
	tools   []*Button // 3 pen sizes, 3 inks
	size    int       // 0..2
	ink     int       // 0 black, 1 gray, 2 white
	lx, ly  int
	drawing bool
}

const paintTop = HeaderH + 136 // first canvas row

var penRadius = [3]int{2, 5, 12}

// Title implements App.
func (p *Paint) Title() string { return "Paint" }

// Icon implements App.
func (p *Paint) Icon(c gfx.Canvas, cx, cy, size int) {
	s := float32(size) / 100
	fx, fy := float32(cx), float32(cy)
	pts := [][2]float32{{-40, 20}, {-25, -15}, {-8, 18}, {8, -20}, {24, 14}, {40, -22}}
	for i := 1; i < len(pts); i++ {
		gfx.Line(c, fx+pts[i-1][0]*s, fy+pts[i-1][1]*s, fx+pts[i][0]*s, fy+pts[i][1]*s, 9*s, gfx.Black)
	}
	gfx.FillCircle(c, fx+30*s, fy+34*s, 9*s, gfx.Mid)
}

// Start implements App.
func (p *Paint) Start(s *System) {
	w, _ := s.Size()
	p.actions = Row(12, w-12, HeaderH+10, 52, 12, "Clear", "Sharpen")
	p.tools = Row(12, w-12, HeaderH+72, 52, 8, "", "", "", "", "", "")
	DrawAll(s.D, p.actions)
	p.drawTools(s)
	gfx.Rect(s.D, 0, paintTop-4, w, 2, gfx.Mid)
	gfx.DrawTextCentered(s.D, fonts.Sans22, w/2, paintTop+40, "draw with your finger", gfx.Light)
}

func (p *Paint) drawTools(s *System) {
	d := s.D
	for i, b := range p.tools {
		sel := (i < 3 && i == p.size) || (i >= 3 && i-3 == p.ink)
		gfx.Rect(d, b.X, b.Y, b.W, b.H, gfx.White)
		t := 2
		if sel {
			t = 6
		}
		gfx.RoundRect(d, b.X, b.Y, b.W, b.H, 14, t, gfx.Black)
		cx, cy := b.X+b.W/2, b.Y+b.H/2
		switch {
		case i < 3:
			gfx.FillCircle(d, float32(cx), float32(cy), float32(penRadius[i])+0.5, gfx.Black)
		default:
			for y := cy - 13; y < cy+13; y++ {
				for x := cx - 13; x < cx+13; x++ {
					d.SetGray(int16(x), int16(y), inkAt(i-3, x, y))
				}
			}
			gfx.Frame(d, cx-14, cy-14, 28, 28, 2, gfx.Black)
		}
	}
}

func inkAt(ink, x, y int) uint8 {
	switch ink {
	case 0:
		return gfx.Black
	case 1:
		return gfx.Dither(x, y, 128)
	}
	return gfx.White
}

// stamp draws one round pen dab.
func (p *Paint) stamp(s *System, cx, cy int) {
	r := penRadius[p.size]
	w, h := s.Size()
	for y := max(cy-r, paintTop); y <= min(cy+r, h-1); y++ {
		for x := max(cx-r, 0); x <= min(cx+r, w-1); x++ {
			dx, dy := x-cx, y-cy
			if dx*dx+dy*dy <= r*r+r {
				s.D.SetGray(int16(x), int16(y), inkAt(p.ink, x, y))
			}
		}
	}
}

// stroke draws from the last position to (x, y).
func (p *Paint) stroke(s *System, x, y int) {
	step := max(penRadius[p.size]/2, 1)
	n := max(abs(x-p.lx), abs(y-p.ly))/step + 1
	for i := 1; i <= n; i++ {
		p.stamp(s, p.lx+(x-p.lx)*i/n, p.ly+(y-p.ly)*i/n)
	}
	p.lx, p.ly = x, y
}

// Event implements App.
func (p *Paint) Event(s *System, e Event) {
	switch e.Kind {
	case Down:
		if e.Y >= paintTop {
			p.drawing = true
			p.lx, p.ly = e.X, e.Y
			p.stamp(s, e.X, e.Y)
			s.Refresh(epd.ModeFast)
		}
	case Move:
		if p.drawing {
			p.stroke(s, e.X, max(e.Y, paintTop))
			s.Refresh(epd.ModeFast)
		}
	case Up:
		p.drawing = false
	case Tap:
		if e.Y >= paintTop {
			return
		}
		switch Hit(p.actions, e.X, e.Y) {
		case 0: // Clear: white canvas, flash only what was drawn
			w, h := s.Size()
			gfx.Rect(s.D, 0, paintTop, w, h-paintTop, gfx.White)
			s.Refresh(epd.ModeText)
			return
		case 1: // Sharpen: full update removes ghosting, shows the grays
			s.FullRefresh(epd.ModeQuality)
			return
		}
		if i := Hit(p.tools, e.X, e.Y); i >= 0 {
			if i < 3 {
				p.size = i
			} else {
				p.ink = i - 3
			}
			p.drawTools(s)
			s.Refresh(epd.ModeFast)
		}
	}
}

// Tick implements App.
func (p *Paint) Tick(s *System) {}
