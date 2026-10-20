package app

import "os"

// fileSize reports the size of an existing file, or zero when it is missing.
func fileSize(path string) int64 {
	if info, err := os.Stat(path); err == nil {
		return info.Size()
	}
	return 0
}

// spaceEffect totals the bytes the selected rows will download and the bytes
// their deletions will free. Delete-marked rows only contribute freed bytes;
// update rows contribute their replacement's size, plus the installed size
// when the old ISO is not kept.
func spaceEffect(updates []Update) (add, freed int64) {
	for _, u := range updates {
		if u.DeleteISO {
			freed += u.CurrentSize
			continue
		}
		if !downloadable(u) {
			continue
		}
		add += u.LatestSize
		if !u.KeepOld && !u.New {
			freed += u.CurrentSize
		}
	}
	return add, freed
}
