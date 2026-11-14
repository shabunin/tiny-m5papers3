package gfx_test

import (
	"image/png"
	"os"
	"strings"
	"testing"

	"tiny-m5papers3/epd"
	"tiny-m5papers3/fonts"
	"tiny-m5papers3/gfx"
)

func newDisplay() *epd.Display {
	d := epd.New()
	d.Configure()
	d.SetRotation(epd.Portrait)
	return d
}

// savePNG writes the frame buffer to $GFX_PNG_DIR/name.png when set.
func savePNG(t *testing.T, d *epd.Display, name string) {
	dir := os.Getenv("GFX_PNG_DIR")
	if dir == "" {
		return
	}
	f, err := os.Create(dir + "/" + name + ".png")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, d.FrameBufferImage()); err != nil {
		t.Fatal(err)
	}
}

func TestTextAndShapes(t *testing.T) {
	d := newDisplay()
	y := 60
	for _, f := range []*gfx.Font{fonts.Sans18, fonts.Sans22, fonts.Sans28, fonts.SansBold28, fonts.SansBold40, fonts.Serif30, fonts.SerifBold36} {
		gfx.DrawText(d, f, 20, y, "Quick brown fox 0123 °·–…×", gfx.Black)
		y += f.LineHeight()
	}
	gfx.DrawTextCentered(d, fonts.Digits180, 270, y+150, "12:34", gfx.Black)
	y += 200
	gfx.FillCircle(d, 80, float32(y+60), 50, gfx.Black)
	gfx.Ring(d, 200, float32(y+60), 50, 4, gfx.Black)
	gfx.Line(d, 270, float32(y+10), 380, float32(y+110), 6, gfx.Black)
	gfx.FillTriangle(d, 400, float32(y+110), 520, float32(y+110), 460, float32(y+10), gfx.Dark)
	y += 140
	gfx.FillRoundRect(d, 20, y, 240, 80, 20, gfx.Black)
	gfx.DrawTextCentered(d, fonts.SansBold28, 140, y+50, "Button", gfx.White)
	gfx.RoundRect(d, 280, y, 240, 80, 20, 3, gfx.Black)
	gfx.DrawTextCentered(d, fonts.SansBold28, 400, y+50, "Outline", gfx.Black)
	y += 100
	for i := 0; i < 16; i++ {
		gfx.Rect(d, 20+i*31, y, 31, 40, uint8(i))
	}
	y += 50
	for x := 0; x < 500; x++ {
		for j := 0; j < 30; j++ {
			d.SetGray(int16(20+x), int16(y+j), gfx.Gray8(x, j, uint8(x*255/499)))
			d.SetGray(int16(20+x), int16(y+35+j), gfx.Dither(x, j, uint8(x*255/499)))
		}
	}
	savePNG(t, d, "gfx")

	if w := fonts.Sans28.Width("Hello"); w < 50 || w > 90 {
		t.Errorf("Sans28 width of Hello = %d", w)
	}
	if !fonts.Sans28.Has('a') || fonts.Digits180.Has('a') {
		t.Error("Has")
	}
}

func TestWrap(t *testing.T) {
	f := fonts.Sans28
	text := "The quick brown fox jumps over the lazy dog.\n\nA verylongwordthatcannotbebrokenatspacesatall goes here."
	lines := gfx.Wrap(f, text, 200)
	for _, l := range lines {
		if w := f.Width(l); w > 200 {
			t.Errorf("line %q is %d px wide", l, w)
		}
		if strings.HasPrefix(l, " ") || strings.HasSuffix(l, " ") {
			t.Errorf("line %q has spaces at the ends", l)
		}
	}
	joined := strings.Join(lines, " ")
	for _, w := range []string{"quick", "lazy", "goes", "here."} {
		if !strings.Contains(joined, w) {
			t.Errorf("lost word %q in %q", w, lines)
		}
	}
	if len(lines) < 5 {
		t.Errorf("too few lines: %q", lines)
	}
}
