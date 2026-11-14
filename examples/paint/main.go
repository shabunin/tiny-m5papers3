//go:build esp32s3

// paint: finger painting on the M5PaperS3.
//
// Strokes are drawn with fast black/white partial updates; tap CLEAR in the
// toolbar for a clean full-quality redraw.
package main

import (
	"image/color"
	"machine"
	"time"

	"tiny-m5papers3/epd"
	"tiny-m5papers3/gt911"

	"tinygo.org/x/tinyfont"
	"tinygo.org/x/tinyfont/freesans"
)

const (
	barH   = 80  // toolbar height
	clearX = 400 // CLEAR button starts here
	brush  = 3   // brush radius
)

var black = color.RGBA{A: 255}

type rect struct{ x0, y0, x1, y1 int16 }

func (r *rect) empty() bool { return r.x1 < r.x0 }

func (r *rect) add(x, y int16) {
	if r.empty() {
		*r = rect{x, y, x, y}
		return
	}
	r.x0, r.y0 = min(r.x0, x), min(r.y0, y)
	r.x1, r.y1 = max(r.x1, x), max(r.y1, y)
}

var noRect = rect{0, 0, -1, -1}

func main() {
	time.Sleep(time.Second)

	d := epd.New()
	d.Configure()
	d.SetRotation(epd.Portrait)

	bus := machine.I2C0
	bus.Configure(machine.I2CConfig{SDA: machine.GPIO41, SCL: machine.GPIO42, Frequency: 400 * machine.KHz})
	ts := gt911.New(bus)
	if err := ts.Configure(); err != nil {
		println("touch:", err.Error())
	}

	drawUI(d)
	if err := d.Update(epd.ModeText); err != nil {
		println("display:", err.Error())
	}
	println("paint: ready; touch at 0x", ts.Address, "; first refresh", d.UpdateUs/1000, "ms")

	type pos struct {
		x, y int16
		down bool
	}
	var last [16]pos // by touch track ID
	dirty := noRect
	clearArmed := true

	for {
		pts, err := ts.Read()
		if err != nil {
			println("touch:", err.Error())
		}
		var seen [16]bool
		for _, p := range pts {
			x, y := d.FromPortrait(p.X, p.Y)
			id := p.ID & 15
			seen[id] = true
			if y < barH {
				if x >= clearX && clearArmed {
					clearArmed = false
					d.FillScreen(epd.White)
					drawUI(d)
					d.Update(epd.ModeText)
					dirty = noRect
				}
				last[id].down = false
				continue
			}
			l := last[id]
			switch {
			case !l.down:
				stamp(d, x, y, &dirty)
			case l.x != x || l.y != y:
				line(d, l.x, l.y, x, y, &dirty)
			}
			last[id] = pos{x, y, true}
		}
		for i := range last {
			if !seen[i] {
				last[i].down = false
			}
		}
		if len(pts) == 0 {
			clearArmed = true
		}

		if !dirty.empty() {
			r := dirty
			dirty = noRect
			d.UpdateRect(r.x0-brush, r.y0-brush, r.x1-r.x0+2*brush+1, r.y1-r.y0+2*brush+1, epd.ModeFast)
			println("stroke", r.x0, r.y0, r.x1, r.y1, "update", d.UpdateUs/1000, "ms")
		} else {
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func drawUI(d *epd.Display) {
	w, _ := d.Size()
	tinyfont.WriteLine(d, &freesans.Bold18pt7b, 16, 52, "TinyGo Paint", black)
	// CLEAR button
	d.FillRect(clearX, 12, w-clearX-10, 3, epd.Black)
	d.FillRect(clearX, 65, w-clearX-10, 3, epd.Black)
	d.FillRect(clearX, 12, 3, 56, epd.Black)
	d.FillRect(w-13, 12, 3, 56, epd.Black)
	tinyfont.WriteLine(d, &freesans.Regular12pt7b, clearX+24, 48, "CLEAR", black)
	d.FillRect(0, barH-3, w, 3, epd.Black)
}

// stamp draws a filled circle of radius brush.
func stamp(d *epd.Display, x, y int16, dirty *rect) {
	for dy := int16(-brush); dy <= brush; dy++ {
		for dx := int16(-brush); dx <= brush; dx++ {
			if dx*dx+dy*dy <= brush*brush+1 && y+dy >= barH {
				d.SetGray(x+dx, y+dy, epd.Black)
			}
		}
	}
	dirty.add(x, y)
}

// line draws a thick line (Bresenham, stamping the brush).
func line(d *epd.Display, x0, y0, x1, y1 int16, dirty *rect) {
	dx, sx := x1-x0, int16(1)
	if dx < 0 {
		dx, sx = -dx, -1
	}
	dy, sy := y1-y0, int16(1)
	if dy < 0 {
		dy, sy = -dy, -1
	}
	err := dx - dy
	for {
		stamp(d, x0, y0, dirty)
		if x0 == x1 && y0 == y1 {
			return
		}
		e2 := 2 * err
		if e2 > -dy {
			err -= dy
			x0 += sx
		}
		if e2 < dx {
			err += dx
			y0 += sy
		}
	}
}
