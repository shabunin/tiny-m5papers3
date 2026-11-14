package papers3

import (
	"testing"
	"time"
)

func TestRTCRoundTrip(t *testing.T) {
	for _, want := range []time.Time{
		time.Date(2026, 11, 13, 22, 31, 14, 0, time.UTC),
		time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2099, 12, 31, 23, 59, 59, 0, time.UTC),
		time.Date(2100, 3, 1, 12, 0, 0, 0, time.UTC),
	} {
		r := encodeRTC(want)
		got, valid := decodeRTC(r[:])
		if !valid || !got.Equal(want) {
			t.Errorf("%v: got %v valid=%v (regs % x)", want, got, valid, r)
		}
	}
	// VL flag set: time not valid.
	r := encodeRTC(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	r[0] |= 0x80
	if _, valid := decodeRTC(r[:]); valid {
		t.Error("VL flag ignored")
	}
}

func TestPercent(t *testing.T) {
	prev := -1
	for mv := 3000; mv <= 4400; mv += 10 {
		p := percent(float32(mv) / 1000)
		if p < prev || p < 0 || p > 100 {
			t.Fatalf("percent(%d mV) = %d after %d", mv, p, prev)
		}
		prev = p
	}
	if percent(3.0) != 0 || percent(4.3) != 100 {
		t.Error("ends")
	}
}

func TestRTCTimer(t *testing.T) {
	for _, c := range []struct {
		d             time.Duration
		source, count byte
	}{
		{0, 2, 1},
		{1400 * time.Millisecond, 2, 1},
		{60 * time.Second, 2, 60},
		{255 * time.Second, 2, 255},
		{256 * time.Second, 3, 4},
		{time.Hour, 3, 60},
		{10 * time.Hour, 3, 255},
	} {
		s, n := rtcTimer(c.d)
		if s != c.source || n != c.count {
			t.Errorf("rtcTimer(%v) = %d, %d; want %d, %d", c.d, s, n, c.source, c.count)
		}
	}
}
