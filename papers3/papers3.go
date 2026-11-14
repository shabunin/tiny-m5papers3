// Package papers3 is board support for the M5Stack PaperS3: the e-paper
// display, GT911 touch panel, BM8563 real-time clock, BMI270 IMU, battery
// gauge, buzzer and LED, behind one small API.
//
// On the ESP32-S3 it drives the real hardware. On other platforms (go test,
// go run) everything is simulated, so UI code can be developed and tested on
// a PC: the display renders into an image, touches and sensor values are set
// by the program (see the Sim* functions in board_sim.go).
//
// Call Configure once at startup.
package papers3

import (
	"time"

	"tiny-m5papers3/epd"
)

// Display is the 960x540 e-paper panel. Configure sets it up.
var Display = epd.New()

// SteadyClock, when set and returning true, stops SetTime and the clock
// correction after light sleep from moving the system clock (time.Now): they
// move only the time Now returns. Set it while something times its work with
// time.Now: the ESP32-S3 radio drivers (tinygo.org/x/espradio v0.5.0) do, and
// their timers stall for as long as the clock is set back. Package ui sets it
// to radio.Active.
var SteadyClock func() bool

// Touch is a finger on the touch panel, in Portrait coordinates (as used by
// epd.Display.FromPortrait): x 0..539, y 0..959.
type Touch struct {
	ID   uint8 // stable while the finger stays down
	X, Y int16
}

// Battery is the state of the built-in Li-ion battery.
type Battery struct {
	Volts    float32
	Percent  int  // estimated from the voltage
	Charging bool // the charger reports charging (USB connected, not full)
}

// Status tells which parts of the board responded during Configure.
type Status struct {
	Touch, RTC, IMU bool
	// RTCValid is false if the RTC lost its time (battery ran flat) and the
	// clock was set to a default.
	RTCValid bool
	// TouchFirmware and TouchConfig identify the GT911 setup.
	TouchFirmware uint16
	TouchConfig   uint8
	// Wake tells why the program started: WakeTimer after DeepSleep,
	// WakeAlarm after PowerOffFor, otherwise WakeNone.
	Wake Wake
}

// percent estimates the charge of a Li-ion cell from its resting voltage.
func percent(v float32) int {
	curve := [...]struct{ v, p float32 }{
		{3.30, 0}, {3.50, 6}, {3.60, 15}, {3.70, 32}, {3.80, 50},
		{3.90, 65}, {4.00, 78}, {4.10, 90}, {4.18, 100},
	}
	if v <= curve[0].v {
		return 0
	}
	for i := 1; i < len(curve); i++ {
		if v < curve[i].v {
			a, b := curve[i-1], curve[i]
			return int(a.p + (v-a.v)/(b.v-a.v)*(b.p-a.p) + 0.5)
		}
	}
	return 100
}

// DefaultTime is used when the RTC has no valid time.
var DefaultTime = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

// RTC registers (BM8563, compatible with the NXP PCF8563).
const (
	rtcAddress    = 0x51
	rtcCtrl1      = 0x00
	rtcCtrl2      = 0x01 // bit 0: TIE, bit 2: TF (timer expired)
	rtcSeconds    = 0x02 // bit 7: VL, clock integrity no longer guaranteed
	rtcTimerCtrl  = 0x0e // bit 7: TE, bits 1-0: 4096, 64, 1, 1/60 Hz
	rtcTimerValue = 0x0f
)

// rtcTimer returns the BM8563 countdown setting for d: the source clock
// (2 = 1 Hz, 3 = 1/60 Hz) and the count. The longest time is 255 minutes.
func rtcTimer(d time.Duration) (source, count byte) {
	s := int((d + time.Second/2) / time.Second)
	if s <= 255 {
		return 2, byte(max(s, 1))
	}
	return 3, byte(min((s+30)/60, 255))
}

func bcd(v int) byte   { return byte(v/10<<4 | v%10) }
func unbcd(b byte) int { return int(b>>4)*10 + int(b&15) }

// decodeRTC converts the 7 time registers (seconds to years) to a time. The
// RTC holds local wall-clock time; it is returned as if it were UTC.
func decodeRTC(r []byte) (t time.Time, valid bool) {
	year := 2000 + unbcd(r[6])
	if r[5]&0x80 != 0 { // century bit
		year += 100
	}
	month := time.Month(unbcd(r[5] & 0x1f))
	t = time.Date(year, month, unbcd(r[3]&0x3f),
		unbcd(r[2]&0x3f), unbcd(r[1]&0x7f), unbcd(r[0]&0x7f), 0, time.UTC)
	valid = r[0]&0x80 == 0 && month >= 1 && month <= 12
	return t, valid
}

// encodeRTC is the inverse of decodeRTC; writing it clears the VL flag.
func encodeRTC(t time.Time) [7]byte {
	month := bcd(int(t.Month()))
	if t.Year() >= 2100 {
		month |= 0x80
	}
	return [7]byte{
		bcd(t.Second()), bcd(t.Minute()), bcd(t.Hour()),
		bcd(t.Day()), byte(t.Weekday()), month, bcd(t.Year() % 100),
	}
}
