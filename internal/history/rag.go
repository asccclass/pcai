package history

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/asccclass/pcai/llms/ollama"
)

// GetRAGEnhancedPrompt 載入長期記憶，增強 System Prompt
func GetRAGEnhancedPrompt() string {
	// 使用 ToolKit 載入
	if GlobalMemoryToolKit != nil {
		bootstrap, err := GlobalMemoryToolKit.LoadBootstrap()
		if err == nil && bootstrap != "" {
			if len(bootstrap) > 4000 {
				bootstrap = bootstrap[:4000] + "\n...(已截斷)"
			}
			return "\n\n---\n以下是你的長期記憶，可用於回答問題：\n" + bootstrap
		}
	}

	// Fallback: 直接讀取檔案
	home, _ := os.Getwd()
	// 嘗試 MEMORY.md
	path := filepath.Join(home, "botmemory", "knowledge", "MEMORY.md")
	data, err := os.ReadFile(path)
	if err != nil {
		// 向下相容：嘗試 knowledge.md
		path = filepath.Join(home, "botmemory", "knowledge", "knowledge.md")
		data, err = os.ReadFile(path)
		if err != nil {
			return "" // 無記憶
		}
	}

	content := string(data)
	if len(content) > 4000 {
		content = content[:4000] + "\n...(已截斷)"
	}
	return "\n\n---\n以下是你的長期記憶，可用於回答問題：\n" + content
}

// ClearKnowledgeBase 清除長期記憶檔案
func ClearKnowledgeBase() error {
	home, _ := os.Getwd()
	// 嘗試刪除 MEMORY.md
	memoryPath := filepath.Join(home, "botmemory", "knowledge", "MEMORY.md")
	if err := os.Remove(memoryPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	// 向下相容：也嘗試刪除 knowledge.md
	legacyPath := filepath.Join(home, "botmemory", "knowledge", "knowledge.md")
	if err := os.Remove(legacyPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// GetSummaryFromAI 呼叫 AI 進行單次總結 (用於存檔前的手動呼叫或自動排程)
func GetSummaryFromAI(modelName string, messages []ollama.Message) (string, error) {
	if len(messages) == 0 {
		return "", fmt.Errorf("沒有對話紀錄可以歸納")
	}

	var sb strings.Builder
	for _, m := range messages {
		if m.Role == "system" {
			continue
		}
		sb.WriteString(fmt.Sprintf("%s: %s\n", m.Role, m.Content))
	}

	prompt := "請將上述對話內容精煉成 3 個重點，使用 Markdown 列表格式輸出。只回傳列表內容，不要有額外開場白。"

	var summaryResult strings.Builder
	_, err := ollama.ChatStream(modelName, []ollama.Message{
		{Role: "system", Content: "你是一個專業的資料歸納員"},
		{Role: "user", Content: "對話內容如下：\n" + sb.String() + "\n\n" + prompt},
	}, nil, ollama.Options{Temperature: 0.1}, func(c string) {
		// 歸納時通常不顯示在 UI 上，僅收集結果
		summaryResult.WriteString(c)
	})

	return summaryResult.String(), err
}

/*
func ArchiveAndSummarize(modelName string) {
	s := CurrentSession
	if s == nil || len(s.Messages) < 2 {
		return
	}

	// 構建歸納 Prompt
	var chatHistory strings.Builder
	for _, m := range s.Messages {
		chatHistory.WriteString(fmt.Sprintf("%s: %s\n", m.Role, m.Content))
	}

	summaryPrompt := "請根據以下對話紀錄，精煉出 3-5 個關鍵知識點，以 Markdown 列表格式輸出：\n\n" + chatHistory.String()

	// 呼叫 AI 進行歸納
	var summaryResult strings.Builder
	_, err := ollama.ChatStream(modelName, "你是一個知識萃取專家", summaryPrompt, nil, ollama.Options{Temperature: 0.3}, nil, func(c string) {
		summaryResult.WriteString(c)
	})

	if err == nil {
		// 將歸納內容存入 RAG 資料庫 (knowledge.md)
		f, _ := os.OpenFile("knowledge.md", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		f.WriteString(fmt.Sprintf("\n## %s (%s)\n%s", s.Title, time.Now().Format("2006-01-02"), summaryResult.String()))
		f.Close()

		// 重置 Session（RAG 的精髓：歸納後清除細節，只留精華）
		s.Messages = []Message{}
		s.Context = nil
		SaveCurrentSession()
		fmt.Println("\n✨ 偵測到閒置，對話已歸納至長期記憶知識庫。")
	}
}
*/
