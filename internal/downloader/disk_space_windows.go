//go:build windows

package downloader

import (
	"fmt"
	"syscall"
	"unsafe"
)

// getFreeDiskSpace returns the number of free bytes on the disk containing the given path.
func getFreeDiskSpace(dir string) (int64, error) {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getDiskFreeSpaceEx := kernel32.NewProc("GetDiskFreeSpaceExW")

	dirPtr, err := syscall.UTF16PtrFromString(dir)
	if err != nil {
		return 0, fmt.Errorf("invalid directory path: %w", err)
	}

	var freeBytesAvailable int64
	var totalBytes int64
	var totalFreeBytes int64

	r1, _, e1 := getDiskFreeSpaceEx.Call(
		uintptr(unsafe.Pointer(dirPtr)),
		uintptr(unsafe.Pointer(&freeBytesAvailable)),
		uintptr(unsafe.Pointer(&totalBytes)),
		uintptr(unsafe.Pointer(&totalFreeBytes)),
	)
	if r1 == 0 {
		return 0, fmt.Errorf("GetDiskFreeSpaceEx failed: %v", e1)
	}

	return freeBytesAvailable, nil
}
