package ui

import (
	"math"
	"runtime"
	"strconv"
	"time"

	"github.com/shabunin/tiny-m5papers3/epd"
	"github.com/shabunin/tiny-m5papers3/fonts"
	"github.com/shabunin/tiny-m5papers3/gfx"
	"github.com/shabunin/tiny-m5papers3/papers3"
)

// SysInfo shows the board status and a few settings.
type SysInfo struct {
	buttons []*Button
	updated time.Time
	armed   time.Time // power off asked once
	i2c     string
}

const (
	sysTop = HeaderH + 14
	sysRow = 44
)

// Title implements App.
func (si *SysInfo) Title() string { return "System" }

// Icon implements App: a gear.
func (si *SysInfo) Icon(c gfx.Canvas, cx, cy, size int) {
	fx, fy := float32(cx), float32(cy)
	r := float32(size) * 0.3
	for i := 0; i < 8; i++ {
		a := float64(i) * math.Pi / 4
		sx, sy := float32(math.Sin(a)), float32(math.Cos(a))
		gfx.Line(c, fx+sx*r*0.8, fy+sy*r*0.8, fx+sx*r*1.45, fy+sy*r*1.45, r*0.42, gfx.Black)
	}
	gfx.FillCircle(c, fx, fy, r*1.15, gfx.Black)
	gfx.FillCircle(c, fx, fy, r*0.5, gfx.White)
}

// Start implements App.
func (si *SysInfo) Start(s *System) {
	w, h := s.Size()
	b1 := Row(16, w-16, h-136, 54, 14, "", "Flip 180°")
	b2 := Row(16, w-16, h-70, 54, 14, "Refresh all", "Power off")
	si.buttons = append(b1, b2...)
	si.labels(s)
	if si.i2c == "" {
		for _, a := range papers3.ScanI2C() {
			si.i2c += " 0x" + strconv.FormatUint(uint64(a), 16)
		}
	}
	si.draw(s)
}

func (si *SysInfo) labels(s *System) {
	si.buttons[0].Label = "Sound: off"
	if s.Sound {
		si.buttons[0].Label = "Sound: on"
	}
	si.buttons[0].On = s.Sound
	si.buttons[3].Label = "Power off"
	si.buttons[3].On = false
	if !si.armed.IsZero() {
		si.buttons[3].Label = "Tap again"
		si.buttons[3].On = true
	}
	DrawAll(s.D, si.buttons)
}

func (si *SysInfo) rows(s *System) [][2]string {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	b := papers3.ReadBattery()
	st := papers3.GetStatus()
	charge := "discharging"
	if b.Charging {
		charge = "charging"
	}
	rtc := "not found"
	if st.RTC {
		rtc = "ok"
		if !st.RTCValid {
			rtc = "lost power, not set"
		}
	}
	motion := "not found"
	if x, y, z, ok := papers3.ReadAccel(); ok {
		motion = "x " + ftoa(x, 2) + "  y " + ftoa(y, 2) + "  z " + ftoa(z, 2) + " g"
	}
	touch := "not found"
	if st.Touch {
		touch = "GT911, fw 0x" + strconv.FormatUint(uint64(st.TouchFirmware), 16) +
			", config " + strconv.Itoa(int(st.TouchConfig))
	}
	up := s.Now.Sub(s.Started)
	return [][2]string{
		{"Software", "TinyGo " + runtime.Version() + ", " + strconv.Itoa(len(s.Apps)) + " apps"},
		{"Chip", "ESP32-S3, " + strconv.Itoa(papers3.CPUMHz()) + " MHz, " + ftoa(papers3.ChipTemperature(), 1) + " °C"},
		{"Memory", "heap " + strconv.Itoa(int(ms.HeapInuse/1024)) + " of " + strconv.Itoa(int(ms.HeapSys/1024)) + " KB in use"},
		{"Uptime", strconv.Itoa(int(up.Hours())) + " h " + strconv.Itoa(int(up.Minutes())%60) + " min" + dozed(s, up)},
		{"Battery", ftoa(b.Volts, 2) + " V, about " + strconv.Itoa(b.Percent) + "%, " + charge},
		{"Clock", rtc + ", " + s.Now.Format("2006-01-02 15:04")},
		{"Touch", touch},
		{"Motion", motion},
		{"I2C", si.i2c},
		{"Display", "ED047TC1, 960×540, 16 gray levels"},
		{"Last", s.LastUpdate},
	}
}

// dozed describes the time spent dozing (see System.Doze).
func dozed(s *System, up time.Duration) string {
	if s.Dozed == 0 || up <= 0 {
		return ", no dozing yet"
	}
	return ", dozed " + strconv.Itoa(int(100*s.Dozed/up)) + "%, " + strconv.Itoa(s.TouchWakes) + " touch wakes"
}

func (si *SysInfo) draw(s *System) {
	w, _ := s.Size()
	rows := si.rows(s)
	gfx.Rect(s.D, 0, sysTop, w, len(rows)*sysRow+80, gfx.White)
	y := sysTop + 30
	for _, r := range rows {
		gfx.DrawText(s.D, fonts.Sans22, 16, y, r[0], gfx.Dark)
		gfx.DrawText(s.D, fonts.Sans22, 128, y, r[1], gfx.Black)
		y += sysRow
	}
	gfx.DrawText(s.D, fonts.Sans18, 16, y+4, "Set the clock over USB serial:  time 2026-11-13 09:41:00", gfx.Mid)
	si.updated = s.Now
}

// Event implements App.
func (si *SysInfo) Event(s *System, e Event) {
	if e.Kind != Tap {
		return
	}
	switch Hit(si.buttons, e.X, e.Y) {
	case 0:
		s.Sound = !s.Sound
		si.labels(s)
		s.Refresh(epd.ModeFast)
		if s.Sound {
			papers3.Beep(2000, 60*time.Millisecond)
		}
	case 1:
		s.SetFlipped(!s.Flipped())
	case 2:
		s.FullRefresh(epd.ModeQuality)
	case 3:
		if si.armed.IsZero() || s.Now.Sub(si.armed) > 4*time.Second {
			si.armed = s.Now
			si.labels(s)
			s.Refresh(epd.ModeFast)
			return
		}
		si.powerOff(s)
	}
}

func (si *SysInfo) powerOff(s *System) {
	w, h := s.Size()
	d := s.D
	d.FillScreen(gfx.White)
	gfx.DrawTextCentered(d, fonts.SerifBold36, w/2, h/2-40, "Switched off", gfx.Black)
	gfx.DrawTextCentered(d, fonts.Sans22, w/2, h/2+10, "press the power button to start again", gfx.Dark)
	s.FullRefresh(epd.ModeQuality)
	s.Flush()
	papers3.PowerOff()
	// Still here: USB power keeps the board on.
	si.armed = time.Time{}
	s.Redraw()
	gfx.DrawTextCentered(d, fonts.SansBold28, w/2, h-160, "USB power keeps the board on", gfx.Black)
}

// Tick implements App: refresh the values every 5 s, or every minute when
// nobody has touched the screen for a while (spares the panel).
func (si *SysInfo) Tick(s *System) {
	if !si.armed.IsZero() && s.Now.Sub(si.armed) > 4*time.Second {
		si.armed = time.Time{}
		si.labels(s)
		s.Refresh(epd.ModeFast)
	}
	every := 5 * time.Second
	if s.IdleFor() > 2*time.Minute {
		every = time.Minute
	}
	if s.Now.Sub(si.updated) >= every {
		si.draw(s)
		s.Refresh(epd.ModeFast)
	}
}
