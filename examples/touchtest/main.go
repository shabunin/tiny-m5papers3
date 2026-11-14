//go:build esp32s3

// touchtest scans the PaperS3's internal I2C bus, sets up the GT911 touch
// controller and prints touches over USB serial. It does not use the display.
package main

import (
	"machine"
	"strconv"
	"time"

	"tiny-m5papers3/gt911"
)

func main() {
	time.Sleep(3 * time.Second)

	bus := machine.I2C0
	err := bus.Configure(machine.I2CConfig{
		SDA:       machine.GPIO41,
		SCL:       machine.GPIO42,
		Frequency: 400 * machine.KHz,
	})
	if err != nil {
		println("i2c configure:", err.Error())
	}

	scan := "i2c devices:"
	var b [1]byte
	for a := uint16(0x08); a < 0x78; a++ {
		if bus.Tx(a, nil, b[:]) == nil {
			scan += " 0x" + strconv.FormatUint(uint64(a), 16)
		}
	}

	machine.GPIO48.Configure(machine.PinConfig{Mode: machine.PinInputPullup}) // touch INT

	ts := gt911.New(bus)
	info := ""
	if err := ts.Configure(); err != nil {
		info = "gt911: " + err.Error()
	} else {
		fw, _ := ts.Firmware()
		cv, _ := ts.ConfigVersion()
		xm, ym, _ := ts.Range()
		info = "gt911 at 0x" + strconv.FormatUint(uint64(ts.Address), 16) +
			" firmware 0x" + strconv.FormatUint(uint64(fw), 16) +
			" config v" + strconv.Itoa(int(cv)) +
			" range " + strconv.Itoa(int(xm)) + "x" + strconv.Itoa(int(ym))
	}

	last := time.Now()
	for {
		if time.Since(last) > 3*time.Second {
			println(scan)
			println(info)
			last = time.Now()
		}
		if ts.Address != 0 {
			pts, err := ts.Read()
			if err != nil {
				println("read:", err.Error())
			}
			for _, p := range pts {
				println("touch id", p.ID, "x", p.X, "y", p.Y, "size", p.Size)
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
}
