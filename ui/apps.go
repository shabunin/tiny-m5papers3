package ui

import "github.com/shabunin/tiny-m5papers3/epd"

// NewLauncher returns a system with the home screen and all built-in apps.
func NewLauncher(d *epd.Display) *System {
	return New(d, &Home{},
		&Paint{}, &Mandel{}, &Life{}, &Level{},
		&Grays{}, &Reader{Text: BookText}, &TouchTest{}, &Clock{},
		&WiFi{}, &Bluetooth{}, &SysInfo{},
	)
}

// Find returns the app with the given title (case-insensitive), or nil.
func (s *System) Find(title string) App {
	for _, a := range append([]App{s.Home}, s.Apps...) {
		if equalFold(a.Title(), title) {
			return a
		}
	}
	return nil
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		x, y := a[i], b[i]
		if 'A' <= x && x <= 'Z' {
			x += 'a' - 'A'
		}
		if 'A' <= y && y <= 'Z' {
			y += 'a' - 'A'
		}
		if x != y {
			return false
		}
	}
	return true
}
