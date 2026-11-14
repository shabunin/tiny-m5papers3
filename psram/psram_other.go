//go:build !esp32s3

package psram

// Size is the PSRAM size of the ESP32-S3R8.
const Size = 8 << 20

// On other platforms (simulator, tests) the "PSRAM" is ordinary memory.
var mem []byte

// Init allocates the simulated PSRAM.
func Init() error {
	if mem == nil {
		mem = make([]byte, Size)
	}
	return nil
}

// ModeRegisters returns zeros on other platforms.
func ModeRegisters() [6]byte { return [6]byte{} }

// Bytes returns the simulated PSRAM (nil before Init).
func Bytes() []byte { return mem }

var used int

// Alloc returns n zeroed bytes of the simulated PSRAM.
func Alloc(n int) []byte {
	if mem == nil {
		return nil
	}
	start := (used + 63) &^ 63
	if n < 0 || start+n > Size {
		return nil
	}
	used = start + n
	return mem[start : start+n : start+n]
}

// Used returns how many bytes Alloc has handed out.
func Used() int { return used }

// Writeback does nothing on other platforms.
func Writeback(b []byte) {}

// Invalidate does nothing on other platforms.
func Invalidate(b []byte) {}
