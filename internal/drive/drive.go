package drive

// wholeDisk maps a partition device to the Ventoy-style drive that holds it:
// trailing digits (sda1 → sda) plus the two separators — the p of nvme/mmcblk
// devices (nvme0n1p2 → nvme0n1, mmcblk0p3 → mmcblk0) and the s of BSD disk
// slices (disk3s1 → disk3) — come off.
func wholeDisk(partition string) string {
	end := len(partition)
	for end > 0 && isDigit(partition[end-1]) {
		end--
	}
	if end > 1 && (partition[end-1] == 'p' || partition[end-1] == 's') && isDigit(partition[end-2]) {
		end--
	}
	return partition[:end]
}

// isDigit reports whether b is an ASCII decimal digit.
func isDigit(b byte) bool {
	return b >= '0' && b <= '9'
}
