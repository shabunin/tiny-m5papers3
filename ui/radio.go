package ui

import (
	"math"
	"strconv"
	"time"

	"tiny-m5papers3/epd"
	"tiny-m5papers3/fonts"
	"tiny-m5papers3/gfx"
	"tiny-m5papers3/papers3"
	"tiny-m5papers3/radio"
)

// WiFi network for "Set clock" in the WiFi app. The launcher sets them,
// e.g. from tinygo build -ldflags "-X main.wifiSSID=... -X main.wifiPassword=...".
var (
	WiFiSSID     string
	WiFiPassword string
)

// Zone converts network time (UTC) to the local time the RTC keeps.
var Zone = TimeZone{Offset: 2 * time.Hour, EUSummer: true}

// Once the radio is on, setting the time (after NTP, for example) must not
// move time.Now, which the radio drivers use as their clock: see
// papers3.SteadyClock.
func init() { papers3.SteadyClock = radio.Active }

// TimeZone is a fixed offset from UTC, optionally with EU summer time.
type TimeZone struct {
	Offset time.Duration // standard time, east of UTC
	// EUSummer adds an hour from the last Sunday in March to the last
	// Sunday in October (01:00 UTC both), as in the European Union.
	EUSummer bool
}

// Local converts t to local wall-clock time, labelled UTC like the RTC.
func (z TimeZone) Local(t time.Time) time.Time {
	t = t.UTC()
	l := t.Add(z.Offset)
	if z.EUSummer && euSummer(t) {
		l = l.Add(time.Hour)
	}
	return l
}

func euSummer(utc time.Time) bool {
	y := utc.Year()
	start := lastSunday(y, time.March).Add(time.Hour)
	end := lastSunday(y, time.October).Add(time.Hour)
	return !utc.Before(start) && utc.Before(end)
}

func lastSunday(y int, m time.Month) time.Time {
	last := time.Date(y, m+1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, -1)
	return last.AddDate(0, 0, -int(last.Weekday()))
}

// Layout of the radio apps.
const (
	radioMsgY = HeaderH + 44
	radioTop  = HeaderH + 72
	radioRow  = 54
	radioBtnH = 54
	radioBtnY = 70 // from the bottom
	radioPadX = 16
)

// radioJob runs a slow radio call in the background; the app polls it in
// Tick.
type radioJob struct {
	what string // shown while it runs, "" when idle
	done chan func()
}

// radioWork runs all radio jobs, one after the other, on one goroutine that
// lives on. The radio drivers leave pointers into the stack of the goroutine
// that called them in their memory arena, which TinyGo's conservative GC
// scans: with a new goroutine per job, the dead goroutines' stacks stayed
// allocated (measured: 12 KB more heap in use after each scan, out of memory
// after a few).
var radioWork chan func()

func (j *radioJob) start(what string, work func() func()) {
	if radioWork == nil {
		radioWork = make(chan func(), 4)
		go func() {
			for w := range radioWork {
				w()
			}
		}()
	}
	j.what = what
	done := make(chan func(), 1)
	j.done = done
	radioWork <- func() { done <- work() }
}

// poll runs the job's result function once it is done; it reports whether
// that happened.
func (j *radioJob) poll() bool {
	if j.done == nil {
		return false
	}
	select {
	case apply := <-j.done:
		j.done = nil
		j.what = ""
		apply()
		return true
	default:
		return false
	}
}

func (j *radioJob) busy() bool { return j.done != nil }

// drawBars draws a 4-bar signal meter with its bottom left corner at (x, y).
func drawBars(c gfx.Canvas, x, y, bars int) {
	for i := range 4 {
		h := 8 + 6*i
		level := gfx.Light
		if i < bars {
			level = gfx.Black
		}
		gfx.Rect(c, x+i*10, y-h, 7, h, level)
	}
}

// arc draws a circular arc of width t from angle a0 to a1 (radians, 0 =
// up, clockwise).
func arc(c gfx.Canvas, cx, cy, r, t, a0, a1 float32) {
	const steps = 16
	px, py := cx+r*float32(math.Sin(float64(a0))), cy-r*float32(math.Cos(float64(a0)))
	for i := 1; i <= steps; i++ {
		a := float64(a0 + (a1-a0)*float32(i)/steps)
		x, y := cx+r*float32(math.Sin(a)), cy-r*float32(math.Cos(a))
		gfx.Line(c, px, py, x, y, t, gfx.Black)
		px, py = x, y
	}
}

// radioMessage draws the status line of a radio app.
func radioMessage(s *System, msg string) {
	w, _ := s.Size()
	gfx.Rect(s.D, 0, radioMsgY-30, w, 40, gfx.White)
	gfx.DrawText(s.D, fonts.Sans22, radioPadX, radioMsgY, msg, gfx.Dark)
}

// drawRadioRow draws the name and signal strength of a list row with its
// baseline at y (the area is white).
func drawRadioRow(s *System, y int, name string, level uint8, rssi int) {
	w, _ := s.Size()
	dbm := strconv.Itoa(rssi) + " dBm"
	x := radioPadX + 56
	right := w - radioPadX - fonts.Sans22.Width(dbm) - 12
	gfx.DrawText(s.D, fonts.Sans28, x, y, Fit(fonts.Sans28, name, right-x), level)
	gfx.DrawTextRight(s.D, fonts.Sans22, w-radioPadX, y, dbm, gfx.Mid)
}

// radioRows returns how many list rows fit above the buttons.
func radioRows(s *System) int {
	_, h := s.Size()
	return (h - radioBtnY - 16 - radioTop) / radioRow
}

// WiFi lists the WiFi networks around, and sets the clock from the
// internet (NTP) if WiFiSSID is set.
type WiFi struct {
	buttons []*Button
	job     radioJob
	nets    []radio.Network
	msg     string
}

// Title implements App.
func (wf *WiFi) Title() string { return "WiFi" }

// Icon implements App: the usual three arcs and a dot.
func (wf *WiFi) Icon(c gfx.Canvas, cx, cy, size int) {
	fx, fy := float32(cx), float32(cy)+float32(size)*0.32
	s := float32(size)
	for i := 1; i <= 3; i++ {
		arc(c, fx, fy, s*0.22*float32(i), s*0.09, -0.8, 0.8)
	}
	gfx.FillCircle(c, fx, fy, s*0.08, gfx.Black)
}

// Start implements App.
func (wf *WiFi) Start(s *System) {
	w, h := s.Size()
	clock := "Set clock"
	if WiFiSSID == "" {
		clock = "Set clock (no network)"
	}
	wf.buttons = Row(radioPadX, w-radioPadX, h-radioBtnY, radioBtnH, 14, "Scan", clock)
	if wf.nets == nil && !wf.job.busy() {
		wf.scan(s)
	}
	wf.draw(s)
}

func (wf *WiFi) scan(s *System) {
	wf.msg = "Scanning…"
	wf.job.start(wf.msg, func() func() {
		nets, err := radio.ScanWiFi()
		return func() {
			if err != nil {
				wf.msg = "Scan failed: " + err.Error()
				return
			}
			wf.nets = nets
			wf.msg = strconv.Itoa(len(nets)) + " networks · " + HHMM(papers3.Now())
		}
	})
}

func (wf *WiFi) setClock(s *System) {
	if WiFiSSID == "" {
		wf.msg = "No network: build with -ldflags \"-X main.wifiSSID=…\""
		return
	}
	wf.msg = "Connecting to " + WiFiSSID + "…"
	wf.job.start(wf.msg, func() func() {
		utc, err := radio.NetworkTime(WiFiSSID, WiFiPassword)
		got := time.Now()
		return func() {
			if err != nil {
				wf.msg = "No network time: " + err.Error()
				return
			}
			// This runs a little later, after a display update maybe.
			local := Zone.Local(utc.Add(time.Since(got)))
			if err := papers3.SetTime(local); err != nil {
				wf.msg = "Clock not set: " + err.Error()
				return
			}
			wf.msg = "Clock set to " + local.Format("15:04:05") + " · " + radio.Address()
		}
	})
}

func (wf *WiFi) draw(s *System) {
	w, h := s.Size()
	d := s.D
	gfx.Rect(d, 0, radioTop-30, w, h-radioBtnY-radioTop+14, gfx.White)
	radioMessage(s, wf.msg)
	for i, n := range wf.nets {
		if i >= radioRows(s) {
			break
		}
		y := radioTop + i*radioRow + 36
		drawBars(d, radioPadX, y, radio.Bars(n.RSSI))
		name, level := n.SSID, gfx.Black
		if name == "" {
			name, level = "(hidden network)", gfx.Mid
		}
		drawRadioRow(s, y, name, level, n.RSSI)
		gfx.Rect(d, radioPadX, y+12, w-2*radioPadX, 1, gfx.Light)
	}
	for _, b := range wf.buttons {
		b.On = false
	}
	if wf.job.busy() {
		wf.buttons[0].On = wf.job.what == "Scanning…"
		wf.buttons[1].On = !wf.buttons[0].On
	}
	DrawAll(d, wf.buttons)
}

// Event implements App.
func (wf *WiFi) Event(s *System, e Event) {
	if e.Kind != Tap || wf.job.busy() {
		return
	}
	switch Hit(wf.buttons, e.X, e.Y) {
	case 0:
		wf.scan(s)
	case 1:
		wf.setClock(s)
	default:
		return
	}
	wf.draw(s)
	s.Refresh(epd.ModeFast)
}

// Tick implements App.
func (wf *WiFi) Tick(s *System) {
	if wf.job.poll() {
		wf.draw(s)
		s.Refresh(epd.ModeText)
	}
}

// Bluetooth lists the Bluetooth LE devices that advertise nearby.
type Bluetooth struct {
	buttons []*Button
	job     radioJob
	devs    []radio.Device
	msg     string
	scanned bool
}

// bleScanTime is how long one scan listens.
const bleScanTime = 8 * time.Second

// Title implements App.
func (bt *Bluetooth) Title() string { return "Bluetooth" }

// Icon implements App: the Bluetooth rune.
func (bt *Bluetooth) Icon(c gfx.Canvas, cx, cy, size int) {
	s := float32(size)
	x, y := float32(cx), float32(cy)
	hw, hh := s*0.2, s*0.36
	t := s * 0.08
	pts := [][2]float32{
		{x - hw, y - hh/2}, {x + hw, y + hh/2}, {x, y + hh},
		{x, y - hh}, {x + hw, y - hh/2}, {x - hw, y + hh/2},
	}
	for i := 1; i < len(pts); i++ {
		gfx.Line(c, pts[i-1][0], pts[i-1][1], pts[i][0], pts[i][1], t, gfx.Black)
	}
}

// Start implements App.
func (bt *Bluetooth) Start(s *System) {
	w, h := s.Size()
	bt.buttons = Row(radioPadX, w-radioPadX, h-radioBtnY, radioBtnH, 14, "Scan")
	if !bt.scanned && !bt.job.busy() {
		bt.scan(s)
	}
	bt.draw(s)
}

func (bt *Bluetooth) scan(s *System) {
	bt.scanned = true
	bt.msg = "Listening for " + strconv.Itoa(int(bleScanTime/time.Second)) + " s…"
	bt.job.start(bt.msg, func() func() {
		devs, err := radio.ScanBLE(bleScanTime)
		return func() {
			if err != nil {
				bt.msg = "Scan failed: " + err.Error()
				return
			}
			bt.devs = devs
			bt.msg = strconv.Itoa(len(devs)) + " devices · " + HHMM(papers3.Now())
		}
	})
}

func (bt *Bluetooth) draw(s *System) {
	w, h := s.Size()
	d := s.D
	gfx.Rect(d, 0, radioTop-30, w, h-radioBtnY-radioTop+14, gfx.White)
	radioMessage(s, bt.msg)
	for i, dev := range bt.devs {
		if i >= radioRows(s) {
			break
		}
		y := radioTop + i*radioRow + 36
		drawBars(d, radioPadX, y, radio.Bars(dev.RSSI))
		name, level := dev.Name, gfx.Black
		if name == "" {
			name, level = dev.Address, gfx.Mid
		}
		drawRadioRow(s, y, name, level, dev.RSSI)
		gfx.Rect(d, radioPadX, y+12, w-2*radioPadX, 1, gfx.Light)
	}
	bt.buttons[0].On = bt.job.busy()
	DrawAll(d, bt.buttons)
}

// Event implements App.
func (bt *Bluetooth) Event(s *System, e Event) {
	if e.Kind != Tap || bt.job.busy() || Hit(bt.buttons, e.X, e.Y) != 0 {
		return
	}
	bt.scan(s)
	bt.draw(s)
	s.Refresh(epd.ModeFast)
}

// Tick implements App.
func (bt *Bluetooth) Tick(s *System) {
	if bt.job.poll() {
		bt.draw(s)
		s.Refresh(epd.ModeText)
	}
}
