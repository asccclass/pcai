package history

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/asccclass/pcai/llms/ollama"
)

// Session 代表一次完整的對話會話
type Session struct {
	ID         string           `json:"id"`
	Title      string           `json:"title"`
	Messages   []ollama.Message `json:"messages"`
	LastUpdate time.Time        `json:"last_update"`
}

// 移除全域 CurrentSession，避免多使用者衝突
// var CurrentSession *Session

// LoadSession 載入指定 ID 的對話紀錄
func LoadSession(id string) *Session {
	home, _ := os.Getwd()
	// 檔名規則：SessionID.json
	// 如果是舊版純數字 ID，仍然可以讀取，但內部都視為 string
	path := filepath.Join(home, "botmemory", "history", fmt.Sprintf("%s.json", id))

	data, err := os.ReadFile(path)
	if err != nil {
		// 檔案不存在，回傳新 Session
		return &Session{
			ID:         id,
			Messages:   []ollama.Message{},
			LastUpdate: time.Now(),
		}
	}

	var s Session
	// 嘗試 Unmarshal
	if err := json.Unmarshal(data, &s); err != nil {
		// 如果失敗可能是因為舊版 ID 是數字，這裡做個簡單的相容處理或是直接強制轉換
		// 暫時假定使用者會容忍舊紀錄格式不相容，或是我們手動修正
		// 若要相容舊版 (int ID)，需要自訂 UnmarshalJSON，這裡先採簡單策略：
		// 若失敗則建立新的
		return &Session{ID: id, Messages: []ollama.Message{}, LastUpdate: time.Now()}
	}

	// 確保 ID 一致 (防止 JSON 內的 ID 與檔名不符)
	if s.ID == "" {
		s.ID = id
	}
	return &s
}

// LoadLatestSession 掃描目錄，找出最後更新的 Session (CLI 用)
func LoadLatestSession() *Session {
	home, _ := os.Getwd()
	dir := filepath.Join(home, "botmemory", "history")

	// 確保目錄存在
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		_ = os.MkdirAll(dir, 0755)
	}

	files, err := os.ReadDir(dir)
	if err != nil || len(files) == 0 {
		// 沒檔案，建立一個以當前 timestamp 為 ID 的新 Session
		newID := fmt.Sprintf("%d", time.Now().Unix())
		return &Session{
			ID:         newID,
			Messages:   []ollama.Message{},
			LastUpdate: time.Now(),
		}
	}

	// 找出最後修改的 JSON 檔案
	var latestFile os.DirEntry
	var latestTime time.Time

	for _, file := range files {
		// 排除舊版 legacy naming 以及 Telegram 專屬 Session (CLI 只回復一般對話)
		if filepath.Ext(file.Name()) == ".json" && file.Name() != "latest.json" && !strings.HasPrefix(file.Name(), "telegram_") {
			info, err := file.Info()
			if err == nil {
				if info.ModTime().After(latestTime) {
					latestTime = info.ModTime()
					latestFile = file
				}
			}
		}
	}

	// 舊版相容：如果只找到 latest.json 或沒找到其他檔案
	if latestFile == nil {
		// 嘗試讀取舊版 latest.json
		legacyPath := filepath.Join(dir, "latest.json")
		if _, err := os.Stat(legacyPath); err == nil {
			// 讀取並轉移
			data, _ := os.ReadFile(legacyPath)
			var temp struct {
				ID         interface{}      `json:"id"` // 可能是 int 或 string
				Title      string           `json:"title"`
				Messages   []ollama.Message `json:"messages"`
				LastUpdate time.Time        `json:"last_update"`
			}
			if err := json.Unmarshal(data, &temp); err == nil {
				// 轉換 ID
				var idStr string
				switch v := temp.ID.(type) {
				case float64:
					idStr = fmt.Sprintf("%.0f", v)
				case string:
					idStr = v
				default:
					idStr = fmt.Sprintf("%d", time.Now().Unix())
				}

				return &Session{
					ID:         idStr,
					Title:      temp.Title,
					Messages:   temp.Messages,
					LastUpdate: temp.LastUpdate,
				}
			}
		}

		// 真的沒檔案
		newID := fmt.Sprintf("%d", time.Now().Unix())
		return &Session{ID: newID, Messages: []ollama.Message{}, LastUpdate: time.Now()}
	}

	// 載入該檔案
	id := strings.TrimSuffix(latestFile.Name(), ".json")
	return LoadSession(id)
}

// SaveSession 將 Session 持久化
func SaveSession(s *Session) error {
	if s == nil {
		return nil
	}

	s.LastUpdate = time.Now()

	home, _ := os.Getwd()
	dir := filepath.Join(home, "botmemory", "history")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("無法建立紀錄目錄: %v", err)
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("JSON 編碼失敗: %v", err)
	}

	// 使用 ID 作為檔名
	path := filepath.Join(dir, fmt.Sprintf("%s.json", s.ID))
	return os.WriteFile(path, data, 0644)
}

// ClearHistory 刪除本地所有的對話紀錄
func ClearHistory() error {
	home, _ := os.Getwd()
	dir := filepath.Join(home, "botmemory", "history")
	return os.RemoveAll(dir) // 刪除整個目錄內容
}
