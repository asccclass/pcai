//go:build windows

package tools

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

func GetDiskUsage(path string) (usage DiskStatus) {
	h := windows.MustLoadDLL("kernel32.dll")
	c := h.MustFindProc("GetDiskFreeSpaceExW")

	var freeBytes, totalBytes, availBytes uint64

	// Windows 需要將路徑轉換為 UTF16
	pathPtr, _ := windows.UTF16PtrFromString(path)

	_, _, _ = c.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&freeBytes)),
		uintptr(unsafe.Pointer(&totalBytes)),
		uintptr(unsafe.Pointer(&availBytes)),
	)

	usage.All = totalBytes
	usage.Free = availBytes
	usage.Used = usage.All - usage.Free
	return
}
