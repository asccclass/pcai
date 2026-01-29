package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ResolvePromptPaths 接收原始 Prompt，將其中的目錄關鍵字替換為系統絕對路徑
func ResolvePromptPaths(prompt string) (string, error) {
	// 1. 取得當前使用者的家目錄 (Cross-platform: Windows/Linux/macOS 皆適用)
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return prompt, fmt.Errorf("無法取得使用者家目錄: %v", err)
	}

	// 2. 定義關鍵字映射表 (可根據需求擴充)
	// Key: 使用者可能輸入的中文關鍵字
	// Value: 該 OS 下對應的標準英文資料夾名稱
	pathAliases := map[string]string{
		"桌面": "Desktop",
		"下載": "Downloads",
		"文件": "Documents",
		"圖片": "Pictures",
		"影片": "Videos",
	}

	processedPrompt := prompt

	// 3. 遍歷映射表進行替換
	for alias, folderName := range pathAliases {
		// 如果 Prompt 中包含這個關鍵字 (例如 "桌面")
		if strings.Contains(processedPrompt, alias) {
			// 組合絕對路徑
			// filepath.Join 會自動處理分隔符號：
			// Windows -> C:\Users\User\Desktop
			// Linux   -> /home/user/Desktop
			fullPath := filepath.Join(homeDir, folderName)

			// 替換關鍵字
			processedPrompt = strings.ReplaceAll(processedPrompt, alias, fullPath)
		}
	}
	return processedPrompt, nil
}
