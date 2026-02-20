package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/asccclass/pcai/internal/database"
)

// memorySourceMap 定義使用者輸入關鍵字 → 短期記憶來源的映射
// source 名稱必須與 toolNameToMemorySource() 中的值一致
var memorySourceMap = []struct {
	InputKeywords []string // 使用者輸入中可能包含的關鍵字
	Source        string   // 對應的 short_term_memory.source 值
}{
	{
		InputKeywords: []string{"天氣", "氣象", "weather", "預報", "會冷", "會熱", "下雨", "溫度", "氣溫"},
		Source:        "weather",
	},
	{
		InputKeywords: []string{"行事曆", "行程", "日程", "calendar", "schedule"},
		Source:        "calendar",
	},
	{
		InputKeywords: []string{"郵件", "信件", "信箱", "email", "mail"},
		Source:        "email",
	},
}

// BuildMemorySearchFunc 建立短期記憶預搜尋函式
// 傳入 SQLite DB，回傳可設定給 Agent.OnMemorySearch 的回調函式
func BuildMemorySearchFunc(db *database.DB) func(query string) string {
	if db == nil {
		return nil
	}

	return func(query string) string {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		lower := strings.ToLower(query)

		// 根據輸入關鍵字找到對應的記憶來源 (source)
		source := ""
		for _, mapping := range memorySourceMap {
			for _, kw := range mapping.InputKeywords {
				if strings.Contains(lower, strings.ToLower(kw)) {
					source = mapping.Source
					break
				}
			}
			if source != "" {
				break
			}
		}

		if source == "" {
			// 沒有匹配任何已知分類，不搜尋記憶
			return ""
		}

		// 按來源搜尋短期記憶（精確比對 source 欄位）
		entries, err := db.GetShortTermMemoryBySource(ctx, source, 3)
		if err != nil || len(entries) == 0 {
			return ""
		}

		// 格式化記憶上下文
		var sb strings.Builder
		sb.WriteString("[MEMORY CONTEXT] 以下是系統短期記憶中的相關資訊。若內容足以回答問題，請直接引用此資訊回答，不需要再呼叫工具：\n")
		for i, e := range entries {
			content := strings.TrimSpace(e.Content)
			if len(content) > 1500 {
				content = content[:1500] + "...«已截斷»"
			}
			sb.WriteString(fmt.Sprintf("\n--- 記憶 %d [%s] ---\n%s\n", i+1, e.CreatedAt, content))
		}

		return sb.String()
	}
}
