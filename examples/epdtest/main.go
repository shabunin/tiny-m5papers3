// epdtest brings up the M5PaperS3 e-paper display.
//
//  1. Dry run: generates a few frames with the panel supply OFF and reports
//     the bus timing (the panel is not driven).
//  2. If that worked: draws a test pattern (border, corner marker, text and
//     16 gray bars) with a full-quality update.
//
// Results are printed over USB serial every few seconds.
package main

import (
	"image/color"
	"strconv"
	"time"

	"github.com/shabunin/tiny-m5papers3/epd"

	"tinygo.org/x/tinyfont"
	"tinygo.org/x/tinyfont/freesans"
)

var black = color.RGBA{0, 0, 0, 255}

func main() {
	time.Sleep(3 * time.Second)
	println("epdtest: configuring")

	d := epd.New()
	d.Configure()

	// 1. Dry run, panel supply stays off.
	d.DryRun = true
	d.ResetStats()
	err := d.Update(epd.ModeFastest)
	dry := "dry run: " + stats(d, err)
	println(dry)
	d.DebugRegs()
	d.DryRun = false

	real, fast := "skipped (dry run failed)", ""
	if err == nil {
		drawPattern(d)
		d.ResetStats()
		err = d.Update(epd.ModeQuality)
		real = "quality update: " + stats(d, err)

		// 2. A small fast update: a black box in the bottom right corner.
		d.FillRect(440, 860, 60, 60, epd.Black)
		d.ResetStats()
		err = d.Refresh(epd.ModeFast)
		fast = "fast update: " + stats(d, err)
	}

	for {
		println(dry)
		println(real)
		println(fast)
		time.Sleep(3 * time.Second)
	}
}

func stats(d *epd.Display, err error) string {
	s := "frames=" + strconv.Itoa(d.Frames) +
		" total=" + strconv.Itoa(int(d.UpdateUs/1000)) + "ms" +
		" per frame=" + strconv.Itoa(int(d.UpdateUs)/max(d.Frames, 1)) + "us" +
		" late rows=" + strconv.Itoa(d.LateRows) +
		" max fill=" + strconv.Itoa(int(d.FillMaxUs)) + "us"
	if err != nil {
		s += " ERROR: " + err.Error()
	}
	return s
}

func drawPattern(d *epd.Display) {
	d.SetRotation(epd.Portrait)
	w, h := d.Size()
	d.FillScreen(epd.White)

	// 3 px border
	d.FillRect(0, 0, w, 3, epd.Black)
	d.FillRect(0, h-3, w, 3, epd.Black)
	d.FillRect(0, 0, 3, h, epd.Black)
	d.FillRect(w-3, 0, 3, h, epd.Black)

	// marker in the (0,0) corner
	d.FillRect(0, 0, 60, 60, epd.Black)
	tinyfont.WriteLine(d, &freesans.Regular12pt7b, 70, 30, "<- (0,0) corner", black)

	tinyfont.WriteLine(d, &freesans.Bold18pt7b, 40, 110, "TinyGo on M5PaperS3", black)

	// 16 gray bars, black (0) at the top to white (15) at the bottom
	const top, bh = 150, 44
	for i := 0; i < 16; i++ {
		y := int16(top + i*bh)
		tinyfont.WriteLine(d, &freesans.Regular12pt7b, 20, y+32, strconv.Itoa(i), black)
		d.FillRect(70, y, 400, bh-4, uint8(i))
	}
	tinyfont.WriteLine(d, &freesans.Regular12pt7b, 140, h-30, "bottom edge", black)
}
