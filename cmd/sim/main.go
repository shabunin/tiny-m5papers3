// sim runs the launcher on the PC with a simulated PaperS3 and saves a
// screenshot of every screen, driven by a scripted tour (taps, strokes,
// sensor values).
//
//	go run ./cmd/sim -out /tmp/shots
//
// Each screenshot is the frame buffer (what the panel should show); with
// -ink, the simulated ink is saved as well (what the waveforms produced:
// pixels driven in a fast mode show up as pure black or white).
package main

import (
	"flag"
	"image"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"time"

	"tiny-m5papers3/papers3"
	"tiny-m5papers3/ui"
)

var (
	out = flag.String("out", "sim-out", "directory for the screenshots")
	ink = flag.Bool("ink", false, "also save the simulated ink")
	sys *ui.System
)

func main() {
	flag.Parse()
	if err := os.MkdirAll(*out, 0o755); err != nil {
		log.Fatal(err)
	}
	papers3.Configure()
	papers3.SimLog = func(s string) { log.Println("board:", s) }
	sys = ui.NewLauncher(papers3.Display)
	sys.Log = func(s string) { log.Println(s) }

	step()
	shot("00-home")

	open(0) // Paint
	drag(80, 400, 200, 330, 330, 420, 460, 340)
	tap(tool(4), 64+98) // gray ink
	tap(tool(2), 64+98) // large pen
	drag(90, 600, 450, 640)
	tap(tool(5), 64+98) // eraser
	drag(270, 300, 270, 700)
	shot("01-paint")
	home()

	open(1) // Fractal
	shot("02-fractal")
	tap(140, 470)
	shot("03-fractal-zoom")
	home()

	open(2) // Life
	shot("04-life")
	tap(124+52, 960-76+38) // Run
	for range 30 {
		step()
	}
	tap(124+52, 960-76+38) // Pause
	shot("05-life-run")
	home()

	papers3.SimAccel = [3]float32{0.12, -0.06, 0.99}
	open(3) // Level
	for range 10 {
		papers3.SimTime = papers3.SimTime.Add(60 * time.Millisecond)
		step()
	}
	shot("06-level")
	papers3.SimAccel = [3]float32{0.002, 0.004, 1}
	for range 10 {
		papers3.SimTime = papers3.SimTime.Add(60 * time.Millisecond)
		step()
	}
	shot("07-level-flat")
	home()

	open(4) // Grays
	shot("08-grays")
	home()

	open(5) // Reader
	shot("09-reader")
	tap(400, 500)
	shot("10-reader-2")
	home()

	open(6) // Touch
	papers3.SimTouches = []papers3.Touch{{ID: 0, X: 150, Y: 300}, {ID: 1, X: 380, Y: 520}}
	step()
	papers3.SimTouches = []papers3.Touch{{ID: 0, X: 160, Y: 330}, {ID: 1, X: 370, Y: 560}}
	step()
	shot("11-touch")
	papers3.SimTouches = nil
	step()
	home()

	open(7) // Clock
	shot("12-clock")
	papers3.SimTime = papers3.SimTime.Add(time.Minute)
	step()
	shot("13-clock-next-minute")
	home()

	open(8) // WiFi (simulated networks; Set clock without a network)
	for range 5 {
		time.Sleep(time.Millisecond) // let the scan goroutine finish
		step()
	}
	shot("14-wifi")
	home()

	open(9) // Bluetooth
	for range 5 {
		time.Sleep(time.Millisecond)
		step()
	}
	shot("15-bluetooth")
	home()

	open(10) // System
	shot("16-system")
	tap(140, 960-136+27) // Sound
	tap(400, 960-136+27) // Flip
	shot("17-system-flipped")
	// The finger is in panel coordinates: on the flipped screen the button
	// is at the mirrored place.
	tap(539-400, 959-(960-136+27)) // flip back
	home()
	shot("18-home-again")
}

// step runs one loop iteration with the current simulated touches.
func step() {
	sys.Step(papers3.Now(), papers3.ReadTouches(nil))
	papers3.SimTime = papers3.SimTime.Add(30 * time.Millisecond)
}

func touch(x, y int) []papers3.Touch { return []papers3.Touch{{X: int16(x), Y: int16(y)}} }

func tap(x, y int) {
	papers3.SimTouches = touch(x, y)
	step()
	step()
	papers3.SimTouches = nil
	step()
}

// drag moves one finger along a polyline (pairs of coordinates).
func drag(pts ...int) {
	for i := 0; i+3 < len(pts); i += 2 {
		x0, y0, x1, y1 := pts[i], pts[i+1], pts[i+2], pts[i+3]
		for k := 0; k <= 8; k++ {
			papers3.SimTouches = touch(x0+(x1-x0)*k/8, y0+(y1-y0)*k/8)
			step()
		}
	}
	papers3.SimTouches = nil
	step()
}

// open taps the launcher tile of app i.
func open(i int) {
	tap(ui.TileCenter(i, len(sys.Apps)))
}

func home() { tap(60, 30) }

// tool is the x of Paint's tool button i.
func tool(i int) int { return 12 + i*87 + 39 }

func shot(name string) {
	save(name, papers3.Display.FrameBufferImage())
	if *ink {
		save(name+"-ink", papers3.Display.InkImage())
	}
}

func save(name string, img image.Image) {
	f, err := os.Create(filepath.Join(*out, name+".png"))
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		log.Fatal(err)
	}
}
