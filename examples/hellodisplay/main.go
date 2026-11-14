// hellodisplay is the smallest display + touch program (see README).
package main

import (
	"time"

	"github.com/shabunin/tiny-m5papers3/epd"
	"github.com/shabunin/tiny-m5papers3/fonts"
	"github.com/shabunin/tiny-m5papers3/gfx"
	"github.com/shabunin/tiny-m5papers3/papers3"
)

func main() {
	papers3.Configure()
	d := papers3.Display // 540x960 portrait, 0 = black ... 15 = white
	gfx.DrawText(d, fonts.SansBold40, 40, 200, "Hello, PaperS3!", gfx.Black)
	gfx.FillCircle(d, 270, 500, 120, gfx.Mid)
	d.Update(epd.ModeQuality)
	for {
		for _, t := range papers3.ReadTouches(nil) {
			gfx.FillCircle(d, float32(t.X), float32(t.Y), 6, gfx.Black)
		}
		d.Refresh(epd.ModeFast)
		time.Sleep(20 * time.Millisecond)
	}
}
