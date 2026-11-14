package ui

import (
	"strings"
	"testing"
	"time"

	"tiny-m5papers3/epd"
	"tiny-m5papers3/fonts"
	"tiny-m5papers3/papers3"
)

type harness struct {
	t   *testing.T
	sys *System
}

func newHarness(t *testing.T) *harness {
	papers3.Configure()
	papers3.SimTouches = nil
	papers3.SimTime = time.Date(2026, 11, 13, 9, 41, 0, 0, time.UTC)
	h := &harness{t: t, sys: NewLauncher(papers3.Display)}
	h.step()
	return h
}

// step runs one iteration and checks the invariants: every change in the
// frame buffer was sent to the panel, and no update failed.
func (h *harness) step() {
	h.t.Helper()
	h.sys.Step(papers3.Now(), papers3.ReadTouches(nil))
	papers3.SimTime = papers3.SimTime.Add(30 * time.Millisecond)
	if h.sys.D.Dirty() {
		h.t.Fatalf("%s: frame buffer changes not refreshed", h.sys.Current().Title())
	}
	if strings.Contains(h.sys.LastUpdate, "ERROR") {
		h.t.Fatalf("%s: %s", h.sys.Current().Title(), h.sys.LastUpdate)
	}
}

func (h *harness) tap(x, y int) {
	h.t.Helper()
	papers3.SimTouches = []papers3.Touch{{X: int16(x), Y: int16(y)}}
	h.step()
	h.step()
	papers3.SimTouches = nil
	h.step()
}

func (h *harness) drag(x0, y0, x1, y1 int) {
	h.t.Helper()
	for k := 0; k <= 10; k++ {
		papers3.SimTouches = []papers3.Touch{{X: int16(x0 + (x1-x0)*k/10), Y: int16(y0 + (y1-y0)*k/10)}}
		h.step()
	}
	papers3.SimTouches = nil
	h.step()
}

// Every app opens from its tile, survives some interaction, and Home leads
// back to the launcher.
func TestAllApps(t *testing.T) {
	h := newHarness(t)
	if h.sys.Current() != h.sys.Home {
		t.Fatal("not on home")
	}
	for i, a := range h.sys.Apps {
		x, y := tileRect(i, len(h.sys.Apps))
		h.tap(x+tileW(len(h.sys.Apps))/2, y+tileH/2)
		if h.sys.Current() != a {
			t.Fatalf("tile %d opened %v, want %s", i, h.sys.Current().Title(), a.Title())
		}
		// Poke around: taps all over the screen and a swipe.
		for _, p := range [][2]int{{270, 480}, {100, 900}, {450, 900}, {270, 150}, {450, 160}} {
			h.tap(p[0], p[1])
		}
		h.drag(60, 300, 480, 800)
		for range 20 {
			h.step()
		}
		h.tap(60, 30) // Home
		if h.sys.Current() != h.sys.Home {
			// Some apps leave a mode where Home is still the header button.
			t.Fatalf("%s: Home button did not return (now %s)", a.Title(), h.sys.Current().Title())
		}
	}
}

// After a full quality update the simulated ink matches the frame buffer for
// pure black and white pixels.
func TestInkMatches(t *testing.T) {
	h := newHarness(t)
	d := h.sys.D
	fbImg, ink := d.FrameBufferImage(), d.InkImage()
	b := fbImg.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			v := fbImg.GrayAt(x, y).Y
			if (v == 0 || v == 255) && ink.GrayAt(x, y).Y != v {
				t.Fatalf("pixel %d,%d: frame buffer %d, ink %d", x, y, v, ink.GrayAt(x, y).Y)
			}
		}
	}
}

// The clock on the home screen changes with the minute, with a partial
// refresh of only the changed pixels.
func TestHomeClockTick(t *testing.T) {
	h := newHarness(t)
	papers3.SimTime = time.Date(2026, 11, 13, 9, 42, 0, 0, time.UTC)
	h.sys.D.ResetStats()
	h.step()
	if !strings.HasPrefix(h.sys.LastUpdate, "refresh text") {
		t.Errorf("minute tick: %s", h.sys.LastUpdate)
	}
	// Few pixels changed: the "1" became a "2" (and the header).
	n := 0
	for _, v := range h.sys.D.Driven {
		if v != 0 {
			n++
		}
	}
	if n == 0 || n > 540*960/10 {
		t.Errorf("%d pixels driven for a minute change", n)
	}
}

func TestFlip(t *testing.T) {
	h := newHarness(t)
	h.sys.SetFlipped(true)
	if h.sys.D.Rotation() != epd.PortraitFlipped {
		t.Fatal("not flipped")
	}
	h.step()
	// Physical top-left is now the bottom-right of the drawing: the Home
	// screen's System tile (bottom right) is under the finger at the
	// physical top-left area.
	x, y := tileRect(len(h.sys.Apps)-1, len(h.sys.Apps))
	h.tap(539-(x+tileW(len(h.sys.Apps))/2), 959-(y+tileH/2))
	if h.sys.Current().Title() != "System" {
		t.Errorf("flipped tap opened %s", h.sys.Current().Title())
	}
	h.sys.SetFlipped(false)
}

// Local time from network time: EET with EU summer time.
func TestTimeZone(t *testing.T) {
	z := TimeZone{Offset: 2 * time.Hour, EUSummer: true}
	for _, c := range []struct{ utc, local string }{
		{"2026-11-14T10:30:00Z", "2026-11-14T12:30:00Z"},
		{"2026-07-01T12:00:00Z", "2026-07-01T15:00:00Z"},
		{"2026-03-29T00:59:59Z", "2026-03-29T02:59:59Z"}, // last Sunday in March
		{"2026-03-29T01:00:00Z", "2026-03-29T04:00:00Z"},
		{"2026-10-25T00:59:59Z", "2026-10-25T03:59:59Z"}, // last Sunday in October
		{"2026-10-25T01:00:00Z", "2026-10-25T03:00:00Z"},
	} {
		utc, _ := time.Parse(time.RFC3339, c.utc)
		if got := z.Local(utc).Format(time.RFC3339); got != c.local {
			t.Errorf("Local(%s) = %s, want %s", c.utc, got, c.local)
		}
	}
}

// The WiFi app scans when it opens and lists the networks; Set clock sets
// the RTC from network time in local time.
func TestWiFiApp(t *testing.T) {
	h := newHarness(t)
	wf := h.sys.Find("WiFi").(*WiFi)
	h.sys.Open(wf)
	for i := 0; i < 20 && wf.job.busy(); i++ {
		time.Sleep(time.Millisecond)
		h.step()
	}
	if len(wf.nets) == 0 || !strings.Contains(wf.msg, "networks") {
		t.Fatalf("after scan: %d networks, message %q", len(wf.nets), wf.msg)
	}
	for i := 1; i < len(wf.nets); i++ {
		if wf.nets[i].RSSI > wf.nets[i-1].RSSI || (wf.nets[i].SSID != "" && wf.nets[i].SSID == wf.nets[i-1].SSID) {
			t.Fatalf("networks not sorted/deduplicated: %v", wf.nets)
		}
	}
	WiFiSSID = "Home"
	defer func() { WiFiSSID = "" }()
	b := wf.buttons[1]
	h.tap(b.X+b.W/2, b.Y+b.H/2)
	for i := 0; i < 20 && wf.job.busy(); i++ {
		time.Sleep(time.Millisecond)
		h.step()
	}
	if got := papers3.Now().Format("15:04"); got != "12:30" || !strings.HasPrefix(wf.msg, "Clock set") {
		t.Fatalf("after Set clock: time %s, message %q", got, wf.msg)
	}
}

// On battery, after a while without touches, the board dozes until the next
// minute (at most DozeMax); not while touched or with a busy app, nor with a
// computer on the USB port.
func TestDoze(t *testing.T) {
	h := newHarness(t)
	defer func() { papers3.SimUSBHost = true }()
	papers3.SimUSBHost = false
	if h.sys.Doze() {
		t.Fatal("dozed right after start")
	}
	papers3.SimTime = papers3.SimTime.Add(DozeAfter)
	h.step()
	t0 := papers3.Now()
	if !h.sys.Doze() {
		t.Fatal("no doze when idle on battery")
	}
	if d := papers3.Now().Sub(t0); d <= 0 || d > DozeMax {
		t.Fatalf("dozed %v", d)
	}
	if h.sys.Dozed <= 0 {
		t.Fatal("Dozed not counted")
	}
	// Up to the minute: the clock gets its update.
	papers3.SimTime = time.Date(2026, 11, 13, 9, 59, 59, 500e6, time.UTC)
	h.step()
	if !h.sys.Doze() || papers3.Now().Minute() != 0 || papers3.Now().Second() != 0 {
		t.Fatalf("dozed until %v, want just after 10:00", papers3.Now())
	}
	h.step()
	if !strings.HasPrefix(h.sys.LastUpdate, "refresh text") {
		t.Errorf("no clock update after dozing: %s", h.sys.LastUpdate)
	}
	papers3.SimUSBHost = true
	if h.sys.Doze() {
		t.Error("dozed with a USB host")
	}
	papers3.SimUSBHost = false
	h.sys.Open(h.sys.Find("Level"))
	papers3.SimTime = papers3.SimTime.Add(DozeAfter)
	h.step()
	if h.sys.Doze() {
		t.Error("dozed with the spirit level open")
	}
	papers3.SimTouches = []papers3.Touch{{X: 270, Y: 600}}
	h.sys.Open(h.sys.Home)
	h.step()
	if h.sys.Doze() {
		t.Error("dozed while touched")
	}
	papers3.SimTouches = nil
}

func TestFit(t *testing.T) {
	f := fonts.Sans28
	if got := Fit(f, "Home", 200); got != "Home" {
		t.Errorf("short text changed: %q", got)
	}
	long := "A very long network name that does not fit"
	got := Fit(f, long, 200)
	if !strings.HasSuffix(got, "…") || f.Width(got) > 200 || len(got) < 5 {
		t.Errorf("Fit(long) = %q, %d px", got, f.Width(got))
	}
	if got := Fit(f, "Кафе 5G", 300); got != "Кафе 5G" || f.Width(got) != f.Width("???? 5G") {
		t.Errorf("missing glyphs: %q, %d px", got, f.Width(got))
	}
}
