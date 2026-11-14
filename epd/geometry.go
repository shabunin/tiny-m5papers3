// Package epd drives the 4.7" ED047TC1 e-paper panel (960x540, 16 gray
// levels) of the M5Stack PaperS3 directly from the ESP32-S3.
//
// The panel has no controller of its own: the MCU scans it like a CRT,
// frame after frame, telling every pixel on every frame whether to move
// towards black, towards white, or to stay put. On the ESP32-S3 this is done
// like M5GFX does it: the LCD_CAM peripheral (i8080 mode, 8 data lines,
// 16 MHz clock on CL) shifts out each row via GDMA, while the gate driver
// signals (SPV, CKV) and the source latch (LE) are plain GPIOs.
//
// On other platforms (go test, go run on a PC) the bus is simulated: frames
// are decoded and applied to an ink model that can be saved as an image.
package epd

// Native panel geometry. The source drivers take one row of Width pixels at a
// time (2 bits per pixel on the bus); the gate driver walks over Height rows.
const (
	Width  = 960
	Height = 540

	rowBytes    = Width / 4 // bus bytes per row (4 pixels per byte)
	linePadding = 8         // trailing no-op bytes per row, as in M5GFX
	lineBytes   = rowBytes + linePadding
	lineWords   = lineBytes / 4
)
