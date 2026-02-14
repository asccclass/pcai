package history

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/asccclass/pcai/llms/ollama"
)

// Session represents a chat session state
type Session struct {
	ID         string           `json:"id"`
	Messages   []ollama.Message `json:"messages"`
	LastUpdate time.Time        `json:"last_update"`
}

// EnsureHistoryDir creates the history directory if it doesn't exist
func EnsureHistoryDir() string {
	home, _ := os.Getwd()
	historyDir := filepath.Join(home, "botmemory", "history")
	_ = os.MkdirAll(historyDir, 0755)
	return historyDir
}

// LoadLatestSession loads the most recently modified session
func LoadLatestSession() *Session {
	dir := EnsureHistoryDir()
	files, err := os.ReadDir(dir)
	if err != nil || len(files) == 0 {
		return NewSession()
	}

	// Sort files by mod time
	var fileInfos []os.FileInfo
	for _, entry := range files {
		if filepath.Ext(entry.Name()) == ".json" {
			info, err := entry.Info()
			if err == nil {
				fileInfos = append(fileInfos, info)
			}
		}
	}

	if len(fileInfos) == 0 {
		return NewSession()
	}

	sort.Slice(fileInfos, func(i, j int) bool {
		return fileInfos[i].ModTime().After(fileInfos[j].ModTime())
	})

	latestFile := filepath.Join(dir, fileInfos[0].Name())
	return LoadSession(latestFile)
}

// LoadSession loads a specific session file
func LoadSession(path string) *Session {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Printf("⚠️ 無法讀取歷史檔: %v\n", err)
		return NewSession()
	}

	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		fmt.Printf("⚠️ 解析歷史檔失敗: %v\n", err)
		return NewSession()
	}

	// 確保至少有一則訊息 (避免空指針)
	if s.Messages == nil {
		s.Messages = []ollama.Message{}
	}
	return &s
}

// NewSession creates a fresh session
func NewSession() *Session {
	return &Session{
		ID:         fmt.Sprintf("session_%d", time.Now().Unix()),
		Messages:   []ollama.Message{},
		LastUpdate: time.Now(),
	}
}

// SaveSession saves the session to disk
func SaveSession(s *Session) {
	if s == nil {
		return
	}
	s.LastUpdate = time.Now()

	// 如果 ID 為空，生成一個
	if s.ID == "" {
		s.ID = fmt.Sprintf("session_%d", time.Now().Unix())
	}

	home, _ := os.Getwd()
	path := filepath.Join(home, "botmemory", "history", s.ID+".json")

	// 確保目錄存在
	dir := filepath.Dir(path)
	_ = os.MkdirAll(dir, 0755)

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		fmt.Printf("⚠️ 無法儲存 Session: %v\n", err)
		return
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		fmt.Printf("⚠️ 寫入 Session 失敗: %v\n", err)
	}
}
