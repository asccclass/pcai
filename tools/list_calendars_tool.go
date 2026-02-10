package tools

import (
	"fmt"
	"strings"

	"github.com/asccclass/pcai/internal/calendar"
	"github.com/ollama/ollama/api"
)

// ListCalendarsTool 讓 LLM 可以列出所有行事曆
type ListCalendarsTool struct{}

func (t *ListCalendarsTool) Name() string { return "list_calendars" }

func (t *ListCalendarsTool) Definition() api.Tool {
	return api.Tool{
		Type: "function",
		Function: api.ToolFunction{
			Name:        "list_calendars",
			Description: "列出使用者所有的 Google Calendar 行事曆及其 ID。請轉化成表格（行事曆名稱,行事曆ID）方式顯示出來，不要summary或總結，單純列出就可以。",
			Parameters:  api.ToolFunctionParameters{Type: "object", Properties: &api.ToolPropertiesMap{}, Required: []string{}},
		},
	}
}

func (t *ListCalendarsTool) Run(args string) (string, error) {
	items, err := calendar.ListCalendars()
	if err != nil {
		return "", fmt.Errorf("[ListCalendarsTool] 列出行事曆失敗: %v", err)
	}

	if len(items) == 0 {
		return "[ListCalendarsTool] 未找到任何行事曆。", nil
	}

	var sb strings.Builder
	sb.WriteString("📅 **可用行事曆列表**:\n\n")

	// 支援簡單過濾 (若 args 為非 JSON 字串，視為關鍵字)
	query := ""
	cleanArgs := strings.TrimSpace(args)
	if cleanArgs != "" && cleanArgs != "{}" && !strings.Contains(cleanArgs, "{") {
		query = strings.TrimSpace(strings.ToLower(cleanArgs))
	}

	for i, item := range items {
		// 若有 query，則只顯示符合的項目
		if query != "" {
			if !strings.Contains(strings.ToLower(item.Summary), query) && !strings.Contains(strings.ToLower(item.ID), query) {
				continue
			}
		}

		primaryTag := ""
		if item.Primary {
			primaryTag = " (主要)"
		}
		// 使用更明確的格式列出
		fmt.Fprintf(&sb, "%d. **%s**%s\n", i+1, item.Summary, primaryTag)
		fmt.Fprintf(&sb, "   - ID: `%s`\n", item.ID)
		fmt.Fprintf(&sb, "   - 權限: `%s`\n", item.AccessRole)
		sb.WriteString("\n")
	}
	// sb.WriteString("\n請從以上列表中複製 ID (例如 `user@example.com`) 來讀取特定行事曆。\n")
	// sb.WriteString("讀取指令範例: `read_calendar ID1,ID2`\n")
	sb.WriteString("\n[SYSTEM INSTRUCTION: The user wants to see the FULL RAW LIST above. Do not summarize. Do not say 'You have 17 calendars'. Just copy the list above exactly.]")
	return sb.String(), nil
}
