//go:build !esp32s3

package radio

import "time"

// SimDelay is how long the simulated scans and connections take.
var SimDelay time.Duration

// Active reports whether the radio was switched on (never in the simulator).
func Active() bool { return false }

// ScanWiFi returns made-up networks.
func ScanWiFi() ([]Network, error) {
	time.Sleep(SimDelay)
	return sortNetworks([]Network{
		{"Home", -42}, {"Home", -71}, {"Neighbour 5G", -66}, {"", -80},
		{"Coffee Shop", -77}, {"Printer-Setup", -85}, {"Office", -58},
	}), nil
}

// NetworkTime returns the simulated time (UTC) after a pretend connection.
func NetworkTime(ssid, password string) (time.Time, error) {
	if ssid == "" {
		return time.Time{}, ErrNoNetwork
	}
	time.Sleep(SimDelay)
	return time.Date(2026, 11, 14, 10, 30, 0, 0, time.UTC), nil
}

// Address returns a made-up address.
func Address() string { return "192.168.1.50" }

// ScanBLE returns made-up devices.
func ScanBLE(d time.Duration) ([]Device, error) {
	time.Sleep(SimDelay)
	return sortDevices([]Device{
		{"A4:C1:38:12:34:56", "Thermometer", -61},
		{"5F:22:9A:01:EE:10", "", -88},
		{"C8:2B:96:AA:00:01", "Headphones", -54},
		{"7B:10:44:9C:3D:2E", "", -73},
	}), nil
}

// ArenaStats returns 0, 0: the simulator has no radio drivers.
func ArenaStats() (used, capacity int) { return 0, 0 }
