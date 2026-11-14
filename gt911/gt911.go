// Package gt911 is a driver for the Goodix GT911 capacitive touch controller,
// as used in the M5Stack PaperS3 (I2C: SDA GPIO41, SCL GPIO42, INT GPIO48).
//
// The controller scans on its own; Read polls its status register over I2C
// and returns up to 5 touch points.
package gt911

import (
	"errors"

	"tinygo.org/x/drivers/touch"
)

// I2C is the bus the controller is on (satisfied by *machine.I2C).
type I2C interface {
	Tx(addr uint16, w, r []byte) error
}

// The GT911 answers at one of two addresses, depending on the level of its
// INT pin when it came out of reset.
const (
	Address    = 0x5D
	AddressAlt = 0x14
)

// Registers.
const (
	regConfigVersion = 0x8047
	regXMax          = 0x8048 // configured output range, little endian
	regYMax          = 0x804A
	regTouchNumber   = 0x804C
	regProductID     = 0x8140 // "911\0"
	regFirmware      = 0x8144
	regStatus        = 0x814E // bit 7: buffer ready, bits 0-3: number of points
	regPoints        = 0x814F // 8 bytes per point
)

// MaxPoints is the number of simultaneous touches the GT911 can report.
const MaxPoints = 5

// ErrNotFound is returned by Configure if no GT911 answers.
var ErrNotFound = errors.New("gt911: not found")

// Point is one touch.
type Point struct {
	ID   uint8 // track ID, stable while the finger stays down
	X, Y int16 // in the controller's coordinate range (see Range)
	Size uint16
}

// Device is a GT911 touch controller.
type Device struct {
	bus     I2C
	Address uint16

	wbuf   [3]byte
	rbuf   [8 * MaxPoints]byte
	points [MaxPoints]Point
	n      int
}

// New returns a GT911 on the given bus. Call Configure before use.
func New(bus I2C) *Device {
	return &Device{bus: bus}
}

// Configure finds the controller on either of its addresses (unless Address
// is already set) and checks its product ID.
func (d *Device) Configure() error {
	addrs := []uint16{AddressAlt, Address}
	if d.Address != 0 {
		addrs = []uint16{d.Address}
	}
	for _, a := range addrs {
		d.Address = a
		if id, err := d.ProductID(); err == nil && id == "911" {
			return nil
		}
	}
	d.Address = 0
	return ErrNotFound
}

func (d *Device) read(reg uint16, buf []byte) error {
	d.wbuf[0], d.wbuf[1] = byte(reg>>8), byte(reg)
	return d.bus.Tx(d.Address, d.wbuf[:2], buf)
}

// ReadRegisters reads len(buf) bytes starting at register reg (for
// diagnostics; see the GT911 programming guide for the register map).
func (d *Device) ReadRegisters(reg uint16, buf []byte) error { return d.read(reg, buf) }

func (d *Device) write8(reg uint16, v byte) error {
	d.wbuf[0], d.wbuf[1], d.wbuf[2] = byte(reg>>8), byte(reg), v
	return d.bus.Tx(d.Address, d.wbuf[:3], nil)
}

func (d *Device) read16(reg uint16) (uint16, error) {
	var b [2]byte
	err := d.read(reg, b[:])
	return uint16(b[0]) | uint16(b[1])<<8, err
}

// ProductID returns the product ID string ("911").
func (d *Device) ProductID() (string, error) {
	var b [4]byte
	if err := d.read(regProductID, b[:]); err != nil {
		return "", err
	}
	n := 0
	for n < len(b) && b[n] != 0 {
		n++
	}
	return string(b[:n]), nil
}

// Firmware returns the firmware version.
func (d *Device) Firmware() (uint16, error) { return d.read16(regFirmware) }

// ConfigVersion returns the version of the configuration stored in the chip.
func (d *Device) ConfigVersion() (uint8, error) {
	var b [1]byte
	err := d.read(regConfigVersion, b[:])
	return b[0], err
}

// Range returns the configured coordinate range: X goes from 0 to xmax-1, Y
// from 0 to ymax-1.
func (d *Device) Range() (xmax, ymax uint16, err error) {
	if xmax, err = d.read16(regXMax); err != nil {
		return
	}
	ymax, err = d.read16(regYMax)
	return
}

// Read returns the current touch points; an empty slice means no finger.
// The slice is only valid until the next call. If the controller has no new
// scan yet, the previous state is returned.
func (d *Device) Read() ([]Point, error) {
	var st [1]byte
	if err := d.read(regStatus, st[:]); err != nil {
		return nil, err
	}
	if st[0]&0x80 == 0 {
		return d.points[:d.n], nil
	}
	n := min(int(st[0]&0x0f), MaxPoints)
	if n > 0 {
		if err := d.read(regPoints, d.rbuf[:8*n]); err != nil {
			return nil, err
		}
		for i := 0; i < n; i++ {
			p := d.rbuf[8*i : 8*i+8]
			d.points[i] = Point{
				ID:   p[0],
				X:    int16(uint16(p[1]) | uint16(p[2])<<8),
				Y:    int16(uint16(p[3]) | uint16(p[4])<<8),
				Size: uint16(p[5]) | uint16(p[6])<<8,
			}
		}
	}
	d.n = n
	// Acknowledge, so the controller can store the next scan.
	return d.points[:d.n], d.write8(regStatus, 0)
}

// ReadTouchPoint implements tinygo.org/x/drivers/touch.Pointer: the first
// touch point, with Z = 0 when nothing touches the screen.
func (d *Device) ReadTouchPoint() touch.Point {
	pts, err := d.Read()
	if err != nil || len(pts) == 0 {
		return touch.Point{}
	}
	return touch.Point{X: int(pts[0].X), Y: int(pts[0].Y), Z: int(pts[0].Size) + 1}
}
