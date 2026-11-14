package ui

import (
	"strconv"

	"tiny-m5papers3/epd"
	"tiny-m5papers3/fonts"
	"tiny-m5papers3/gfx"
)

// Reader shows a text in pages: tap the right side for the next page, the
// left side for the previous one.
type Reader struct {
	Text  string
	lines []string
	page  int
	turns int
}

const (
	readerMargin = 34
	readerTop    = HeaderH + 28
	readerFooter = 64
	readerGap    = 6 // extra space between lines
)

// Title implements App.
func (r *Reader) Title() string { return "Reader" }

// Icon implements App: an open book.
func (r *Reader) Icon(c gfx.Canvas, cx, cy, size int) {
	w, h := size*42/100, size*60/100
	for _, x := range []int{cx - w - 2, cx + 2} {
		gfx.RoundRect(c, x, cy-h/2, w, h, 6, 3, gfx.Black)
		for i := 0; i < 5; i++ {
			gfx.Rect(c, x+8, cy-h/2+12+i*9, w-16, 3, gfx.Mid)
		}
	}
}

func (r *Reader) lineStep() int { return fonts.Serif30.LineHeight() + readerGap }

func (r *Reader) perPage(s *System) int {
	_, h := s.Size()
	return (h - readerTop - readerFooter) / r.lineStep()
}

func (r *Reader) pages(s *System) int {
	n := r.perPage(s)
	return (len(r.lines) + n - 1) / n
}

// Start implements App.
func (r *Reader) Start(s *System) {
	if r.lines == nil {
		w, _ := s.Size()
		r.lines = Wrap(r.Text, w-2*readerMargin)
	}
	r.draw(s)
}

// Wrap splits text into lines for the reader font; a line starting with "# "
// is a heading.
func Wrap(text string, width int) []string {
	return gfx.Wrap(fonts.Serif30, text, width)
}

func (r *Reader) draw(s *System) {
	d := s.D
	w, h := s.Size()
	gfx.Rect(d, 0, HeaderH, w, h-HeaderH, gfx.White)
	n := r.perPage(s)
	y := readerTop + fonts.Serif30.Ascent()
	for i := r.page * n; i < min((r.page+1)*n, len(r.lines)); i++ {
		line := r.lines[i]
		f := fonts.Serif30
		if len(line) > 2 && line[0] == '#' && line[1] == ' ' {
			line, f = line[2:], fonts.SerifBold36
		}
		gfx.DrawText(d, f, readerMargin, y, line, gfx.Black)
		y += r.lineStep()
	}
	// Footer: page number and a progress bar.
	total := r.pages(s)
	fy := h - readerFooter + 14
	gfx.Rect(d, readerMargin, fy, w-2*readerMargin, 4, gfx.Light)
	gfx.Rect(d, readerMargin, fy, (w-2*readerMargin)*(r.page+1)/total, 4, gfx.Black)
	gfx.DrawTextCentered(d, fonts.Sans22, w/2, fy+34, strconv.Itoa(r.page+1)+" / "+strconv.Itoa(total), gfx.Dark)
}

// Event implements App.
func (r *Reader) Event(s *System, e Event) {
	if e.Kind != Tap {
		return
	}
	w, _ := s.Size()
	p := r.page
	if e.X > w/3 {
		p++
	} else {
		p--
	}
	if p < 0 || p >= r.pages(s) {
		return
	}
	r.page = p
	r.draw(s)
	r.turns++
	if r.turns%6 == 0 {
		s.FullRefresh(epd.ModeText) // clean page every few turns
	} else {
		s.Refresh(epd.ModeText)
	}
}

// Tick implements App.
func (r *Reader) Tick(s *System) {}
