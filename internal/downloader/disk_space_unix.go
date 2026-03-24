//go:build !windows

package downloader

import (
	"fmt"
	"syscall"
)

// getFreeDiskSpace returns the number of free bytes on the disk containing the given path.
func getFreeDiskSpace(dir string) (int64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(dir, &stat); err != nil {
		return 0, fmt.Errorf("failed to stat disk: %w", err)
	}

	// Bavail is available blocks for unprivileged users
	freeBytes := int64(stat.Bavail) * int64(stat.Bsize)
	return freeBytes, nil
}
