//go:build unix

package space

import "syscall"

// Free stats the filesystem containing path.
func Free(path string) (Space, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return Space{}, err
	}
	return Space{Total: int64(st.Blocks) * int64(st.Bsize), Free: int64(st.Bavail) * int64(st.Bsize)}, nil
}
