// Package ui is a small app framework for the PaperS3: a launcher, a status
// header, touch gestures and refresh scheduling for the e-paper panel.
//
// Apps draw into the frame buffer with package gfx and then ask for a
// refresh with the waveform that suits the change (see System.Refresh).
// The same code runs on the device and in the PC simulation (cmd/sim).
package ui

import (
	"strconv"
	"time"

	"github.com/shabunin/tiny-m5papers3/epd"
	"github.com/shabunin/tiny-m5papers3/fonts"
	"github.com/shabunin/tiny-m5papers3/gfx"
	"github.com/shabunin/tiny-m5papers3/papers3"
	"github.com/shabunin/tiny-m5papers3/radio"
)

// EventKind is the type of a touch event.
type EventKind uint8

const (
	Down EventKind = iota // a finger touched the screen
	Move                  // it moved
	Up                    // the last finger was lifted (X, Y: last position)
	Tap                   // after Up, if the finger did not move much
)

func (k EventKind) String() string {
	return [...]string{"down", "move", "up", "tap"}[k]
}

// Event is a touch event in drawing coordinates. Multi-touch apps can look
// at System.Touches instead.
type Event struct {
	Kind   EventKind
	X, Y   int // current position of the first finger
	X0, Y0 int // where it touched down
}

// App is a full-screen application.
type App interface {
	// Title is shown in the header and on the launcher.
	Title() string
	// Icon draws the launcher icon, centered on (cx, cy), about size pixels.
	Icon(c gfx.Canvas, cx, cy, size int)
	// Start draws the app below the header (the screen is white).
	Start(s *System)
	// Event handles a touch below the header.
	Event(s *System, e Event)
	// Tick is called on every loop iteration (every few ms) for animations
	// and timers; use s.Now.
	Tick(s *System)
}

// HeaderH is the height of the status header.
const HeaderH = 64

// System runs the apps.
type System struct {
	D    *epd.Display
	Home App
	Apps []App
	Now  time.Time
	// Started is the time of the first Step.
	Started time.Time
	// Touches are the current touch points in drawing coordinates.
	Touches []papers3.Touch

	app App

	// Pending refresh.
	refresh bool
	mode    epd.Mode
	full    bool

	// Touch tracking.
	down           bool
	inHeader       bool
	moved          bool
	x0, y0, lx, ly int
	downAt         time.Time
	lastTouch      time.Time

	// Header state.
	hdrMinute  int
	hdrBattery int
	lastFull   time.Time

	// Dozed is the time spent in light sleep by Doze, TouchWakes how many
	// dozes a touch ended.
	Dozed      time.Duration
	TouchWakes int

	// Sound enables a short click on taps.
	Sound bool
	// Log, if set, receives debug messages.
	Log func(string)
	// LastUpdate describes the last display update (for the System app).
	LastUpdate string
}

// New returns a system showing home.
func New(d *epd.Display, home App, apps ...App) *System {
	return &System{D: d, Home: home, Apps: apps, hdrMinute: -1}
}

func (s *System) log(msg string) {
	if s.Log != nil {
		s.Log(msg)
	}
}

// Size returns the drawing area.
func (s *System) Size() (w, h int) {
	x, y := s.D.Size()
	return int(x), int(y)
}

// Current returns the running app.
func (s *System) Current() App { return s.app }

// Open switches to an app: clears the screen, draws the header and the app,
// and schedules a full-quality update.
func (s *System) Open(a App) {
	s.app = a
	s.down = false
	s.D.FillScreen(gfx.White)
	s.drawHeader(true)
	a.Start(s)
	s.FullRefresh(epd.ModeQuality)
	s.log("open " + a.Title())
}

// Redraw redraws the current app from scratch (e.g. after a rotation).
func (s *System) Redraw() { s.Open(s.app) }

// Refresh asks for the changed pixels to be sent to the panel after the
// current step, with mode m. If several are requested, the best quality
// wins (ModeQuality > ModeText > ModeFast > ModeFastest).
func (s *System) Refresh(m epd.Mode) {
	if !s.refresh || m < s.mode {
		s.mode = m
	}
	s.refresh = true
}

// FullRefresh asks for a full-screen update with mode m (flashes the whole
// panel in ModeQuality/ModeText: clears all ghosting).
func (s *System) FullRefresh(m epd.Mode) {
	s.Refresh(m)
	s.full = true
}

// Flush performs the pending refresh now (for apps that want to show
// progress before a long computation).
func (s *System) Flush() {
	if !s.refresh {
		return
	}
	var err error
	if s.full {
		err = s.D.Update(s.mode)
	} else {
		err = s.D.Refresh(s.mode)
	}
	what := "refresh"
	if s.full {
		what = "full update"
		s.lastFull = s.Now
	}
	s.LastUpdate = what + " " + modeName(s.mode) + ": " +
		strconv.Itoa(int(s.D.UpdateUs/1000)) + " ms, " + strconv.Itoa(s.D.Frames) + " frames"
	if err != nil {
		s.LastUpdate += " ERROR " + err.Error()
	}
	s.log(s.LastUpdate)
	s.refresh, s.full = false, false
}

func modeName(m epd.Mode) string {
	return [...]string{"quality", "text", "fast", "fastest"}[m]
}

// Step runs one iteration: touch handling, app tick, display refresh.
func (s *System) Step(now time.Time, touches []papers3.Touch) {
	s.Now = now
	if s.Started.IsZero() {
		s.Started = now
	}
	if s.app == nil {
		s.Open(s.Home)
	}
	// Touches to drawing coordinates.
	s.Touches = s.Touches[:0]
	for _, t := range touches {
		x, y := s.D.FromPortrait(t.X, t.Y)
		s.Touches = append(s.Touches, papers3.Touch{ID: t.ID, X: x, Y: y})
	}
	s.gestures()
	s.app.Tick(s)
	s.updateHeader()
	s.Flush()
}

// Busy is implemented by apps that animate or poll something while nobody
// touches the screen (the Life simulation, the spirit level): the board does
// not doze while Busy reports true.
type Busy interface {
	Busy() bool
}

var (
	// DozeAfter is how long nobody must have touched the screen before the
	// board dozes.
	DozeAfter = 15 * time.Second
	// DozeMax caps a doze, so that the board looks at the touch panel at
	// least that often even if a touch did not wake it.
	DozeMax = 2 * time.Second
	// DozeAllowed reports whether the board may doze at all: on battery (a
	// computer on the USB port would see its serial port hang) with the
	// radio off.
	DozeAllowed = func() bool { return !radio.Active() && !papers3.USBHost() }
)

// Doze puts the chip into light sleep (papers3.LightSleep) when nothing
// happens: DozeAllowed, nobody touching the screen for DozeAfter, no refresh
// pending and the app not Busy. A touch wakes it, or the next minute (for
// the clock), or DozeMax. It reports whether the chip slept; the display
// keeps its picture meanwhile. The launcher calls it between Steps.
func (s *System) Doze() bool {
	d := s.dozeFor()
	if d <= 0 || !DozeAllowed() {
		return false
	}
	if papers3.LightSleep(d) == papers3.WakeTouch {
		s.TouchWakes++
	}
	s.Dozed += papers3.LastSleep()
	return true
}

// dozeFor returns how long the board may sleep now, 0 if not at all.
func (s *System) dozeFor() time.Duration {
	if s.app == nil || s.down || s.refresh || s.IdleFor() < DozeAfter {
		return 0
	}
	if b, ok := s.app.(Busy); ok && b.Busy() {
		return 0
	}
	// Wake just after the minute changes, to update the clock.
	next := s.Now.Truncate(time.Minute).Add(time.Minute + 20*time.Millisecond).Sub(s.Now)
	return min(next, DozeMax)
}

// IdleFor is the time since the screen was last touched.
func (s *System) IdleFor() time.Duration {
	if s.lastTouch.IsZero() {
		return s.Now.Sub(s.Started)
	}
	return s.Now.Sub(s.lastTouch)
}

func (s *System) gestures() {
	if len(s.Touches) > 0 {
		s.lastTouch = s.Now
		x, y := int(s.Touches[0].X), int(s.Touches[0].Y)
		if !s.down {
			s.down, s.moved = true, false
			s.x0, s.y0, s.lx, s.ly = x, y, x, y
			s.downAt = s.Now
			s.inHeader = y < HeaderH
			if !s.inHeader {
				s.app.Event(s, Event{Down, x, y, x, y})
			}
			return
		}
		if abs(x-s.x0) > 14 || abs(y-s.y0) > 14 {
			s.moved = true
		}
		if (x != s.lx || y != s.ly) && !s.inHeader {
			s.app.Event(s, Event{Move, x, y, s.x0, s.y0})
		}
		s.lx, s.ly = x, y
		return
	}
	if !s.down {
		return
	}
	s.down = false
	tap := !s.moved && s.Now.Sub(s.downAt) < 1500*time.Millisecond
	if s.inHeader {
		if tap {
			s.headerTap(s.x0, s.y0)
		}
		return
	}
	s.app.Event(s, Event{Up, s.lx, s.ly, s.x0, s.y0})
	if tap {
		if s.Sound {
			papers3.Beep(2400, 15*time.Millisecond)
		}
		s.app.Event(s, Event{Tap, s.x0, s.y0, s.x0, s.y0})
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// headerTap handles the Home button.
func (s *System) headerTap(x, y int) {
	if s.app != s.Home && x < homeButtonW+20 {
		if s.Sound {
			papers3.Beep(2400, 15*time.Millisecond)
		}
		s.Open(s.Home)
	}
}

const homeButtonW = 130

// drawHeader draws the status header; with all=false only the clock and
// battery part.
func (s *System) drawHeader(all bool) {
	d := s.D
	w, _ := s.Size()
	if all {
		gfx.Rect(d, 0, 0, w, HeaderH, gfx.White)
		gfx.Rect(d, 0, HeaderH-3, w, 3, gfx.Black)
		if s.app != s.Home {
			// Home button: chevron + label.
			gfx.RoundRect(d, 10, 9, homeButtonW, 42, 21, 2, gfx.Black)
			gfx.Line(d, 36, 22, 26, 30, 4, gfx.Black)
			gfx.Line(d, 26, 30, 36, 38, 4, gfx.Black)
			gfx.DrawText(d, fonts.SansBold28, 46, 40, "Home", gfx.Black)
			gfx.DrawText(d, fonts.SansBold28, homeButtonW+26, 42, s.app.Title(), gfx.Black)
		} else {
			gfx.DrawText(d, fonts.SansBold28, 16, 42, "TinyGo", gfx.Black)
			gfx.DrawText(d, fonts.Sans22, 120, 41, "on M5PaperS3", gfx.Dark)
		}
	}
	s.drawStatus()
}

// drawStatus draws time and battery at the right end of the header.
func (s *System) drawStatus() {
	d := s.D
	w, _ := s.Size()
	gfx.Rect(d, w-236, 4, 236, HeaderH-8, gfx.White)
	b := papers3.ReadBattery()
	s.hdrMinute = s.Now.Hour()*60 + s.Now.Minute()
	s.hdrBattery = b.Percent
	bx := w - 78
	BatteryIcon(d, bx, 20, b)
	if s.app != s.Home {
		gfx.DrawTextRight(d, fonts.SansBold28, bx-62, 42, HHMM(s.Now), gfx.Black)
	}
}

// BatteryIcon draws a battery gauge of about 70x26 pixels with its top-left
// corner at (x, y).
func BatteryIcon(c gfx.Canvas, x, y int, b papers3.Battery) {
	gfx.RoundRect(c, x, y, 58, 26, 5, 3, gfx.Black)
	gfx.Rect(c, x+58, y+8, 5, 10, gfx.Black)
	fill := (52 - 4) * min(max(b.Percent, 0), 100) / 100
	gfx.Rect(c, x+5, y+5, fill, 16, gfx.Dark)
	if b.Charging {
		// lightning bolt
		gfx.FillTriangle(c, float32(x+31), float32(y+2), float32(x+21), float32(y+15), float32(x+29), float32(y+15), gfx.Black)
		gfx.FillTriangle(c, float32(x+27), float32(y+24), float32(x+37), float32(y+11), float32(x+29), float32(y+11), gfx.Black)
	}
	gfx.DrawTextRight(c, fonts.Sans18, x-6, y+20, strconv.Itoa(b.Percent)+"%", gfx.Black)
}

// updateHeader refreshes the header clock and battery once a minute.
func (s *System) updateHeader() {
	m := s.Now.Hour()*60 + s.Now.Minute()
	if m == s.hdrMinute {
		return
	}
	s.drawStatus()
	s.Refresh(epd.ModeText)
	// Full refresh every hour against ghosting, unless the user is busy.
	if s.Now.Sub(s.lastFull) > time.Hour && !s.down {
		s.FullRefresh(epd.ModeQuality)
	}
}

// HHMM formats the time as HH:MM.
func HHMM(t time.Time) string {
	return two(t.Hour()) + ":" + two(t.Minute())
}

func two(v int) string {
	return string([]byte{byte('0' + v/10), byte('0' + v%10)})
}

// Flipped reports whether the display is upside down (PortraitFlipped).
func (s *System) Flipped() bool { return s.D.Rotation() == epd.PortraitFlipped }

// SetFlipped turns the display upside down (or back) and redraws.
func (s *System) SetFlipped(on bool) {
	r := epd.Portrait
	if on {
		r = epd.PortraitFlipped
	}
	s.D.SetRotation(r)
	s.Redraw()
}
