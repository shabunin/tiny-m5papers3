package ui

import (
	"strings"

	"github.com/shabunin/tiny-m5papers3/fonts"
	"github.com/shabunin/tiny-m5papers3/gfx"
)

// Button is a rounded push button.
type Button struct {
	X, Y, W, H int
	Label      string
	// On draws the button filled (selected/active state).
	On bool
}

// Draw draws the button.
func (b *Button) Draw(c gfx.Canvas) {
	gfx.Rect(c, b.X, b.Y, b.W, b.H, gfx.White)
	r := min(b.H/2, 22)
	fg := gfx.Black
	if b.On {
		gfx.FillRoundRect(c, b.X, b.Y, b.W, b.H, r, gfx.Black)
		fg = gfx.White
	} else {
		gfx.RoundRect(c, b.X, b.Y, b.W, b.H, r, 3, gfx.Black)
	}
	f := fonts.SansBold28
	if f.Width(b.Label) > b.W-16 {
		f = fonts.Sans22
	}
	gfx.DrawTextCentered(c, f, b.X+b.W/2, b.Y+b.H/2+f.Ascent()*7/20, b.Label, fg)
}

// Hit reports whether (x, y) is on the button (with a little slack).
func (b *Button) Hit(x, y int) bool {
	return x >= b.X-6 && x < b.X+b.W+6 && y >= b.Y-6 && y < b.Y+b.H+6
}

// Row lays out buttons in a row from x0 to x1 at y, with equal widths.
func Row(x0, x1, y, h, gap int, labels ...string) []*Button {
	n := len(labels)
	w := (x1 - x0 - gap*(n-1)) / n
	bs := make([]*Button, n)
	for i, l := range labels {
		bs[i] = &Button{X: x0 + i*(w+gap), Y: y, W: w, H: h, Label: l}
	}
	return bs
}

// Hit returns the index of the button at (x, y), or -1.
func Hit(bs []*Button, x, y int) int {
	for i, b := range bs {
		if b.Hit(x, y) {
			return i
		}
	}
	return -1
}

// DrawAll draws all buttons.
func DrawAll(c gfx.Canvas, bs []*Button) {
	for _, b := range bs {
		b.Draw(c)
	}
}

// Label clears a box and draws one line of text in it (baseline near the
// bottom); returns the box for a later refresh.
func Label(c gfx.Canvas, f *gfx.Font, x, y, w int, text string, level uint8) {
	gfx.Rect(c, x, y, w, f.LineHeight(), gfx.White)
	gfx.DrawText(c, f, x, y+f.Ascent(), text, level)
}

// Fit returns text shortened with "…" so that it is at most w pixels wide
// in font f. (Characters that f lacks are drawn as "?".)
func Fit(f *gfx.Font, text string, w int) string {
	if f.Width(text) <= w {
		return text
	}
	rs := []rune(text)
	for n := len(rs) - 1; n > 0; n-- {
		s := strings.TrimRight(string(rs[:n]), " ") + "…"
		if f.Width(s) <= w {
			return s
		}
	}
	return "…"
}
