//go:build esp32s3

// bench measures the e-paper row fill loop (no panel activity).
package main

import (
	"time"

	"tiny-m5papers3/epd"
)

func main() {
	time.Sleep(2 * time.Second)
	d := epd.New()
	d.Configure()
	during, after := d.ProbeRowSignals()
	for {
		println("row signals [CKV LE SPH] during transfer:", during[0], during[1], during[2], " after:", after[0], after[1], after[2])
		f, i, l := d.BenchFill()
		println("fill cycles: flash", f, "iram", i, "loads", l, "(240 cycles = 1us)")
		time.Sleep(2 * time.Second)
	}
}
