package ui

import (
	"strconv"

	"github.com/shabunin/tiny-m5papers3/epd"
	"github.com/shabunin/tiny-m5papers3/fonts"
	"github.com/shabunin/tiny-m5papers3/gfx"
)

// Life is Conway's Game of Life on a torus, animated with the fastest
// waveform: only the cells that change are driven.
type Life struct {
	cols, rows int
	cells      []uint8 // 1 = alive
	next       []uint8
	running    bool
	gen        int
	runGens    int // generations since Run
	buttons    []*Button
	rng        uint32
	paint      uint8 // value painted by the current drag
}

const (
	lifeCell = 10
	lifeBar  = 76
	lifeTop  = HeaderH + 4
	// lifeMaxRun stops a run after this many generations (the panel is
	// not meant to animate for hours).
	lifeMaxRun = 500
)

// Title implements App.
func (l *Life) Title() string { return "Life" }

// Icon implements App: a glider.
func (l *Life) Icon(c gfx.Canvas, cx, cy, size int) {
	cell := size / 4
	for _, p := range [][2]int{{1, 0}, {2, 1}, {0, 2}, {1, 2}, {2, 2}} {
		gfx.FillRoundRect(c, cx-3*cell/2+p[0]*cell+2, cy-3*cell/2+p[1]*cell+2, cell-4, cell-4, 4, gfx.Black)
	}
	gfx.Frame(c, cx-3*cell/2-4, cy-3*cell/2-4, 3*cell+8, 3*cell+8, 2, gfx.Mid)
}

func (l *Life) random() uint32 {
	// xorshift32
	l.rng ^= l.rng << 13
	l.rng ^= l.rng >> 17
	l.rng ^= l.rng << 5
	return l.rng
}

// Start implements App.
func (l *Life) Start(s *System) {
	w, h := s.Size()
	if l.cells == nil {
		l.cols, l.rows = w/lifeCell, (h-lifeTop-lifeBar)/lifeCell
		l.cells = make([]uint8, l.cols*l.rows)
		l.next = make([]uint8, l.cols*l.rows)
		l.rng = uint32(s.Now.UnixNano()) | 1
		l.randomize()
	}
	l.running = false
	l.buttons = Row(124, w-10, h-lifeBar+12, 52, 8, "Run", "Step", "Random", "Clear")
	for i := range l.cells {
		l.drawCell(s, i)
	}
	l.drawBar(s)
}

func (l *Life) randomize() {
	for i := range l.cells {
		l.cells[i] = 0
		if l.random()%100 < 28 {
			l.cells[i] = 1
		}
	}
	l.gen = 0
}

func (l *Life) drawCell(s *System, i int) {
	x, y := (i%l.cols)*lifeCell, lifeTop+(i/l.cols)*lifeCell
	v := gfx.White
	if l.cells[i] != 0 {
		v = gfx.Black
	}
	gfx.Rect(s.D, x+1, y+1, lifeCell-2, lifeCell-2, v)
}

func (l *Life) drawBar(s *System) {
	w, h := s.Size()
	y := h - lifeBar
	gfx.Rect(s.D, 0, y-2, w, 3, gfx.Black)
	l.buttons[0].Label = "Run"
	if l.running {
		l.buttons[0].Label = "Pause"
	}
	l.buttons[0].On = l.running
	DrawAll(s.D, l.buttons)
	gfx.Rect(s.D, 0, y+4, 118, lifeBar-4, gfx.White)
	gfx.DrawText(s.D, fonts.Sans18, 12, y+30, "generation", gfx.Dark)
	gfx.DrawText(s.D, fonts.SansBold28, 12, y+62, strconv.Itoa(l.gen), gfx.Black)
}

// step computes one generation and draws the changed cells.
func (l *Life) step(s *System) {
	cols, rows := l.cols, l.rows
	for y := 0; y < rows; y++ {
		up, down := (y+rows-1)%rows*cols, (y+1)%rows*cols
		row := y * cols
		for x := 0; x < cols; x++ {
			left, right := (x+cols-1)%cols, (x+1)%cols
			n := l.cells[up+left] + l.cells[up+x] + l.cells[up+right] +
				l.cells[row+left] + l.cells[row+right] +
				l.cells[down+left] + l.cells[down+x] + l.cells[down+right]
			alive := l.cells[row+x]
			if n == 3 || (n == 2 && alive != 0) {
				l.next[row+x] = 1
			} else {
				l.next[row+x] = 0
			}
		}
	}
	l.cells, l.next = l.next, l.cells
	for i := range l.cells {
		if l.cells[i] != l.next[i] {
			l.drawCell(s, i)
		}
	}
	l.gen++
}

func (l *Life) cellAt(x, y int) int {
	cx, cy := x/lifeCell, (y-lifeTop)/lifeCell
	if y < lifeTop || cx < 0 || cx >= l.cols || cy < 0 || cy >= l.rows {
		return -1
	}
	return cy*l.cols + cx
}

// Event implements App.
func (l *Life) Event(s *System, e Event) {
	switch e.Kind {
	case Down:
		if i := l.cellAt(e.X, e.Y); i >= 0 && !l.running {
			l.paint = 1 - l.cells[i]
			l.cells[i] = l.paint
			l.drawCell(s, i)
			s.Refresh(epd.ModeFastest)
		}
	case Move:
		if i := l.cellAt(e.X, e.Y); i >= 0 && !l.running && l.cellAt(e.X0, e.Y0) >= 0 {
			l.cells[i] = l.paint
			l.drawCell(s, i)
			s.Refresh(epd.ModeFastest)
		}
	case Tap:
		switch Hit(l.buttons, e.X, e.Y) {
		case 0:
			l.setRunning(s, !l.running)
		case 1:
			l.setRunning(s, false)
			l.step(s)
			l.drawBar(s)
			s.Refresh(epd.ModeFastest)
		case 2:
			l.randomize()
			l.redraw(s)
		case 3:
			for i := range l.cells {
				l.cells[i] = 0
			}
			l.gen = 0
			l.redraw(s)
		}
	}
}

func (l *Life) redraw(s *System) {
	l.running = false
	for i := range l.cells {
		l.drawCell(s, i)
	}
	l.drawBar(s)
	s.Refresh(epd.ModeText)
}

func (l *Life) setRunning(s *System, on bool) {
	l.running = on
	l.runGens = 0
	l.drawBar(s)
	if on {
		s.Refresh(epd.ModeFast)
		return
	}
	// Paused: one clean update of the whole grid against ghosting.
	w, h := s.Size()
	s.D.MarkDirty(0, lifeTop, int16(w), int16(h-lifeTop))
	s.Refresh(epd.ModeText)
}

// Busy implements Busy: no dozing while the simulation runs.
func (l *Life) Busy() bool { return l.running }

// Tick implements App.
func (l *Life) Tick(s *System) {
	if !l.running {
		return
	}
	l.step(s)
	l.runGens++
	if l.runGens%10 == 0 {
		l.drawBar(s)
	}
	s.Refresh(epd.ModeFastest)
	if l.runGens >= lifeMaxRun {
		l.setRunning(s, false)
	}
}
