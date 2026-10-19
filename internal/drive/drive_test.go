package drive

import "testing"

// TestWholeDisk verifies partition names collapse to the Ventoy-style drive
// on both families: Linux (trailing digits, the nvme/mmcblk p-separator) and
// BSD/macOS (disk slice numbers, disk3s1 → disk3).
func TestWholeDisk(t *testing.T) {
	cases := map[string]string{
		"sda1":      "sda",
		"sda10":     "sda",
		"nvme0n1p2": "nvme0n1",
		"mmcblk0p3": "mmcblk0",
		"disk3s1":   "disk3",
		"disk1s2":   "disk1",
		"loop0p1":   "loop0",
		"sdp1":      "sdp", // a device that already ends in p keeps the letter
		"vdb":       "vdb",
	}
	for partition, want := range cases {
		if got := wholeDisk(partition); got != want {
			t.Errorf("wholeDisk(%q) = %q, want %q", partition, got, want)
		}
	}
}
