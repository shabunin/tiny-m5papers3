// Package radio gives the launcher WiFi (scan, connect, network time) and
// Bluetooth LE (scan for devices) on the ESP32-S3, through
// tinygo.org/x/espradio and tinygo.org/x/bluetooth. In the PC simulator it
// returns made-up results.
//
// All calls block (a WiFi scan takes about 4 s, connecting several seconds),
// so apps run them in a goroutine. Make all calls from the same long-lived
// goroutine: the drivers keep pointers into the caller's stack, and TinyGo's
// conservative garbage collector then cannot free the stack of a goroutine
// that has ended (~12 KB lost per call; see ui.radioJob). Once switched on,
// the radio stays on until the next reset: the driver cannot switch it off
// again.
package radio

import (
	"errors"
	"sort"
)

// Network is a WiFi access point found by ScanWiFi.
type Network struct {
	SSID string // "" for a hidden network
	RSSI int    // signal strength in dBm: -30 is very strong, -90 very weak
}

// Device is a Bluetooth LE device found by ScanBLE.
type Device struct {
	Address string
	Name    string // "" if it did not advertise one
	RSSI    int
}

// ErrNoNetwork is returned by NetworkTime without a network name.
var ErrNoNetwork = errors.New("no WiFi network configured")

// NTPServer is asked for the time by NetworkTime.
var NTPServer = "pool.ntp.org"

// Bars turns a signal strength into 0-4 bars, like a phone does.
func Bars(rssi int) int {
	switch {
	case rssi >= -55:
		return 4
	case rssi >= -67:
		return 3
	case rssi >= -78:
		return 2
	case rssi >= -89:
		return 1
	}
	return 0
}

// sortNetworks sorts by signal, strongest first, and keeps only the
// strongest entry of each named network (several access points often share
// a name).
func sortNetworks(ns []Network) []Network {
	sort.SliceStable(ns, func(i, j int) bool { return ns[i].RSSI > ns[j].RSSI })
	seen := map[string]bool{}
	out := ns[:0]
	for _, n := range ns {
		if n.SSID != "" {
			if seen[n.SSID] {
				continue
			}
			seen[n.SSID] = true
		}
		out = append(out, n)
	}
	return out
}

// sortDevices sorts by signal, strongest first; named devices before
// unnamed ones of the same strength.
func sortDevices(ds []Device) []Device {
	sort.SliceStable(ds, func(i, j int) bool {
		if ds[i].RSSI != ds[j].RSSI {
			return ds[i].RSSI > ds[j].RSSI
		}
		return ds[i].Name != "" && ds[j].Name == ""
	})
	return ds
}
