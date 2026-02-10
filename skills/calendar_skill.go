package skills

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/asccclass/pcai/internal/calendar"
	"github.com/ollama/ollama/api"
)

// CalendarSkill 負責行事曆讀取與 AI 摘要
type CalendarSkill struct {
	ollamaClient *api.Client
	modelName    string
}

func NewCalendarSkill(client *api.Client, modelName string) *CalendarSkill {
	return &CalendarSkill{
		ollamaClient: client,
		modelName:    modelName,
	}
}

// Execute 讀取行事曆並產生摘要報告
func (s *CalendarSkill) Execute() {
	fmt.Println("[CalendarSkill] 開始檢查行事曆...")

	// 1. 抓取未來 10 筆事件
	// 1. 抓取未來 10 筆事件
	// 1. 抓取未來 10 筆事件
	events, err := calendar.FetchUpcomingEvents("", "", 10)
	if err != nil {
		log.Printf("[CalendarSkill Error] 抓取行事曆失敗: %v", err)
		return
	}

	if len(events) == 0 {
		log.Println("[CalendarSkill] 近期無行事曆活動。")
		return
	}

	// 2. 格式化事件列表
	var sb strings.Builder
	sb.WriteString("以下是使用者近期的行事曆活動：\n")
	for _, e := range events {
		sb.WriteString(fmt.Sprintf("- 時間: %s ~ %s | 事件: %s | 地點: %s\n", e.Start, e.End, e.Summary, e.Location))
		if e.Description != "" {
			sb.WriteString(fmt.Sprintf("  備註: %s\n", e.Description))
		}
	}

	eventContent := sb.String()

	// 3. 呼叫 Ollama 生成簡報 (Adapter)
	ctx := context.Background()
	prompt := fmt.Sprintf(`你是一個貼心的個人助理。請根據以下行事曆內容，為使用者生成一份簡短的「行程提醒」。
重點在於提醒即將到來的會議或活動。請用繁體中文回答，語氣輕鬆自然。

%s`, eventContent)

	req := &api.GenerateRequest{
		Model:  s.modelName,
		Prompt: prompt,
		Stream: new(bool),
	}

	var summary string
	err = s.ollamaClient.Generate(ctx, req, func(resp api.GenerateResponse) error {
		summary = resp.Response
		return nil
	})

	if err != nil {
		log.Printf("[CalendarSkill Error] Ollama 生成失敗: %v", err)
		return
	}

	// 4. 顯示或通知
	fmt.Printf("\n🗓️ [行程提醒]\n%s\n\n", summary)

	// 5. 寫入 Knowledge (選擇性)
	s.saveToKnowledge(summary)
}

func (s *CalendarSkill) saveToKnowledge(summary string) {
	home, _ := os.Getwd()
	path := filepath.Join(home, "botmemory", "knowledge", "knowledge.md")

	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		log.Printf("[CalendarSkill Error] 無法寫入知識庫: %v", err)
		return
	}
	defer f.Close()

	timestamp := time.Now().Format("2006-01-02 15:04")
	content := fmt.Sprintf("\n\n## 📅 行事曆快照: %s\n%s\n", timestamp, summary)

	f.WriteString(content)
}
