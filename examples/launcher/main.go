//go:build esp32s3

// launcher is the PaperS3 home screen with its apps (see package ui).
//
// On battery it dozes (light sleep) when nobody touches the screen for a
// while; see ui.System.Doze.
//
// Besides the touch screen it listens on the USB serial port for commands,
// one per line (tools/papers3.py sends them for you):
//
//	time 2026-11-13 09:41:00   set the RTC (local time)
//	tap X Y                    touch at X,Y (Portrait coordinates) and lift
//	drag X0 Y0 X1 Y1           swipe from X0,Y0 to X1,Y1
//	open NAME                  open an app by its title (or "home")
//	shot                       dump the frame buffer (hex, native rows)
//	status                     print the board status
//	beep                       short beep
//	sleep MS                   light sleep (the serial port hangs meanwhile)
//	deepsleep S                deep sleep, then restart
//	sleepinfo                  RTC slow clock, sleep voltages, USB host
//	dozetest N                 doze N times (up to 2 s each) despite USB
package main

import (
	"machine"
	"runtime"
	"strconv"
	"strings"
	"time"

	"tiny-m5papers3/epd"
	"tiny-m5papers3/papers3"
	"tiny-m5papers3/radio"
	"tiny-m5papers3/ui"
)

var (
	sys    *ui.System
	queue  [][]papers3.Touch // injected touch frames, one per step
	line   []byte
	status papers3.Status
)

// WiFi network for "Set clock" in the WiFi app, set at build time:
//
//	tinygo flash -target esp32s3-generic \
//	    -ldflags "-X main.wifiSSID=MyNetwork -X main.wifiPassword=secret" ./examples/launcher
var wifiSSID, wifiPassword string

func main() {
	ui.WiFiSSID, ui.WiFiPassword = wifiSSID, wifiPassword
	status = papers3.Configure()
	sys = ui.NewLauncher(papers3.Display)
	sys.Log = func(s string) { println(s) }
	printStatus()

	var touches []papers3.Touch
	for {
		touches = papers3.ReadTouches(touches)
		if len(queue) > 0 {
			touches = append(touches[:0], queue[0]...)
			queue = queue[1:]
		}
		sys.Step(papers3.Now(), touches)
		serial()
		// On battery, light sleep when idle (see ui.System.Doze).
		if !sys.Doze() {
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func printStatus() {
	println("launcher: touch", status.Touch, "rtc", status.RTC, "valid", status.RTCValid,
		"imu", status.IMU, "wake", status.Wake.String(), "time", papers3.Now().Format("2006-01-02 15:04:05"))
	var ms runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&ms)
	b := papers3.ReadBattery()
	println("heap", ms.HeapInuse, "/", ms.HeapSys, "goroutines", runtime.NumGoroutine(), "battery", int(b.Volts*1000), "mV", b.Percent, "% charging", b.Charging,
		"temp", int(papers3.ChipTemperature()*10), "/10 C")
	if x, y, z, ok := papers3.ReadAccel(); ok {
		println("accel mg", int(x*1000), int(y*1000), int(z*1000))
	}
	println("dozed", sys.Dozed.String(), "touch wakes", sys.TouchWakes)
	if radio.Active() {
		u, c := radio.ArenaStats()
		println("radio on; driver arena", u, "/", c)
	}
	if app := sys.Current(); app != nil {
		println("app", app.Title(), "last", sys.LastUpdate)
	}
}

// serial reads command lines from the USB serial port.
func serial() {
	for machine.Serial.Buffered() > 0 {
		c, _ := machine.Serial.ReadByte()
		if c == '\r' {
			continue
		}
		if c != '\n' {
			if len(line) < 120 {
				line = append(line, c)
			}
			continue
		}
		command(strings.Fields(string(line)))
		line = line[:0]
	}
}

func command(f []string) {
	if len(f) == 0 {
		return
	}
	num := func(i int) int {
		if i >= len(f) {
			return 0
		}
		v, _ := strconv.Atoi(f[i])
		return v
	}
	switch f[0] {
	case "time":
		if len(f) < 3 {
			println("usage: time YYYY-MM-DD HH:MM:SS")
			return
		}
		t, err := time.Parse("2006-01-02 15:04:05", f[1]+" "+f[2])
		if err != nil {
			println("time:", err.Error())
			return
		}
		if err := papers3.SetTime(t); err != nil {
			println("time:", err.Error())
			return
		}
		println("time set", papers3.Now().Format("2006-01-02 15:04:05"))
	case "tap":
		p := []papers3.Touch{{X: int16(num(1)), Y: int16(num(2))}}
		queue = append(queue, p, p, nil)
	case "drag":
		x0, y0, x1, y1 := num(1), num(2), num(3), num(4)
		const n = 12
		for k := 0; k <= n; k++ {
			queue = append(queue, []papers3.Touch{{X: int16(x0 + (x1-x0)*k/n), Y: int16(y0 + (y1-y0)*k/n)}})
		}
		queue = append(queue, nil)
	case "open":
		name := "home"
		if len(f) > 1 {
			name = f[1]
		}
		if a := sys.Find(name); a != nil {
			sys.Open(a)
		} else {
			println("open: no app", name)
		}
	case "shot":
		shot()
	case "status":
		printStatus()
	case "beep":
		papers3.Beep(2000, 100*time.Millisecond)
	case "sleep":
		d := time.Duration(num(1)) * time.Millisecond
		t0 := time.Now()
		w := papers3.LightSleep(d)
		lastWoke = "woke: " + w.String() + " after " + papers3.LastSleep().String() +
			", clock moved " + time.Since(t0).String()
		println(lastWoke)
	case "deepsleep":
		println("deep sleep", num(1), "s; slow clock", papers3.SlowClockHz(), "Hz")
		papers3.SetRetained(0, uint32(papers3.Now().Unix()))
		papers3.DeepSleep(time.Duration(num(1)) * time.Second)
	case "epd":
		// Full-screen refresh with timing statistics:
		// epd [quality|text|fast|fastest].
		mode := epd.ModeQuality
		if len(f) > 1 {
			switch f[1] {
			case "text":
				mode = epd.ModeText
			case "fast":
				mode = epd.ModeFast
			case "fastest":
				mode = epd.ModeFastest
			}
		}
		d := papers3.Display
		d.MarkAllDirty()
		d.ResetStats()
		err := d.Update(mode)
		fl, ir, ld := d.BenchFill()
		println("epd: err", err == nil, "frames", d.Frames, "time", d.UpdateUs/1000, "ms; late rows", d.LateRows,
			"; fill max", d.FillMaxUs, "us; bench cycles flash", fl, "iram", ir, "loads", ld)
		d.MarkAllDirty()
		d.Update(epd.ModeQuality)
	case "dozetest":
		// Doze N times although the USB host is there (the serial port
		// hangs meanwhile), then print what happened.
		n, allowed, after := num(1), ui.DozeAllowed, ui.DozeAfter
		ui.DozeAfter = time.Second
		ui.DozeAllowed = func() bool {
			if n <= 0 {
				ui.DozeAllowed, ui.DozeAfter = allowed, after
				println("dozetest: dozed", sys.Dozed.String(), "touch wakes", sys.TouchWakes,
					"time", papers3.Now().Format("15:04:05"))
				return false
			}
			n--
			return true
		}
	case "sleepinfo":
		rtc, dig, rtcS, digS := papers3.SleepVoltages()
		println("slow clock", papers3.SlowClockHz(), "Hz; dbias rtc", rtc, "dig", dig, "sleep", rtcS, digS,
			"; usb host", papers3.USBHost(), "; wake", status.Wake.String(), "; retained", papers3.Retained(0),
			papers3.Retained(1), "; reset cause", papers3.ResetCause())
		println("last light sleep:", lastWoke)
	default:
		println("unknown command", f[0])
	}
}

// shot prints the frame buffer as hex, one native row (960 pixels, 480
// bytes) per line, framed by "SHOT <rotation>" and "END".
func shot() {
	const digits = "0123456789abcdef"
	fb := papers3.Display.FrameBuffer()
	println("SHOT", int(papers3.Display.Rotation()))
	buf := make([]byte, 0, 961)
	for y := 0; y < 540; y++ {
		buf = buf[:0]
		for _, b := range fb[y*480 : (y+1)*480] {
			buf = append(buf, digits[b>>4], digits[b&15])
		}
		buf = append(buf, '\n')
		machine.Serial.Write(buf)
	}
	println("END")
}

// lastWoke describes the last "sleep" command (the serial port often drops
// output right after a light sleep).
var lastWoke = "none"
