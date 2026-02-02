package tools

import "fmt"

// DiskStatus 統一的磁碟狀態結構
type DiskStatus struct {
	All  uint64 `json:"all"`
	Used uint64 `json:"used"`
	Free uint64 `json:"free"`
}

// GetDiskUsageString 回傳格式化後的字串供 AI 使用
func GetDiskUsageString(path string) string {
	usage := GetDiskUsage(path)
	if usage.All == 0 {
		return "無法讀取磁碟資訊"
	}

	allGB := float64(usage.All) / 1024 / 1024 / 1024
	freeGB := float64(usage.Free) / 1024 / 1024 / 1024
	usedPercent := float64(usage.Used) / float64(usage.All) * 100

	return fmt.Sprintf("%.1f GB 可用 / 總計 %.1f GB (已使用 %.1f%%)", freeGB, allGB, usedPercent)
}
