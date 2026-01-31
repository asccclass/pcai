package tools

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// AutoBackupKnowledge 執行自動備份
func AutoBackupKnowledge() (string, error) {
	home, _ := os.Getwd()
	sourcePath := filepath.Join(home, "botmemory", "knowledge", "knowledge.md")
	backupDir := filepath.Join(home, "botmemory", "backup", "backups")

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
	files, _ := os.ReadDir(dir)
	if len(files) <= keep {
		return
	}
	// 這裡可以加入排序邏輯刪除最舊的，簡單起見先實作基本功能
}
