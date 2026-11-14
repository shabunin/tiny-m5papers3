//go:build esp32s3

// Package psram switches on the 8 MB octal PSRAM inside the ESP32-S3R8 of the
// M5Stack PaperS3 and maps it into the data address space, so that Go code
// can use it as plain memory (see Bytes).
//
// The register sequence follows ESP-IDF v5.3 (components/esp_psram/esp32s3/
// esp_psram_impl_octal.c and esp_hw_support/mspi_timing_*.c), without the
// timing tuning: the PSRAM runs at a clock that needs none.
package psram

/*
#include <stdint.h>
#include <stdbool.h>

#define PSRAM_REG(a) (*(volatile uint32_t *)(a))
#define PSRAM_SPI0 0x60003000u
#define PSRAM_SPI1 0x60002000u
#define PSRAM_CACHE_STATE 0x600c4130u // EXTMEM_CACHE_STATE_REG, 1 = idle
#define PSRAM_IO_MUX_GPIO0 0x60009004u
#define PSRAM_IO_MUX_GPIO26 0x6000906cu

typedef void (*psram_exec_t)(int spi, int mode, uint32_t cmd, int cmd_bits,
	uint32_t addr, int addr_bits, int dummy_bits, uint8_t *mosi, int mosi_bits,
	uint8_t *miso, int miso_bits, uint32_t cs_mask, bool is_write_erase);
#define PSRAM_EXEC ((psram_exec_t)0x400008b8) // esp_rom_opiflash_exec_cmd

#define PSRAM_OPI_DTR 7         // ESP_ROM_SPIFLASH_OPI_DTR_MODE
#define PSRAM_REG_READ 0x4040   // OPI_PSRAM_REG_READ
#define PSRAM_REG_WRITE 0xc0c0  // OPI_PSRAM_REG_WRITE
#define PSRAM_RD_DUMMY 18       // 2*(10-1)
#define PSRAM_WR_DUMMY 8        // 2*(5-1)
#define PSRAM_CS1 (1u << 1)

// psram_init talks to the PSRAM chip over SPI1 (mode registers), sets up
// SPI0 so that the data cache can read and write it, and maps pages 64 KB
// pages of it at vaddr. mr receives MR0-MR3, MR4 and MR8. It returns 1 on
// success. It runs from internal RAM with the caches suspended: SPI1 and
// the caches share the memory bus.
__attribute__((section(".iram1.psram_init"), noinline))
static uint32_t psram_init(uint8_t *mr, uint32_t vaddr, uint32_t pages, uint32_t div) {
	uint32_t ic = ((uint32_t (*)(void))0x4000189c)(); // rom_Cache_Suspend_ICache
	while ((PSRAM_REG(PSRAM_CACHE_STATE) & 0xfff) != 1) {
	}
	uint32_t dc = ((uint32_t (*)(void))0x400018b4)(); // rom_Cache_Suspend_DCache
	while (((PSRAM_REG(PSRAM_CACHE_STATE) >> 12) & 0xfff) != 1) {
	}

	// Octal data pins SPIIO4-7 and DQS (GPIO33-37), from the eFuse settings
	// (esp_mspi_pin_init: esp_rom_opiflash_pin_config).
	((void (*)(void))0x40000894)();
	// All memory SPI pins at drive strength 3 (mspi_timing_ll_set_all_pin_drive).
	PSRAM_REG(PSRAM_SPI0 + 0x3fc) = (PSRAM_REG(PSRAM_SPI0 + 0x3fc) & ~0xfu) | (1u << 4) | (3u << 2) | 3u;
	for (uint32_t pin = 27; pin <= 37; pin++) {
		if (pin == 29 || pin == 30) {
			continue; // CS0 and clock
		}
		uint32_t a = PSRAM_IO_MUX_GPIO0 + 4 * pin;
		PSRAM_REG(a) = (PSRAM_REG(a) & ~(3u << 10)) | (3u << 10);
	}

	// CS1 on GPIO26 (function 0, SPICS1), drive strength 3; PSRAM clock
	// drive strength 3 (s_init_psram_pins).
	PSRAM_REG(PSRAM_IO_MUX_GPIO26) = (PSRAM_REG(PSRAM_IO_MUX_GPIO26) & ~((7u << 12) | (3u << 10))) | (3u << 10);
	PSRAM_REG(PSRAM_SPI0 + 0x3fc) = (PSRAM_REG(PSRAM_SPI0 + 0x3fc) & ~3u) | 3u;
	// CS setup/hold (s_set_psram_cs_timing; SPI0 and SPI1 share it).
	uint32_t ac = PSRAM_REG(PSRAM_SPI0 + 0xdc);
	ac |= 3u;                                  // CS_HOLD, CS_SETUP
	ac = (ac & ~(0x1fu << 7)) | (3u << 7);     // CS_HOLD_TIME
	ac = (ac & ~(0x1fu << 2)) | (3u << 2);     // CS_SETUP_TIME
	ac = (ac & ~(0x3fu << 25)) | (2u << 25);   // CS_HOLD_DELAY
	PSRAM_REG(PSRAM_SPI0 + 0xdc) = ac;

	// SPI1 at a quarter of the MSPI core clock (20-60 MHz) for the register
	// commands.
	uint32_t clk1 = PSRAM_REG(PSRAM_SPI1 + 0x14);
	PSRAM_REG(PSRAM_SPI1 + 0x14) = (3u << 16) | (1u << 8) | 3u;
	PSRAM_REG(PSRAM_SPI1 + 0xe0) |= 1u << 1; // SPI_FMEM_VAR_DUMMY
	((void (*)(int, bool, bool))0x4000093c)(1, false, false); // esp_rom_spi_set_dtr_swap_mode

	// MR0: fixed latency, read latency code 2 (10 cycles), drive strength 0.
	uint8_t buf[2] = {0, 0};
	PSRAM_EXEC(1, PSRAM_OPI_DTR, PSRAM_REG_READ, 16, 0, 32, PSRAM_RD_DUMMY, 0, 0, buf, 16, PSRAM_CS1, false);
	buf[0] = (buf[0] & ~0x3fu) | (1u << 5) | (2u << 2);
	PSRAM_EXEC(1, PSRAM_OPI_DTR, PSRAM_REG_WRITE, 16, 0, 32, 0, buf, 16, 0, 0, PSRAM_CS1, false);
	PSRAM_EXEC(1, PSRAM_OPI_DTR, PSRAM_REG_READ, 16, 0, 32, PSRAM_RD_DUMMY, 0, 0, &mr[0], 16, PSRAM_CS1, false);
	PSRAM_EXEC(1, PSRAM_OPI_DTR, PSRAM_REG_READ, 16, 2, 32, PSRAM_RD_DUMMY, 0, 0, &mr[2], 16, PSRAM_CS1, false);
	PSRAM_EXEC(1, PSRAM_OPI_DTR, PSRAM_REG_READ, 16, 4, 32, PSRAM_RD_DUMMY, 0, 0, &mr[4], 8, PSRAM_CS1, false);
	PSRAM_EXEC(1, PSRAM_OPI_DTR, PSRAM_REG_READ, 16, 8, 32, PSRAM_RD_DUMMY, 0, 0, &mr[5], 8, PSRAM_CS1, false);
	PSRAM_REG(PSRAM_SPI1 + 0x14) = clk1;

	uint32_t result = 0;
	if ((mr[1] & 0x1f) == 0x0d) { // vendor ID: AP Memory
		// PSRAM clock of SPI0: core clock / div.
		PSRAM_REG(PSRAM_SPI0 + 0x50) = div == 1 ? (1u << 31) : ((div - 1) << 16) | ((div / 2 - 1) << 8) | (div - 1);

		// SPI0 command, address and dummy phases for cache access
		// (s_config_psram_spi_phases).
		uint32_t sc = PSRAM_REG(PSRAM_SPI0 + 0x40); // CACHE_SCTRL
		sc |= (1u << 20) | (1u << 5);               // CACHE_SRAM_USR_WCMD, RCMD
		sc = (sc & ~(0x3fu << 14)) | (31u << 14);   // SRAM_ADDR_BITLEN: 32 bits
		sc |= 1u;                                   // CACHE_USR_SCMD_4BYTE
		sc |= (1u << 4) | (1u << 3);                // USR_RD/WR_SRAM_DUMMY
		sc = (sc & ~(0x3fu << 6)) | ((PSRAM_RD_DUMMY - 1) << 6);
		sc = (sc & ~(0x3fu << 22)) | ((PSRAM_WR_DUMMY - 1) << 22);
		sc |= 1u << 21;                             // SRAM_OCT
		PSRAM_REG(PSRAM_SPI0 + 0x40) = sc;
		PSRAM_REG(PSRAM_SPI0 + 0x4c) = (15u << 28) | 0x8080; // SRAM_DWR_CMD: sync write
		PSRAM_REG(PSRAM_SPI0 + 0x48) = (15u << 28) | 0x0000; // SRAM_DRD_CMD: sync read
		uint32_t ddr = PSRAM_REG(PSRAM_SPI0 + 0xe4);        // SPI_SMEM_DDR
		ddr |= 1u << 1;                                     // SPI_SMEM_VAR_DUMMY
		ddr &= ~((1u << 3) | (1u << 2));                    // no data byte swap
		ddr |= 1u;                                          // SPI_SMEM_DDR_EN
		PSRAM_REG(PSRAM_SPI0 + 0xe4) = ddr;
		// SDUMMY_OUT, SCMD/SADDR/SDOUT/SDIN octal.
		PSRAM_REG(PSRAM_SPI0 + 0x44) |= (1u << 22) | (1u << 21) | (1u << 20) | (1u << 19) | (1u << 18);

		// Map the PSRAM into the data bus (MMU_ACCESS_SPIRAM, 64 KB pages).
		int err = ((int (*)(uint32_t, uint32_t, uint32_t, uint32_t, uint32_t, uint32_t))0x400019b0)(
			1u << 15, vaddr, 0, 64, pages, 0); // Cache_Dbus_MMU_Set
		result = err == 0 ? 1 : 0x100 | err;
	}

	((void (*)(uint32_t))0x400018c0)(dc); // Cache_Resume_DCache
	((void (*)(uint32_t))0x400018a8)(ic); // Cache_Resume_ICache
	return result;
}

struct psram_autoload_config {
	uint8_t ena, order, trigger, size;
};
struct psram_autoload_region_config {
	uint8_t region, ena;
	uint32_t addr, size;
};

// psram_autoload makes the data cache fetch the following cache lines in
// the background after a miss in [addr, addr+size), for sequential reads.
static int psram_autoload(uint32_t addr, uint32_t size, uint32_t trigger, uint32_t lines) {
	struct psram_autoload_config c = {1, 0, (uint8_t)trigger, (uint8_t)lines};
	struct psram_autoload_region_config r = {0, 1, addr, size};
	((void (*)(void))0x400017e8)(); // Cache_Disable_DCache_Autoload
	((void (*)(const struct psram_autoload_config *))0x400017c4)(&c); // Cache_Config_DCache_Autoload
	int err = ((int (*)(const struct psram_autoload_region_config *))0x400017d0)(&r); // Cache_Config_DCache_Region_Autoload
	((void (*)(void))0x400017dc)(); // Cache_Enable_DCache_Autoload
	return err;
}

// psram_invalidate drops cached lines of [addr, addr+size) without writing
// them back (Cache_Invalidate_Addr).
static void psram_invalidate(uint32_t addr, uint32_t size) {
	((void (*)(uint32_t, uint32_t))0x400016b0)(addr, size);
}

// psram_writeback writes dirty cached lines of [addr, addr+size) to the
// PSRAM (rom_Cache_WriteBack_Addr), for DMA.
static void psram_writeback(uint32_t addr, uint32_t size) {
	((void (*)(uint32_t, uint32_t))0x400016c8)(addr, size);
}
*/
import "C"

import (
	"device/esp"
	"errors"
	"runtime/interrupt"
	"unsafe"
)

const (
	// Base is the address where the PSRAM appears (data bus, MMU entries
	// 256 and up, well above the flash data TinyGo maps from 0x3C000000).
	Base = 0x3D000000
	// Size is the PSRAM size of the ESP32-S3R8.
	Size = 8 << 20

	pageSize = 64 << 10
)

// Errors returned by Init.
var (
	ErrNotFound = errors.New("psram: no AP Memory octal PSRAM found")
	ErrMap      = errors.New("psram: MMU mapping failed")
)

// ClockMHz is the PSRAM clock that Init sets up. ESP-IDF tunes the input
// timing for clocks above 40 MHz; this package does not.
var ClockMHz uint32 = 40

var (
	ready bool
	modes [6]byte
)

// coreClockMHz returns the MSPI core clock (SPI_MEM_CORE_CLK_SEL).
func coreClockMHz() uint32 {
	return [4]uint32{80, 120, 160, 240}[esp.SPI0.CORE_CLK_SEL.Get()&3]
}

// Init switches the PSRAM on (once). Afterwards Bytes returns it.
func Init() error {
	if ready {
		return nil
	}
	state := interrupt.Disable()
	div := max(coreClockMHz()/ClockMHz, 1)
	r := C.psram_init((*C.uint8_t)(unsafe.Pointer(&modes[0])), Base, Size/pageSize, C.uint32_t(div))
	interrupt.Restore(state)
	switch {
	case r == 1:
		ready = true
		return nil
	case r&0x100 != 0:
		return ErrMap
	}
	return ErrNotFound
}

// ModeRegisters returns the PSRAM mode registers MR0-MR3, MR4 and MR8 as read
// by Init (vendor ID in MR1, density in MR2).
func ModeRegisters() [6]byte { return modes }

// Bytes returns the whole PSRAM as a byte slice (nil before Init succeeded).
// The garbage collector does not look inside it: do not store the only
// pointer to a Go object there.
func Bytes() []byte {
	if !ready {
		return nil
	}
	return unsafe.Slice((*byte)(unsafe.Pointer(uintptr(Base))), Size)
}

// used is the number of bytes handed out by Alloc.
var used int

// Alloc returns n zeroed bytes of PSRAM, aligned to a 64-byte cache line, or
// nil if the PSRAM is not ready or full. The memory is never freed.
func Alloc(n int) []byte {
	if !ready {
		return nil
	}
	start := (used + 63) &^ 63
	if n < 0 || start+n > Size {
		return nil
	}
	used = start + n
	b := Bytes()[start : start+n : start+n]
	clear(b)
	return b
}

// Used returns how many bytes Alloc has handed out.
func Used() int { return used }

// Autoload makes the data cache read ahead after a miss in b (part of
// Bytes), which speeds up sequential reads. trigger: 0 on a miss, 1 on a hit,
// 2 both; lines: 0-3 (more lines per read-ahead).
func Autoload(b []byte, trigger, lines int) int {
	if len(b) == 0 {
		return -1
	}
	return int(C.psram_autoload(C.uint32_t(uintptr(unsafe.Pointer(&b[0]))), C.uint32_t(len(b)),
		C.uint32_t(trigger), C.uint32_t(lines)))
}

// Writeback makes sure data the CPU wrote to b (part of Bytes) has reached
// the PSRAM, before a DMA engine reads it.
func Writeback(b []byte) {
	if len(b) > 0 {
		C.psram_writeback(C.uint32_t(uintptr(unsafe.Pointer(&b[0]))), C.uint32_t(len(b)))
	}
}

// Invalidate drops cached copies of b, after a DMA engine wrote to it.
func Invalidate(b []byte) {
	if len(b) > 0 {
		C.psram_invalidate(C.uint32_t(uintptr(unsafe.Pointer(&b[0]))), C.uint32_t(len(b)))
	}
}
