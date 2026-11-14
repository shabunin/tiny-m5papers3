//go:build esp32s3

package radio

import (
	"errors"
	"sync"
	"time"

	"tinygo.org/x/bluetooth"
	nl "tinygo.org/x/drivers/netlink"
	"tinygo.org/x/espradio"
	link "tinygo.org/x/espradio/netlink"
)

// espradio (v0.5.0) has no WiFi/Bluetooth coexistence, so the radio does one
// thing at a time. Measured on the PaperS3:
//
//   - Bluetooth must start before WiFi is started for the first time.
//     Started after WiFi has run, it hears nothing, and WiFi finds nothing
//     from then on either. So enable starts both, Bluetooth first.
//   - A Bluetooth scan works while WiFi is started, but the next WiFi scan
//     then times out. So WiFi is stopped for a Bluetooth scan, and started
//     again (5 ms) when it is needed.
//   - WiFi and Bluetooth together take about 58 KB of the drivers' memory
//     arena (WiFi alone 30 KB): more than the default of 48 KB.
//
// mu lets one app at a time use the radio.
const arenaSize = 96 << 10

var (
	mu       sync.Mutex
	radioOn  bool
	radioErr error // espradio.Enable failed: only a reset helps
	bleErr   error // Bluetooth did not start (WiFi may still work)
	wifiOn   bool  // WiFi driver started

	esplink = link.Esplink{ArenaPoolSize: arenaSize}
	address string // IP address of NetworkTime's last connection
)

// Active reports whether the radio was switched on (it then stays on, and
// the chip must not go to sleep).
func Active() bool { return radioOn }

// enable switches the radio on, with Bluetooth started and WiFi not.
func enable() error {
	if radioOn || radioErr != nil {
		return radioErr
	}
	err := espradio.Enable(espradio.Config{ArenaPoolSize: arenaSize})
	if err != nil && !errors.Is(err, espradio.ErrAlreadyEnabled) {
		radioErr = err
		return err
	}
	radioOn = true
	bleErr = bluetooth.DefaultAdapter.Enable()
	return nil
}

func startWiFi() error {
	if err := enable(); err != nil {
		return err
	}
	if wifiOn {
		return nil
	}
	if err := espradio.Start(); err != nil {
		return err
	}
	wifiOn = true
	return nil
}

// stopWiFi frees the radio for Bluetooth.
func stopWiFi() {
	if !wifiOn {
		return
	}
	espradio.Stop()
	wifiOn = false
}

// ScanWiFi lists the WiFi networks in range, strongest first.
func ScanWiFi() ([]Network, error) {
	mu.Lock()
	defer mu.Unlock()
	if err := startWiFi(); err != nil {
		return nil, err
	}
	aps, err := espradio.Scan()
	if err != nil {
		return nil, err
	}
	ns := make([]Network, 0, len(aps))
	for _, ap := range aps {
		ns = append(ns, Network{SSID: ap.SSID, RSSI: ap.RSSI})
	}
	return sortNetworks(ns), nil
}

// NetworkTime connects to the WiFi network ssid (DHCP), asks NTPServer for
// the time and leaves the network again. It returns the time in UTC.
func NetworkTime(ssid, password string) (time.Time, error) {
	if ssid == "" {
		return time.Time{}, ErrNoNetwork
	}
	mu.Lock()
	defer mu.Unlock()
	if err := startWiFi(); err != nil {
		return time.Time{}, err
	}
	address = ""
	if err := connect(ssid, password); err != nil {
		return time.Time{}, err
	}
	// Leave when done. An access point keeps the session of a station that
	// just goes away (a reset, for example) for minutes, and the station's
	// next WPA handshake then times out (measured: the first try after
	// every reset failed while the board had stayed connected).
	defer esplink.NetDisconnect()
	if a, err := esplink.Addr(); err == nil {
		address = a.String()
	}
	offset, err := esplink.NTP(NTPServer, 5*time.Second, 3)
	if err != nil {
		return time.Time{}, err
	}
	return time.Now().Add(offset).UTC(), nil
}

// connectTries is how often connect tries to join a network. A try fails
// now and then (the WPA handshake times out, for example right after a
// reset that left the access point with a stale session) and the next one
// works.
const connectTries = 3

// errAPNotFound is the driver's "AP not found": the driver scanned all
// channels, so trying again does not help.
var errAPNotFound = espradio.Error(201)

// connect joins the network ssid and gets an address by DHCP.
func connect(ssid, password string) (err error) {
	for try := 1; try <= connectTries; try++ {
		forgetAddress()
		err = esplink.NetConnect(&nl.ConnectParams{Ssid: ssid, Passphrase: password})
		if err == nil || errors.Is(err, errAPNotFound) {
			return err
		}
		println("radio: connect try", try, "failed:", err.Error())
		esplink.NetDisconnect()
	}
	return err
}

// forgetAddress makes the network stack, if it got an address before, ask
// for one like a new stack. espradio keeps the stack across connections and
// only asks for a new lease, but once the stack has an address it drops
// broadcast packets (espradio does not set lneto's AcceptIPv4Broadcast), and
// routers broadcast their DHCP replies: without this, every connection after
// the first timed out in DHCP.
func forgetAddress() {
	s := esplink.StackAsync()
	if s.Addr4() == ([4]byte{}) {
		return
	}
	s.SetAddr4([4]byte{}) // no address: lneto takes every packet
	s.SetGatewayHardwareAddr([6]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff})
}

// Address returns the IP address NetworkTime's last connection got, or "".
func Address() string { return address }

// ScanBLE listens for Bluetooth LE advertisements for d and returns the
// devices heard, strongest first. It stops WiFi.
func ScanBLE(d time.Duration) ([]Device, error) {
	mu.Lock()
	defer mu.Unlock()
	if err := enable(); err != nil {
		return nil, err
	}
	if bleErr != nil {
		return nil, bleErr
	}
	stopWiFi()
	a := bluetooth.DefaultAdapter
	found := map[string]int{}
	var ds []Device
	stop := time.AfterFunc(d, func() { a.StopScan() })
	defer stop.Stop()
	err := a.Scan(func(_ *bluetooth.Adapter, r bluetooth.ScanResult) {
		addr := r.Address.String()
		i, ok := found[addr]
		if !ok {
			i = len(ds)
			found[addr] = i
			ds = append(ds, Device{Address: addr, RSSI: int(r.RSSI)})
		}
		ds[i].RSSI = int(r.RSSI)
		if n := r.LocalName(); n != "" {
			ds[i].Name = n
		}
	})
	if err != nil {
		return nil, err
	}
	return sortDevices(ds), nil
}

// ArenaStats returns how much of the drivers' memory arena is in use.
func ArenaStats() (used, capacity int) {
	if !radioOn {
		return 0, 0
	}
	u, c := espradio.ArenaStats()
	return int(u), int(c)
}
