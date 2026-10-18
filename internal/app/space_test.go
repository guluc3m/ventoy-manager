package app

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
		if got := humanBytes(n); got != want {
			t.Errorf("humanBytes(%d) = %q, want %q", n, got, want)
		}
	}
}

// TestSpaceEffect verifies that only selected, healthy rows count toward the
// download total and that deletions are tallied separately.
func TestSpaceEffect(t *testing.T) {
	updates := []Update{
		{CurrentSize: 5 << 30, LatestSize: 6 << 30, Selected: true, DeleteOld: true},
		{CurrentSize: 3 << 30, LatestSize: 4 << 30, Selected: true},
		{CurrentSize: 9 << 30, LatestSize: 9 << 30, Selected: false, DeleteOld: true},
		{Err: errTest, Selected: true, LatestSize: 2 << 30},
	}
	add, freed := spaceEffect(updates)
	if add != 10<<30 {
		t.Errorf("add = %d, want %d", add, int64(10<<30))
	}
	if freed != 5<<30 {
		t.Errorf("freed = %d, want %d", freed, int64(5<<30))
	}
}

// TestUsageBar verifies segment widths, clipping of oversubscribed bars, and
// the one-cell minimum that keeps small changes visible.
func TestUsageBar(t *testing.T) {
	bar := usageBar(100, segment{60, "86"}, segment{40, "196"})
	if plain := stripANSI(bar); strings.Count(plain, "█") != 40 || strings.Count(plain, "░") != 0 {
		t.Errorf("full bar = %q, want 40 blocks and no track", plain)
	}
	bar = usageBar(100, segment{25, "86"}, segment{100, "196"})
	if plain := stripANSI(bar); strings.Count(plain, "█") != 40 || strings.Count(plain, "░") != 0 {
		t.Errorf("oversubscribed bar = %q, want clipped to 40 blocks", plain)
	}
	bar = usageBar(100, segment{10, "86"})
	if plain := stripANSI(bar); strings.Count(plain, "█") != 4 || strings.Count(plain, "░") != 36 {
		t.Errorf("partial bar = %q, want 4 blocks and 36 track cells", plain)
	}
	bar = usageBar(1000, segment{900, "86"}, segment{1, "196"})
	if plain := stripANSI(bar); strings.Count(plain, "█") != 37 || strings.Count(plain, "░") != 3 {
		t.Errorf("tiny-segment bar = %q, want the 1/1000 segment forced to one cell", plain)
	}
	if plain := stripANSI(usageBar(0)); strings.Count(plain, "░") != 40 {
		t.Errorf("degenerate total = %q, want an empty track", plain)
	}
}

// errTest is a placeholder error for unresolvable rows.
var errTest = &testError{}

type testError struct{}

func (*testError) Error() string { return "test" }

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
