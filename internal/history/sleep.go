package history

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// OptimizeAutoSummaries 會讀取目前的 auto_summaries.md，並要求 LLM 合併碎片化的紀錄。
// 類似人類晚間睡夢中的長期記憶重整與壓實過程。
func OptimizeAutoSummaries(ctx context.Context, llmAsk func(string) (string, error)) error {
	home, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("獲取工作目錄失敗: %w", err)
	}

	path := filepath.Join(home, "botmemory", "history", "auto_summaries.md")

	// 1. 讀取檔案
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// 檔案不存在，代表沒有需要最佳化的歷史，不視為錯誤
			return nil
		}
		return fmt.Errorf("讀取 %s 失敗: %w", path, err)
	}

	content := string(data)

	// 2. 避免文本過短不需處理 (簡單字數限制：低於 300 個字元就不特別呼叫 LLM 浪費資源)
	if len(content) < 300 {
		return nil
	}

	fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("63")).Render("\n🌙 [Memory Sleep] 系統正在背景進行記憶重整（睡眠優化）..."))

	// 3. 準備 LLM 的 Prompt
	prompt := fmt.Sprintf(`
你現在負責執行人類大腦的「睡眠記憶重整合併」任務。
請扮演一個冷靜、高效的整理專家，閱讀以下過往累積的碎片化記憶日誌。

你的目標：
1. 將所有相關聯的事件合併。
2. 消除重複出現或互相覆蓋的資訊（若事件被取消或刪除，請反映最終結果）。
3. 去除不必要的瑣碎細節，保留【人、事、時、地、物】等核心事實。
4. 以俐落且結構清晰的 Markdown 【無序列表】格式輸出合併後的最終記憶清單。
5. 嚴禁編造沒出現過的內容，如果內容互相矛盾，請根據常理或時序脈絡整合。

以下是待整理的記憶日誌：
---
%s
---

請直接輸出整理後的 Markdown 無序列表結果，不要有任何開場白或結語。
`, content)

	// 4. 交給 LLM 處理
	optimizedContent, err := llmAsk(prompt)
	if err != nil {
		return fmt.Errorf("LLM 重整記憶失敗: %w", err)
	}

	if strings.TrimSpace(optimizedContent) == "" {
		return fmt.Errorf("LLM 回傳內容為空，取消覆寫")
	}

	// 加上標頭戳記證明這是一份重整過後的日誌
	newContent := fmt.Sprintf("\n\n## [Optimized Sleep Summary] %s\n%s\n---\n",
		time.Now().Format("2006-01-02 15:04"), optimizedContent)

	// 5. 將重整過後的結果覆寫回檔案
	// 先備份舊黨策全
	backupPath := path + ".bak"
	_ = os.WriteFile(backupPath, data, 0644)

	err = os.WriteFile(path, []byte(newContent), 0644)
	if err != nil {
		// 嘗試復原
		_ = os.Rename(backupPath, path)
		return fmt.Errorf("覆寫 auto_summaries.md 失敗: %w", err)
	}

	// 刪除備份
	_ = os.Remove(backupPath)

	fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("35")).Render("✨ [Memory Sleep] 記憶重整完畢！已成功優化並縮減容量。"))

	return nil
}
