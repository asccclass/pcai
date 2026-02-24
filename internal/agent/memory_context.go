package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/asccclass/pcai/internal/database"
	"github.com/asccclass/pcai/internal/memory"
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

// BuildMemorySearchFunc 建立短期與長期記憶預搜尋函式
// 傳入 SQLite DB 與 ToolKit，回傳可設定給 Agent.OnMemorySearch 的回調函式
func BuildMemorySearchFunc(db *database.DB, tk *memory.ToolKit) func(query string) string {
	if db == nil && tk == nil {
		return nil
	}

	return func(query string) string {
		// 自適應檢索：判斷是否需要記憶搜尋 (memory-lancedb-pro)
		if memory.ShouldSkipRetrieval(query) {
			return ""
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		lower := strings.ToLower(query)
		var sb strings.Builder
		foundAny := false

		// 1. 短期記憶搜尋 (SQLite)
		if db != nil {
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

			if source != "" {
				entries, err := db.GetShortTermMemoryBySource(ctx, source, 3)
				if err == nil && len(entries) > 0 {
					foundAny = true
					sb.WriteString("[MEMORY CONTEXT] 以下是系統短期記憶中的相關資訊：\n")
					for i, e := range entries {
						content := strings.TrimSpace(e.Content)
						if len(content) > 1000 {
							content = content[:1000] + "...«已截斷»"
						}
						sb.WriteString(fmt.Sprintf("\n--- 近期紀錄 %d [%s] ---\n%s\n", i+1, e.CreatedAt, content))
					}
					sb.WriteString("\n")
				}
			}
		}

		// 2. 長期記憶混合搜尋 (BM25 + Vector Semantic Search)
		if tk != nil && len(strings.TrimSpace(query)) > 0 {
			resp, err := tk.MemorySearch(ctx, query)
			if err == nil && len(resp.Results) > 0 {
				if !foundAny {
					sb.WriteString("[MEMORY CONTEXT] 以下是長期記憶庫中的高度相關背景知識。\n⚠️【最高優先級警告】：這份背景知識代表使用者實際的生活或專案背景，你必須「絕對無條件信任」並「優先使用」這裡提供的所有名詞、日期與事實來回答問題，嚴禁擅自使用目前的系統時間或其他外部知識進行覆寫：\n")
				} else {
					sb.WriteString("【長期深度記憶】\n")
				}

				for i, res := range resp.Results {
					if i >= 3 { // 最多取前 3 筆避免塞爆 prompt
						break
					}
					content := strings.TrimSpace(res.Chunk.Content)
					runes := []rune(content)
					if len(runes) > 1500 {
						content = string(runes[:1500]) + "...«已截斷»"
					}
					// 輸出 debug 以了解為何常常被略過
					fmt.Printf("[Memory Debug] Match %d: FinalScore=%.3f, VectorScore=%.3f, TextScore=%.3f\n", i, res.FinalScore, res.VectorScore, res.TextScore)

					// 調高閾值，避免過度匹配無關指令 (原本是 > 0.05)
					// 若文字匹配很低但向量匹配很高，通常是語義漂移（如 browser vs someone's name in embedding space）
					if (res.FinalScore > 0.4 && res.TextScore > 0.1) || res.TextScore > 0.5 {
						sb.WriteString(fmt.Sprintf("\n--- 背景知識 %d ---\n%s\n", i+1, content))
						foundAny = true
					} else {
						fmt.Printf("[Memory Debug] Match %d dropped due to low confidence.\n", i)
					}
				}
			}
		}

		if !foundAny {
			return ""
		}

		// 加入收尾提示
		sb.WriteString("\n⚠️【注意】：若上述內容包含能直接回答使用者問題的證據，請優先引用。但若使用者明確要求執行特定操作（如打開網頁、讀取郵件或操作檔案），你必須『立即執行』對應工具，而不僅僅是依靠記憶中舊有的資訊。")
		sb.WriteString("\n你的身分是 PCAI (F.R.I.D.A.Y)，絕對不是使用者本人。")

		return sb.String()
	}
}
