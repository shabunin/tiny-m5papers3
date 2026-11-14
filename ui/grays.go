package ui

import (
	"strconv"

	"tiny-m5papers3/epd"
	"tiny-m5papers3/fonts"
	"tiny-m5papers3/gfx"
)

// Grays is a display test: the 16 gray levels, dithered gradients, text and
// shapes, and buttons to redraw it all with each waveform.
type Grays struct {
	buttons []*Button
}

// Title implements App.
func (g *Grays) Title() string { return "Grays" }

// Icon implements App.
func (g *Grays) Icon(c gfx.Canvas, cx, cy, size int) {
	cell := size / 4
	x0, y0 := cx-2*cell, cy-2*cell
	for i := 0; i < 16; i++ {
		gfx.Rect(c, x0+(i%4)*cell, y0+(i/4)*cell, cell, cell, uint8(i))
	}
	gfx.Frame(c, x0-2, y0-2, 4*cell+4, 4*cell+4, 2, gfx.Black)
}

// Start implements App.
func (g *Grays) Start(s *System) {
	d := s.D
	w, h := s.Size()
	y := HeaderH + 16
	// 16 levels, with numbers.
	bw := (w - 24) / 8
	for i := 0; i < 16; i++ {
		x := 12 + (i%8)*bw
		yy := y + (i/8)*76
		gfx.Rect(d, x, yy, bw, 70, uint8(i))
		fg := gfx.White
		if i >= 8 {
			fg = gfx.Black
		}
		gfx.DrawTextCentered(d, fonts.SansBold28, x+bw/2, yy+46, strconv.Itoa(i), fg)
	}
	gfx.Frame(d, 11, y-1, 8*bw+2, 2*76-4, 1, gfx.Black)
	y += 2*76 + 12

	// Gradients: 16 levels + ordered dithering, and pure black/white dither.
	gfx.DrawText(d, fonts.Sans22, 12, y+20, "dithered 16 levels", gfx.Black)
	y += 28
	for x := 0; x < w-24; x++ {
		v := uint8(x * 255 / (w - 25))
		for j := 0; j < 44; j++ {
			d.SetGray(int16(12+x), int16(y+j), gfx.Gray8(x, j, v))
		}
	}
	y += 52
	gfx.DrawText(d, fonts.Sans22, 12, y+20, "black and white only", gfx.Black)
	y += 28
	for x := 0; x < w-24; x++ {
		v := uint8(x * 255 / (w - 25))
		for j := 0; j < 44; j++ {
			d.SetGray(int16(12+x), int16(y+j), gfx.Dither(x, j, v))
		}
	}
	y += 60

	// Anti-aliased text and shapes.
	gfx.DrawText(d, fonts.Serif30, 12, y+30, "Anti-aliased text, 16 grays", gfx.Black)
	gfx.DrawText(d, fonts.Sans18, 12, y+60, "Small print stays readable: 0123456789 ABCDEFGHIJ abcdefghij", gfx.Black)
	y += 76
	for i := 0; i < 5; i++ {
		cx := float32(60 + i*105)
		gfx.FillCircle(d, cx, float32(y+50), 44-float32(i)*6, uint8(i*3))
		gfx.Ring(d, cx, float32(y+50), 46, 2, gfx.Black)
	}
	y += 110
	for i := 0; i < 12; i++ {
		gfx.Line(d, float32(12+i*44), float32(y), float32(56+i*44), float32(y+70), 1+float32(i)/2, gfx.Black)
	}
	y += 82

	g.buttons = Row(12, w-12, h-70, 52, 8, "Quality", "Text", "Fast", "Fastest")
	DrawAll(d, g.buttons)
	g.status(s)
}

func (g *Grays) status(s *System) {
	w, h := s.Size()
	gfx.Rect(s.D, 0, h-108, w, 36, gfx.White)
	gfx.DrawTextCentered(s.D, fonts.Sans18, w/2, h-84, "last: "+s.LastUpdate, gfx.Dark)
}

// Event implements App: redraw the whole screen with the chosen waveform.
func (g *Grays) Event(s *System, e Event) {
	if e.Kind != Tap {
		return
	}
	if i := Hit(g.buttons, e.X, e.Y); i >= 0 {
		s.FullRefresh(epd.Mode(i))
		s.Flush()
		g.status(s)
		s.Refresh(epd.ModeFast)
	}
}

// Tick implements App.
func (g *Grays) Tick(s *System) {}
