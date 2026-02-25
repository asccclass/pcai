package history

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/asccclass/pcai/internal/memory"
	"github.com/asccclass/pcai/llms/ollama"
)

// DailyEntry 代表單條歷史紀錄
type DailyEntry struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Timestamp string    `json:"timestamp"`
	CreatedAt time.Time `json:"created_at"`
}

// DailyLogger 負責管理每日日誌
type DailyLogger struct {
	WorkspaceDir string
}

// NewDailyLogger 建立每日日誌紀錄器
func NewDailyLogger(workspaceDir string) *DailyLogger {
	return &DailyLogger{WorkspaceDir: workspaceDir}
}

// Record 記錄一條訊息到每日日誌 (YYYY-MM-DD.json)
func (l *DailyLogger) Record(msg ollama.Message) error {
	// 噪音過濾
	opts := memory.DefaultNoiseFilterOptions()
	if memory.IsNoise(msg.Content, opts) {
		return nil // 忽略噪音
	}

	today := time.Now().Format("2006-01-02")
	historyDir := filepath.Join(l.WorkspaceDir, "history")
	if err := os.MkdirAll(historyDir, 0750); err != nil {
		return err
	}

	filePath := filepath.Join(historyDir, today+".json")

	var entries []DailyEntry
	if _, err := os.Stat(filePath); err == nil {
		data, err := os.ReadFile(filePath)
		if err == nil {
			_ = json.Unmarshal(data, &entries)
		}
	}

	entry := DailyEntry{
		Role:      msg.Role,
		Content:   msg.Content,
		Timestamp: time.Now().Format("15:04"),
		CreatedAt: time.Now(),
	}
	entries = append(entries, entry)

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filePath, data, 0644)
}

// LoadToday 載入今日的日誌
func (l *DailyLogger) LoadToday() ([]DailyEntry, error) {
	today := time.Now().Format("2006-01-02")
	filePath := filepath.Join(l.WorkspaceDir, "history", today+".json")

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, nil
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var entries []DailyEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}

	return entries, nil
}
