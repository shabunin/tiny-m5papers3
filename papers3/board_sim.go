//go:build !esp32s3

package papers3

import (
	"time"
)

// Simulated board state. Programs running on a PC set these to drive the UI.
var (
	// SimTouches is what ReadTouches returns.
	SimTouches []Touch
	// SimTime is the wall-clock time returned by Now.
	SimTime = time.Date(2026, 11, 13, 9, 41, 0, 0, time.UTC)
	// SimBattery is what ReadBattery returns.
	SimBattery = Battery{Volts: 4.02, Percent: percent(4.02)}
	// SimAccel is what ReadAccel returns (g).
	SimAccel = [3]float32{0, 0, 1}
	// SimTemperature is the chip temperature (°C).
	SimTemperature float32 = 34.5
	// SimLog receives a line for every Beep, SetLED and PowerOff.
	SimLog func(string)
)

var status Status

// Configure sets up the simulated board.
func Configure() Status {
	Display.Configure()
	Display.SetRotation(0)
	status = Status{Touch: true, RTC: true, IMU: true, RTCValid: true, TouchFirmware: 0x1060, TouchConfig: 66}
	return status
}

// ReadTouches appends SimTouches to buf[:0].
func ReadTouches(buf []Touch) []Touch { return append(buf[:0], SimTouches...) }

// Now returns SimTime.
func Now() time.Time { return SimTime }

// SetTime sets SimTime.
func SetTime(t time.Time) error {
	SimTime = t
	return nil
}

// ReadBattery returns SimBattery.
func ReadBattery() Battery { return SimBattery }

// ChipTemperature returns SimTemperature.
func ChipTemperature() float32 { return SimTemperature }

// ReadAccel returns SimAccel.
func ReadAccel() (x, y, z float32, ok bool) { return SimAccel[0], SimAccel[1], SimAccel[2], true }

// ReadGyro returns zero rotation.
func ReadGyro() (x, y, z float32, ok bool) { return 0, 0, 0, true }

func simLog(s string) {
	if SimLog != nil {
		SimLog(s)
	}
}

// Beep logs the tone.
func Beep(hz int, d time.Duration) { simLog("beep " + itoa(hz) + " Hz " + d.String()) }

// SetLED logs the LED state.
func SetLED(on bool) {
	if on {
		simLog("led on")
	} else {
		simLog("led off")
	}
}

// PowerOff logs the request.
func PowerOff() { simLog("power off") }

// ScanI2C returns the devices of a real PaperS3.
func ScanI2C() []uint8 { return []uint8{0x51, 0x5d, 0x68} }

// GetStatus returns what Configure found.
func GetStatus() Status { return status }

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var b [20]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// CPUMHz returns the CPU clock of the real board.
func CPUMHz() int { return 240 }

// SimUSBHost is what USBHost returns.
var SimUSBHost = true

var retained [RetainedWords]uint32

// USBHost returns SimUSBHost.
func USBHost() bool { return SimUSBHost }

var lastSlept time.Duration

// LightSleep advances SimTime by d, as if nothing touched the screen.
func LightSleep(d time.Duration) Wake {
	lastSlept = max(d, 0)
	SimTime = SimTime.Add(lastSlept)
	return WakeTimer
}

// LastSleep returns how long the last LightSleep lasted.
func LastSleep() time.Duration { return lastSlept }

// DeepSleep logs the request and returns (the real one restarts the
// program).
func DeepSleep(d time.Duration) { simLog("deep sleep " + d.String()) }

// PowerOffFor logs the request.
func PowerOffFor(d time.Duration) error {
	simLog("power off for " + d.String())
	return nil
}

// Retained returns a word set by SetRetained.
func Retained(i int) uint32 {
	if i < 0 || i >= RetainedWords {
		return 0
	}
	return retained[i]
}

// SetRetained sets a word that survives DeepSleep.
func SetRetained(i int, v uint32) {
	if i >= 0 && i < RetainedWords {
		retained[i] = v
	}
}
