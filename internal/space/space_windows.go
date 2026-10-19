//go:build windows

package space

import "golang.org/x/sys/windows"

// Free queries the volume containing path with GetDiskFreeSpaceEx.
func Free(path string) (Space, error) {
	var free, total, _ uint64
	pointer, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return Space{}, err
	}
	if err := windows.GetDiskFreeSpaceEx(pointer, &free, &total, nil); err != nil {
		return Space{}, err
	}
	return Space{Total: int64(total), Free: int64(free)}, nil
}
