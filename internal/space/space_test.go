package space

import (
	"strings"
	"testing"
)

// TestHumanBytes checks the binary-unit formatting.
func TestHumanBytes(t *testing.T) {
	cases := map[int64]string{
		0:         "0 B",
		512:       "512 B",
		1 << 10:   "1.0 KiB",
		5 << 20:   "5.0 MiB",
		4<<30 - 1: "4.0 GiB",
		128 << 30: "128.0 GiB",
	}
	for n, want := range cases {
		if got := HumanBytes(n); got != want {
			t.Errorf("HumanBytes(%d) = %q, want %q", n, got, want)
		}
	}
}

// TestUsageBar verifies segment widths and clipping of oversubscribed bars.
// Cells come from cumulative boundaries, so no rounding step can grow the
// filled length beyond the proportional value.
func TestUsageBar(t *testing.T) {
	bar := UsageBar(100, Segment{60, "86"}, Segment{40, "196"})
	if plain := stripANSI(bar); strings.Count(plain, "█") != 40 || strings.Count(plain, "░") != 0 {
		t.Errorf("full bar = %q, want 40 blocks and no track", plain)
	}
	bar = UsageBar(100, Segment{25, "86"}, Segment{100, "196"})
	if plain := stripANSI(bar); strings.Count(plain, "█") != 40 || strings.Count(plain, "░") != 0 {
		t.Errorf("oversubscribed bar = %q, want clipped to 40 blocks", plain)
	}
	bar = UsageBar(100, Segment{10, "86"})
	if plain := stripANSI(bar); strings.Count(plain, "█") != 4 || strings.Count(plain, "░") != 36 {
		t.Errorf("partial bar = %q, want 4 blocks and 36 track cells", plain)
	}
	bar = UsageBar(1000, Segment{900, "86"}, Segment{1, "196"})
	if plain := stripANSI(bar); strings.Count(plain, "█") != 36 || strings.Count(plain, "░") != 4 {
		t.Errorf("tiny-segment bar = %q, want proportional 36 blocks with no inflating minimum", plain)
	}
	if plain := stripANSI(UsageBar(0)); strings.Count(plain, "░") != 40 {
		t.Errorf("degenerate total = %q, want an empty track", plain)
	}
}

// stripANSI removes escape sequences so tests can count visible characters.
func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == '\x1b' {
			for i < len(s) && s[i] != 'm' {
				i++
			}
			i++
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}
