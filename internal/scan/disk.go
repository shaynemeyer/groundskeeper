package scan

import "syscall"

// Disk is free and total space on the volume holding a path.
type Disk struct {
	Free  int64
	Total int64
}

// DiskUsage reads free and total space for the volume holding path.
// Free excludes space macOS reports as purgeable.
func DiskUsage(path string) (Disk, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return Disk{}, err
	}
	bs := int64(st.Bsize)
	return Disk{Free: int64(st.Bavail) * bs, Total: int64(st.Blocks) * bs}, nil
}
