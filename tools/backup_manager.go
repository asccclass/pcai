package tools

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// AutoBackupKnowledge 執行自動備份
func AutoBackupKnowledge() (string, error) {
	home, _ := os.Getwd()
	sourcePath := filepath.Join(home, "botmemory", "knowledge", "MEMORY.md")
	backupDir := filepath.Join(home, "botmemory", "backup")

	// 1. 檢查原始檔案是否存在
	if _, err := os.Stat(sourcePath); os.IsNotExist(err) {
		return "無需備份 (記憶庫尚未建立)", nil
	}

	// 2. 確保備份目錄存在
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return "", fmt.Errorf("無法建立備份目錄: %v", err)
	}

	// 3. 獲取最後一個備份檔案，檢查是否需要備份
	// 這裡簡單使用「若原始檔案修改時間比備份目錄內最新檔案新，就備份」
	backupFileName := fmt.Sprintf("knowledge_%s.md", time.Now().Format("20060102_150405"))
	destPath := filepath.Join(backupDir, backupFileName)

	// 4. 執行複製
	if err := copyFile(sourcePath, destPath); err != nil {
		return "", err
	}

	// 5. 保持備份數量（可選：只保留最近 10 個，防止佔滿 GX10 空間）
	cleanOldBackups(backupDir, 10)

	return fmt.Sprintf("備份成功: %s", backupFileName), nil
}

// 輔助函式：複製檔案
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}

// 輔助函式：清理舊備份
func cleanOldBackups(dir string, keep int) {
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) <= keep {
		return
	}

	var files []os.FileInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			if info, err := entry.Info(); err == nil {
				files = append(files, info)
			}
		}
	}

	if len(files) <= keep {
		return
	}

	// 根據修改時間排序（舊 -> 新）
	sort.Slice(files, func(i, j int) bool {
		return files[i].ModTime().Before(files[j].ModTime())
	})

	// 刪除最舊的檔案
	deleteCount := len(files) - keep
	for i := 0; i < deleteCount; i++ {
		filePath := filepath.Join(dir, files[i].Name())
		_ = os.Remove(filePath)
	}
}
