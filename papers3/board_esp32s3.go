//go:build esp32s3

package papers3

import (
	"machine"
	"runtime"
	"sync/atomic"
	"time"

	"tiny-m5papers3/gt911"

	"tinygo.org/x/drivers/bmi270"
)

// Pins (M5Unified / M5GFX board data for the PaperS3).
const (
	pinSDA       = machine.GPIO41 // internal I2C: touch, RTC, IMU
	pinSCL       = machine.GPIO42
	pinBattery   = machine.GPIO3  // battery voltage / 2
	pinCharge    = machine.GPIO4  // charger status, low while charging
	pinBuzzer    = machine.GPIO21 // passive buzzer
	pinLED       = machine.GPIO0  // green LED, high = on
	pinPowerHold = machine.GPIO44 // power latch: pulsing it turns the board off
)

var (
	i2c    = machine.I2C0
	touch  *gt911.Device
	imu    bmi270.Device
	adc    machine.ADC
	status Status
	pwm    = machine.PWM1
)

// Configure sets up all on-board devices. A device that does not respond is
// skipped (see the returned Status); the display always works.
func Configure() Status {
	Display.Configure()
	Display.SetRotation(0)
	i2c.Configure(machine.I2CConfig{SDA: pinSDA, SCL: pinSCL, Frequency: 400 * machine.KHz})

	status.Wake = startReason()

	touch = gt911.New(i2c)
	if touch.Configure() == nil {
		status.Touch = true
		status.TouchFirmware, _ = touch.Firmware()
		status.TouchConfig, _ = touch.ConfigVersion()
	}

	syncClock()

	imu = bmi270.NewI2C(i2c)
	status.IMU = imu.Configure(bmi270.Config{}) == nil

	machine.InitADC()
	adc = machine.ADC{Pin: pinBattery}
	adc.Configure(machine.ADCConfig{})
	pinCharge.Configure(machine.PinConfig{Mode: machine.PinInput})
	pinLED.Configure(machine.PinConfig{Mode: machine.PinOutput})
	pinLED.Low()
	return status
}

// ReadTouches appends the current touches to buf[:0].
func ReadTouches(buf []Touch) []Touch {
	buf = buf[:0]
	if !status.Touch {
		return buf
	}
	pts, err := touch.Read()
	if err != nil {
		return buf
	}
	for _, p := range pts {
		buf = append(buf, Touch{
			ID: p.ID,
			X:  min(max(p.X, 0), epdShort-1),
			Y:  min(max(p.Y, 0), epdLong-1),
		})
	}
	return buf
}

const (
	epdShort = 540
	epdLong  = 960
)

// Now returns the wall-clock time (local time, labelled UTC), kept by the
// RTC across resets and power-off. It is time.Now, plus the corrections made
// while SteadyClock held.
func Now() time.Time { return time.Now().Add(time.Duration(nowOffset.Load())) }

// nowOffset is Now - time.Now (ns): zero unless the time was set while
// SteadyClock held.
var nowOffset atomic.Int64

// syncClock sets the system clock from the RTC.
func syncClock() {
	var r [7]byte
	if err := i2c.Tx(rtcAddress, []byte{rtcSeconds}, r[:]); err != nil {
		return
	}
	status.RTC = true
	t, valid := decodeRTC(r[:])
	status.RTCValid = valid
	if !valid {
		t = DefaultTime
	}
	setSystemTime(t)
}

// setSystemTime makes Now return t, by moving the system clock unless
// SteadyClock holds.
func setSystemTime(t time.Time) {
	if SteadyClock != nil && SteadyClock() {
		nowOffset.Store(t.UnixNano() - time.Now().UnixNano())
		return
	}
	nowOffset.Store(0)
	runtime.AdjustTimeOffset(t.UnixNano() - time.Now().UnixNano())
}

// SetTime sets the RTC and the time Now returns (local wall-clock time). It
// moves the system clock too, unless SteadyClock holds.
func SetTime(t time.Time) error {
	r := encodeRTC(t)
	if err := i2c.Tx(rtcAddress, []byte{rtcCtrl1, 0, 0}, nil); err != nil { // clock running
		return err
	}
	buf := append([]byte{rtcSeconds}, r[:]...)
	if err := i2c.Tx(rtcAddress, buf, nil); err != nil {
		return err
	}
	setSystemTime(t)
	status.RTCValid = true
	return nil
}

// ReadBattery measures the battery.
func ReadBattery() Battery {
	_, v := adc.GetVoltage()
	volts := float32(v) * 2 // 1:1 divider
	return Battery{Volts: volts, Percent: percent(volts), Charging: !pinCharge.Get()}
}

// ChipTemperature returns the ESP32-S3's internal temperature in °C (a few
// degrees above the room).
func ChipTemperature() float32 { return float32(machine.ReadTemperature()) / 1000 }

// ReadAccel returns the acceleration in g along the IMU's axes.
func ReadAccel() (x, y, z float32, ok bool) {
	if !status.IMU {
		return 0, 0, 0, false
	}
	ax, ay, az, err := imu.ReadAcceleration() // µg
	if err != nil {
		return 0, 0, 0, false
	}
	return float32(ax) / 1e6, float32(ay) / 1e6, float32(az) / 1e6, true
}

// ReadGyro returns the rotation rate in °/s.
func ReadGyro() (x, y, z float32, ok bool) {
	if !status.IMU {
		return 0, 0, 0, false
	}
	gx, gy, gz, err := imu.ReadRotation() // µ°/s
	if err != nil {
		return 0, 0, 0, false
	}
	return float32(gx) / 1e6, float32(gy) / 1e6, float32(gz) / 1e6, true
}

// Beep plays a tone on the buzzer (blocking).
func Beep(hz int, d time.Duration) {
	if hz <= 0 {
		time.Sleep(d)
		return
	}
	if pwm.Configure(machine.PWMConfig{Period: uint64(1e9 / hz)}) != nil {
		return
	}
	ch, err := pwm.Channel(pinBuzzer)
	if err != nil {
		return
	}
	pwm.Set(ch, pwm.Top()/2)
	time.Sleep(d)
	pwm.Set(ch, 0)
}

// SetLED switches the green LED.
func SetLED(on bool) { pinLED.Set(on) }

// PowerOff cuts the power when running from the battery (like M5Unified:
// GPIO44 is pulsed a few times). With USB connected the board stays on.
func PowerOff() {
	pinPowerHold.Configure(machine.PinConfig{Mode: machine.PinOutput})
	for range 5 {
		pinPowerHold.Low()
		time.Sleep(50 * time.Millisecond)
		pinPowerHold.High()
		time.Sleep(50 * time.Millisecond)
	}
}

// PowerOffFor switches the board off and lets the RTC switch it on again
// after d (1 s to 255 min, rounded to seconds below 255 s and to minutes
// above). The program then starts with GetStatus().Wake == WakeAlarm. With
// USB power connected the board cannot switch off: it deep-sleeps for d
// instead (Wake == WakeTimer). Only returns on an I2C error.
func PowerOffFor(d time.Duration) error {
	source, count := rtcTimer(d)
	for _, w := range [][]byte{
		{rtcTimerCtrl, 0x03},          // stop the timer
		{rtcCtrl2, 0x00},              // clear the flags
		{rtcTimerValue, count},        //
		{rtcTimerCtrl, 0x80 | source}, // start
		{rtcCtrl2, 0x01},              // TIE: INT goes low when it expires
	} {
		if err := i2c.Tx(rtcAddress, w, nil); err != nil {
			return err
		}
	}
	PowerOff()
	time.Sleep(200 * time.Millisecond)
	DeepSleep(d) // still running: on USB power
	return nil
}

// rtcTimerFired reports whether the RTC countdown timer expired with its
// interrupt enabled (it switched the board on), then stops the timer and
// clears the flags, so that the INT line is released.
func rtcTimerFired() bool {
	var c [1]byte
	if i2c.Tx(rtcAddress, []byte{rtcCtrl2}, c[:]) != nil {
		return false
	}
	fired := c[0]&0x05 == 0x05
	if c[0]&0x0f != 0 {
		i2c.Tx(rtcAddress, []byte{rtcCtrl2, 0}, nil)
		i2c.Tx(rtcAddress, []byte{rtcTimerCtrl, 0x03}, nil)
	}
	return fired
}

// ScanI2C lists the addresses that answer on the internal I2C bus.
func ScanI2C() []uint8 {
	var found []uint8
	var b [1]byte
	for a := uint16(8); a < 0x78; a++ {
		if i2c.Tx(a, nil, b[:]) == nil {
			found = append(found, uint8(a))
		}
	}
	return found
}

// GetStatus returns what Configure found.
func GetStatus() Status { return status }

// CPUMHz returns the CPU clock.
func CPUMHz() int {
	f, err := machine.GetCPUFrequency()
	if err != nil {
		return 0
	}
	return int(f / 1_000_000)
}
