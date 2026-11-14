//go:build esp32s3

// psramtest switches on the PSRAM, prints its mode registers and checks all
// 8 MB with a pattern test, then measures the speed (serial output only).
package main

import (
	"device/esp"
	"time"
	"unsafe"

	"tiny-m5papers3/psram"
)

func hex(v uint32) string {
	const d = "0123456789abcdef"
	b := []byte("0x00000000")
	for i := 0; i < 8; i++ {
		b[9-i] = d[v>>(4*i)&15]
	}
	return string(b)
}

func reg(name string, a uintptr) {
	println(name, hex((*(*uint32)(unsafe.Pointer(a)))))
}

func main() {
	time.Sleep(3 * time.Second)
	println("psramtest: before")
	reg("CORE_CLK_SEL", 0x600030ec)
	reg("SPI0 CLOCK  ", 0x60003014)
	reg("SPI0 SRAMCLK", 0x60003050)
	reg("CACHE_SCTRL ", 0x60003040)
	reg("SRAM_CMD    ", 0x60003044)
	reg("SRAM_DRD_CMD", 0x60003048)
	reg("SRAM_DWR_CMD", 0x6000304c)
	reg("SMEM_AC     ", 0x600030dc)
	reg("SMEM_DDR    ", 0x600030e4)
	reg("SPI0 DDR    ", 0x600030e0)
	reg("SPI0 USER   ", 0x60003018)
	reg("SPI0 CTRL   ", 0x60003008)
	reg("SPI1 CLOCK  ", 0x60002014)
	for pin := uintptr(26); pin <= 37; pin++ {
		reg("IO_MUX GPIO"+string(rune('0'+pin/10))+string(rune('0'+pin%10)), 0x60009004+4*pin)
	}
	reg("SPI0 DATE   ", 0x600033fc)
	reg("DCACHE_CTRL1", 0x600c4004)
	for _, e := range []uintptr{0, 1, 2, 3, 4, 5, 6, 7, 8, 255, 256} {
		reg("MMU entry "+string(rune('0'+e/100%10))+string(rune('0'+e/10%10))+string(rune('0'+e%10)), 0x600c5000+4*e)
	}
	_ = esp.SPI0

	err := psram.Init()
	mr := psram.ModeRegisters()
	println("psram.Init:", err == nil, "MR0", hex(uint32(mr[0])), "MR1", hex(uint32(mr[1])), "MR2", hex(uint32(mr[2])),
		"MR3", hex(uint32(mr[3])), "MR4", hex(uint32(mr[4])), "MR8", hex(uint32(mr[5])))
	if err != nil {
		println("error:", err.Error())
		for pin := uintptr(26); pin <= 37; pin++ {
			reg("IO_MUX GPIO"+string(rune('0'+pin/10))+string(rune('0'+pin%10)), 0x60009004+4*pin)
		}
		for {
			time.Sleep(time.Hour)
		}
	}
	reg("MMU entry 256", 0x600c5000+4*256)
	for pin := uintptr(26); pin <= 37; pin++ {
		reg("IO_MUX GPIO"+string(rune('0'+pin/10))+string(rune('0'+pin%10)), 0x60009004+4*pin)
	}
	reg("SPI0 SRAMCLK", 0x60003050)

	mem := psram.Bytes()
	words := unsafe.Slice((*uint32)(unsafe.Pointer(&mem[0])), len(mem)/4)
	// Quick check of the first words.
	words[0] = 0x12345678
	words[1] = 0x9abcdef0
	println("first words", hex(words[0]), hex(words[1]))

	t0 := time.Now()
	for i := range words {
		words[i] = uint32(i)*2654435761 ^ 0x5a5a5a5a
	}
	tw := time.Since(t0)
	t0 = time.Now()
	bad := 0
	for i := range words {
		if words[i] != uint32(i)*2654435761^0x5a5a5a5a {
			if bad < 5 {
				println("mismatch at", i*4, hex(words[i]), "want", hex(uint32(i)*2654435761^0x5a5a5a5a))
			}
			bad++
		}
	}
	tr := time.Since(t0)
	println("pattern test: errors", bad, "of", len(words), "words; write", tw.Milliseconds(), "ms, read+check", tr.Milliseconds(), "ms")

	reg("DCACHE_CTRL ", 0x600c4000)
	reg("AUTOLOAD    ", 0x600c404c)
	speed := func(label string) {
		// Sequential read speed over 1 MB, cache flushed first.
		psram.Invalidate(mem[:1<<20])
		t0 := time.Now()
		var sum uint32
		for _, w := range words[:1<<18] {
			sum += w
		}
		d := time.Since(t0)
		println(label, "read 1 MB:", d.Microseconds(), "us =", (1<<20)/max(d.Microseconds(), 1), "MB/s", hex(sum))
	}
	speed("no autoload")
	for trig := 0; trig <= 2; trig++ {
		for lines := 0; lines <= 3; lines++ {
			e := psram.Autoload(mem[:2<<20], trig, lines)
			speed("trigger " + string(rune('0'+trig)) + " lines " + string(rune('0'+lines)) + " err " + string(rune('0'+e)))
		}
	}
	reg("AUTOLOAD    ", 0x600c404c)
	for {
		time.Sleep(time.Hour)
	}
}
