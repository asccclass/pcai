package history

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/asccclass/pcai/llms/ollama"
)

// Session 代表一次完整的對話會話
type Session struct {
	ID         int64            `json:"id"`
	Title      string           `json:"title"`
	Messages   []ollama.Message `json:"messages"`
	LastUpdate time.Time        `json:"last_update"`
}

// CurrentSession 儲存目前執行中的對話狀態
var CurrentSession *Session

// LoadLatestSession 從本地檔案讀取最近一次的對話紀錄
func LoadLatestSession() *Session {
	home, _ := os.UserHomeDir()
	// 預設儲存在 ~/.pcai/history/latest.json
	path := filepath.Join(home, ".pcai", "history", "latest.json")

	data, err := os.ReadFile(path)
	if err != nil {
		// 如果檔案不存在，回傳一個全新的空 Session
		return &Session{
			ID:         time.Now().Unix(),
			Messages:   []ollama.Message{},
			LastUpdate: time.Now(),
		}
	}

	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		// 若 JSON 損壞，建立新 Session
		return &Session{ID: time.Now().Unix(), Messages: []ollama.Message{}, LastUpdate: time.Now()}
	}
	return &s
}

// SaveSession 將傳入的 Session 物件持久化到本地檔案
func SaveSession(s *Session) error {
	if s == nil {
		return nil
	}

	s.LastUpdate = time.Now()

	// 確保目錄存在
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".pcai", "history")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("無法建立紀錄目錄: %v", err)
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("JSON 編碼失敗: %v", err)
	}

	// 儲存為最新紀錄
	path := filepath.Join(dir, "latest.json")
	return os.WriteFile(path, data, 0644)
}

// ClearHistory 刪除本地所有的對話紀錄
func ClearHistory() error {
	home, _ := os.UserHomeDir()
	path := filepath.Join(home, ".pcai", "history", "latest.json")
	return os.Remove(path)
}
