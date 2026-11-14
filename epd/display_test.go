//go:build !esp32s3

package epd

import (
	"testing"
)

func newTestDisplay(t *testing.T) *Display {
	t.Helper()
	d := New()
	d.Configure()
	if err := d.Update(ModeQuality); err != nil {
		t.Fatal(err)
	}
	if d.Dirty() {
		t.Fatal("dirty after full update")
	}
	d.ResetStats()
	return d
}

// After a full update of a white screen, all ink is white.
func TestFullUpdateWhite(t *testing.T) {
	d := newTestDisplay(t)
	for i, v := range d.Ink {
		if v != 255 {
			t.Fatalf("pixel %d,%d not white: %d", i%Width, i/Width, v)
		}
	}
	if d.Frames != 2*2*8+2+len(lutQuality) {
		t.Errorf("frames = %d", d.Frames)
	}
}

// ModeFast must drive exactly the changed pixels and nothing else.
func TestFastDrivesOnlyChangedPixels(t *testing.T) {
	d := newTestDisplay(t)
	d.SetRotation(Portrait)
	d.FillRect(100, 200, 37, 51, Black) // odd sizes on purpose
	d.SetGray(5, 5, Black)
	d.FillRect(300, 300, 10, 10, White) // no change: already white
	if err := d.Refresh(ModeFast); err != nil {
		t.Fatal(err)
	}
	w, h := d.Size()
	for y := int16(0); y < h; y++ {
		for x := int16(0); x < w; x++ {
			mx, my := d.toNative(int(x), int(y))
			i := my*Width + mx
			want := (x >= 100 && x < 137 && y >= 200 && y < 251) || (x == 5 && y == 5)
			if (d.Driven[i] != 0) != want {
				t.Fatalf("pixel %d,%d driven=%d, want driven=%v", x, y, d.Driven[i], want)
			}
			if want && d.Ink[i] != 0 {
				t.Fatalf("pixel %d,%d ink=%d, want black", x, y, d.Ink[i])
			}
			if !want && d.Ink[i] != 255 {
				t.Fatalf("pixel %d,%d ink=%d, want white", x, y, d.Ink[i])
			}
		}
	}
	if d.Dirty() {
		t.Error("still dirty after refresh")
	}
	// A second refresh has nothing to do.
	d.ResetStats()
	if err := d.Refresh(ModeFast); err != nil {
		t.Fatal(err)
	}
	if d.SimFrames2() != 0 {
		t.Error("refresh without changes drove frames")
	}
}

// SimFrames2 returns the number of frames sent since the last ResetStats.
func (d *Display) SimFrames2() int {
	n := 0
	for _, v := range d.Driven {
		n += int(v)
	}
	return n
}

// Each native pixel must land at its own position on the bus.
func TestBitPacking(t *testing.T) {
	d := newTestDisplay(t)
	d.SetRotation(Landscape) // drawing coordinates == native
	for x := int16(0); x < Width; x++ {
		if x%3 == 0 || x%7 == 0 {
			d.SetGray(x, int16(x)%Height, Black)
		}
	}
	if err := d.Refresh(ModeFastest); err != nil {
		t.Fatal(err)
	}
	for x := 0; x < Width; x++ {
		y := x % Height
		want := x%3 == 0 || x%7 == 0
		if got := d.Ink[y*Width+x] == 0; got != want {
			t.Fatalf("native %d,%d black=%v want %v", x, y, got, want)
		}
	}
}

func TestRotations(t *testing.T) {
	d := New()
	d.Configure()
	cases := []struct {
		r      Rotation
		w, h   int16
		mx, my int // native position of drawing (0,0)
	}{
		{Portrait, 540, 960, 0, Height - 1},
		{Landscape, 960, 540, 0, 0},
		{PortraitFlipped, 540, 960, Width - 1, 0},
		{LandscapeFlipped, 960, 540, Width - 1, Height - 1},
	}
	for _, c := range cases {
		d.SetRotation(c.r)
		if w, h := d.Size(); w != c.w || h != c.h {
			t.Errorf("rotation %d: size %dx%d", c.r, w, h)
		}
		if mx, my := d.toNative(0, 0); mx != c.mx || my != c.my {
			t.Errorf("rotation %d: (0,0) -> native %d,%d", c.r, mx, my)
		}
		// Every drawing pixel maps to a distinct native pixel and back.
		w, h := d.Size()
		for _, p := range [][2]int{{0, 0}, {int(w) - 1, 0}, {0, int(h) - 1}, {13, 77}} {
			mx, my := d.toNative(p[0], p[1])
			if mx < 0 || my < 0 || mx >= Width || my >= Height {
				t.Errorf("rotation %d: %v -> native %d,%d out of range", c.r, p, mx, my)
			}
		}
		// FromPortrait must agree with the Portrait mapping.
		for _, p := range [][2]int16{{0, 0}, {539, 0}, {0, 959}, {123, 456}} {
			x, y := d.FromPortrait(p[0], p[1])
			pmx, pmy := (&Display{rot: Portrait}).toNative(int(p[0]), int(p[1]))
			mx, my := d.toNative(int(x), int(y))
			if mx != pmx || my != pmy {
				t.Errorf("rotation %d: FromPortrait%v = %d,%d -> native %d,%d, want %d,%d",
					c.r, p, x, y, mx, my, pmx, pmy)
			}
		}
	}
}

func TestDirtyBounds(t *testing.T) {
	d := newTestDisplay(t)
	if _, _, _, _, ok := dirtyBounds(); ok {
		t.Fatal("clean display has dirty bounds")
	}
	d.SetRotation(Landscape)
	d.SetGray(40, 10, Black)
	d.SetGray(500, 300, Black)
	x0, y0, x1, y1, ok := dirtyBounds()
	if !ok || x0 > 40 || x1 <= 500 || y0 != 10 || y1 != 301 {
		t.Errorf("bounds %d,%d %d,%d ok=%v", x0, y0, x1, y1, ok)
	}
}

// UpdateRect redraws its area even without changes, and only its area.
func TestUpdateRect(t *testing.T) {
	d := newTestDisplay(t)
	d.SetRotation(Portrait)
	d.SetGray(10, 10, Black) // changed but outside the rect: stays pending
	if err := d.UpdateRect(200, 300, 50, 60, ModeText); err != nil {
		t.Fatal(err)
	}
	mx, my := d.toNative(10, 10)
	if d.Driven[my*Width+mx] != 0 {
		t.Error("pixel outside UpdateRect was driven")
	}
	mx, my = d.toNative(220, 320)
	if d.Driven[my*Width+mx] == 0 {
		t.Error("pixel inside UpdateRect not driven")
	}
	if !d.Dirty() {
		t.Error("pending change outside the rect was lost")
	}
}

func TestGray8Dither(t *testing.T) {
	d := New()
	d.Configure()
	d.SetGray8(0, 0, 0)
	if d.Gray(0, 0) != Black {
		t.Error("black not black")
	}
	d.SetGray8(0, 0, 255)
	if d.Gray(0, 0) != White {
		t.Error("white not white")
	}
	// A mid gray averages to about 8 over a 4x4 block.
	sum := 0
	for y := int16(0); y < 4; y++ {
		for x := int16(0); x < 4; x++ {
			d.SetGray8(x, y, 128)
			sum += int(d.Gray(x, y))
		}
	}
	if sum < 7*16 || sum > 9*16 {
		t.Errorf("dithered 128 averages %d/16", sum)
	}
}
