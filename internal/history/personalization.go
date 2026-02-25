package history

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/asccclass/pcai/internal/database"
)

// PersonalizationWorker 負責背景分析對話日誌以提取用戶偏好
type PersonalizationWorker struct {
	WorkspaceDir string
	DB           *database.DB
	ModelName    string
	AnalyzeFunc  func(model string, prompt string) (string, error)
}

// NewPersonalizationWorker 建立新的背景工作者
func NewPersonalizationWorker(workspaceDir string, db *database.DB, model string, analyzeFunc func(model string, prompt string) (string, error)) *PersonalizationWorker {
	return &PersonalizationWorker{
		WorkspaceDir: workspaceDir,
		DB:           db,
		ModelName:    model,
		AnalyzeFunc:  analyzeFunc,
	}
}

// RunOnce 執行一次分析任務
func (w *PersonalizationWorker) RunOnce() error {
	historyDir := filepath.Join(w.WorkspaceDir, "history")
	files, err := os.ReadDir(historyDir)
	if err != nil {
		return err
	}

	// 只分析最近 3 天的日誌
	now := time.Now()
	for _, file := range files {
		name := file.Name()
		if filepath.Ext(name) != ".json" {
			continue
		}

		// 確保只處理 DailyLogger 產生的 YYYY-MM-DD.json 日誌檔，過濾掉 session_*.json 等其他檔案
		dateStr := strings.TrimSuffix(name, ".json")
		if _, err := time.Parse("2006-01-02", dateStr); err != nil {
			continue
		}

		info, _ := file.Info()
		if now.Sub(info.ModTime()) > 72*time.Hour {
			continue
		}

		if err := w.analyzeFile(filepath.Join(historyDir, file.Name())); err != nil {
			fmt.Fprintf(os.Stderr, "⚠️ [Personalization] 分析檔案失敗 %s: %v\n", file.Name(), err)
		}
	}

	return nil
}

func (w *PersonalizationWorker) analyzeFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var entries []DailyEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return err
	}

	if len(entries) == 0 {
		return nil
	}

	// 構建 Prompt
	var sb strings.Builder
	for _, e := range entries {
		sb.WriteString(fmt.Sprintf("%s: %s\n", e.Role, e.Content))
	}

	prompt := fmt.Sprintf(`請分析以下對話紀錄，提取「用戶偏好 (Preferences)」與「關鍵實體 (Key Entities)」。
請以 JSON 陣列格式輸出，每個物件包含 category (名稱為 preference 或 entity), key (英文標識), value (具體內容), tags (逗號分隔的標籤)。
範例：[{"category": "preference", "key": "language", "value": "繁體中文", "tags": "ui,communication"}]
如果沒有提取到任何內容，請回傳空陣列 []。不要包含任何 Markdown 標記，直接輸出 JSON。

對話紀錄：
%s`, sb.String())

	resultStr, err := w.AnalyzeFunc(w.ModelName, prompt)
	if err != nil {
		return err
	}

	// 解析 AI 回傳的 JSON (簡單清理 markdown 標籤)
	cleanJSON := strings.TrimSpace(resultStr)
	if idx := strings.Index(cleanJSON, "["); idx != -1 {
		if lastIdx := strings.LastIndex(cleanJSON, "]"); lastIdx != -1 {
			cleanJSON = cleanJSON[idx : lastIdx+1]
		}
	}

	var extraction []struct {
		Category string `json:"category"`
		Key      string `json:"key"`
		Value    string `json:"value"`
		Tags     string `json:"tags"`
	}

	if err := json.Unmarshal([]byte(cleanJSON), &extraction); err != nil {
		// Log error but don't fail entire file
		fmt.Fprintf(os.Stderr, "⚠️ [Personalization] JSON 解析失敗: %v\n原始文字: %s\n", err, cleanJSON)
		return nil
	}

	// 存入資料庫
	ctx := context.Background()
	for _, item := range extraction {
		if err := w.DB.AddPermanentMemory(ctx, item.Category, item.Key, item.Value, item.Tags); err != nil {
			fmt.Fprintf(os.Stderr, "⚠️ [Personalization] 存入資料庫失敗: %v\n", err)
		}
	}

	return nil
}
