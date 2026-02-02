//go:build !windows

package tools

import "syscall"

func GetDiskUsage(path string) (usage DiskStatus) {
	var stat syscall.Statfs_t
	err := syscall.Statfs(path, &stat)
	if err != nil {
		return
	}
	usage.All = stat.Blocks * uint64(stat.Bsize)
	usage.Free = stat.Bavail * uint64(stat.Bsize)
	usage.Used = usage.All - usage.Free
	return
}
