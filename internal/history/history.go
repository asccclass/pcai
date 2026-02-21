package history

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/asccclass/pcai/internal/memory"
	"github.com/asccclass/pcai/llms/ollama"

	"github.com/charmbracelet/lipgloss"
)

// Global ToolKit Instance (injected from tools/init.go)
var GlobalMemoryToolKit *memory.ToolKit

// ListHistory 顯示所有儲存過的 Session 簡述
func ListHistory() {
	home, _ := os.Getwd()
	historyDir := filepath.Join(home, "botmemory", "history")

	files, err := os.ReadDir(historyDir)
	if err != nil || len(files) == 0 {
		fmt.Println("ℹ️ 目前沒有任何對話歷史紀錄。")
		return
	}

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	fmt.Println(headerStyle.Render("\n📜 歷史對話清單："))
	fmt.Println(strings.Repeat("─", 40))

	for _, file := range files {
		if filepath.Ext(file.Name()) == ".json" {
			info, _ := file.Info()
			fmt.Printf("- %-20s (最後更新: %s)\n",
				file.Name(),
				info.ModTime().Format("2006-01-02 15:04"))
		}
	}
	fmt.Println()
}

// CheckAndSummarize 執行閒置歸納邏輯 (RAG 核心)
// 如果最後更新時間超過一小時，則進行歸納並清理 Session
// [Refactor] Now also triggers Memory Skills via Controller
func CheckAndSummarize(s *Session, modelName string, systemPrompt string) {
	if s == nil || len(s.Messages) < 2 {
		return
	}

	// 1. Trigger Memory ToolKit (The New Way)
	if GlobalMemoryToolKit != nil {
		lastMsg := s.Messages[len(s.Messages)-1]
		if lastMsg.Role == "user" {
			// 寫入今日日誌
			if err := GlobalMemoryToolKit.WriteToday(lastMsg.Content); err != nil {
				fmt.Fprintf(os.Stderr, "⚠️ [Memory] WriteToday 失敗: %v\n", err)
			} else {
				fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("13")).Render("\n🧠 [Memory] 已記錄至今日日誌"))
			}
		}
	}

	// 2. Legacy Summarization (The Old Way - Keep as fallback or long-term consolidation)
	// 判斷是否閒置超過 1 小時
	if time.Since(s.LastUpdate) > 1*time.Hour {
		fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Render("\n[系統] 偵測到閒置超過 1 小時，正在將對話歸納至長期記憶..."))

		// 準備歸納用的文本
		historyText := sessionToText(s)
		summaryPrompt := fmt.Sprintf("請根據以下對話紀錄，精煉出 3-5 個關鍵知識點，以 Markdown 列表格式輸出：\n\n%s", historyText)

		var summaryResult strings.Builder

		// 呼叫 Ollama 進行歸納 (使用較低的 Temperature 確保穩定)
		opts := ollama.Options{Temperature: 0.3, TopP: 0.9}
		_, err := ollama.ChatStream(modelName, []ollama.Message{
			{Role: "system", Content: "你是一個知識萃取專家"},
			{Role: "user", Content: summaryPrompt},
		}, nil, opts, func(c string) {
			summaryResult.WriteString(c)
		})

		if err == nil {
			// 存入 knowledge.md
			if err := saveToKnowledgeBase(summaryResult.String()); err == nil {
				// 歸納成功後，清空當前訊息流，保留 Context 指標 (或視需求全清)
				s.Messages = []ollama.Message{
					{Role: "system", Content: systemPrompt},
				}
				SaveSession(s)
				fmt.Println("✨ 歸納完成！已更新 knowledge")
			}
		}
	}
}

// sessionToText 輔助函式：將訊息陣列轉為純文字
func sessionToText(s *Session) string {
	var sb strings.Builder
	for _, m := range s.Messages {
		if m.Role == "system" {
			continue
		}
		sb.WriteString(fmt.Sprintf("%s: %s\n", m.Role, m.Content))
	}
	return sb.String()
}

// saveToKnowledgeBase 輔助函式：存入長期對話摘要庫
func saveToKnowledgeBase(summary string) error {
	// 避免將自動摘要直接寫入會干擾使用者確認機制的 MEMORY.md 中，因此改存到 auto_summaries.md
	home, _ := os.Getwd()
	path := filepath.Join(home, "botmemory", "history", "auto_summaries.md")
	_ = os.MkdirAll(filepath.Dir(path), 0755)

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	content := fmt.Sprintf("\n\n## [summarize] %s\n%s\n---\n",
		time.Now().Format("2006-01-02 15:04"), summary)

	_, err = f.WriteString(content)
	return err
}
