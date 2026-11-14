//go:build esp32s3

// wifiscan lists the WiFi networks around the PaperS3 on the serial port
// (a first test of the radio, without the display).
package main

import (
	"time"

	"tinygo.org/x/espradio"
)

func main() {
	time.Sleep(2 * time.Second)
	println("wifiscan: enabling radio")
	if err := espradio.Enable(espradio.Config{}); err != nil {
		println("enable:", err.Error())
		return
	}
	if err := espradio.Start(); err != nil {
		println("start:", err.Error())
		return
	}
	for {
		aps, err := espradio.Scan()
		if err != nil {
			println("scan:", err.Error())
		}
		println("wifiscan:", len(aps), "networks")
		for _, ap := range aps {
			println("  ", ap.RSSI, "dBm", ap.SSID)
		}
		time.Sleep(10 * time.Second)
	}
}
