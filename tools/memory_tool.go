package tools

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/asccclass/pcai/internal/memory"
	"github.com/ollama/ollama/api"
)

// MemoryTool 實作 AgentTool 介面
type MemoryTool struct {
	manager  *memory.Manager
	topK     int     // 每次搜尋回傳幾筆 (例如 3)
	minScore float64 // 最低相關度門檻 (例如 0.5)
}

// NewMemoryTool 初始化工具
func NewMemoryTool(m *memory.Manager) *MemoryTool {
	return &MemoryTool{
		manager:  m,
		topK:     3,   // 預設值
		minScore: 0.4, // 預設值
	}
}

// Name 回傳工具名稱
func (t *MemoryTool) Name() string {
	return "memory_search"
}

// Definition 回傳 Ollama 所需的工具定義 (JSON Schema)
func (t *MemoryTool) Definition() api.Tool {
	return api.Tool{
		Type: "function",
		Function: api.ToolFunction{
			Name:        "memory_search",
			Description: "用於檢索過去的對話記錄、專案知識或使用者偏好。當你不確定問題答案，或覺得以前曾經討論過時，請使用此工具。",
			Parameters: func() api.ToolFunctionParameters {
				var props api.ToolPropertiesMap
				js := `{
					"query": {
						"type": "string",
						"description": "搜尋關鍵字或問題，例如 '專案的 API Key 是多少' 或 '上次會議的結論'"
					}
				}`
				_ = json.Unmarshal([]byte(js), &props)

				return api.ToolFunctionParameters{
					Type:       "object",
					Properties: &props,
					Required:   []string{"query"},
				}
			}(),
		},
	}
}

// Run 執行工具邏輯
func (t *MemoryTool) Run(argsJSON string) (string, error) {
	// 1. 定義參數結構 (對應 Definition 中的 properties)
	var args struct {
		Query string `json:"query"`
	}

	// 2. 解析 JSON 參數
	// LLM 有時會回傳包裹在 markdown block 的 json，這裡做簡單清洗
	cleanJSON := strings.Trim(argsJSON, "`json\n ")

	if err := json.Unmarshal([]byte(cleanJSON), &args); err != nil {
		return "", fmt.Errorf("解析工具參數失敗: %w (原始輸入: %s)", err, argsJSON)
	}

	// 檢查參數是否為空
	if args.Query == "" {
		return "錯誤: 搜尋查詢 (query) 不能為空。", nil
	}

	// 3. 呼叫 Memory Manager 進行搜尋
	results, err := t.manager.Search(args.Query, t.topK, t.minScore)
	if err != nil {
		return "", fmt.Errorf("搜尋執行錯誤: %w", err)
	}

	// 4. 格式化結果回傳給 LLM
	if len(results) == 0 {
		return "記憶庫搜尋結果: 未找到相關資訊。", nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("找到 %d 條相關記憶:\n", len(results)))
	for i, res := range results {
		sb.WriteString(fmt.Sprintf("--- 結果 %d (相關度: %.2f) ---\n", i+1, res.Score))
		sb.WriteString(res.Entry.Content)
		sb.WriteString("\n")
	}

	return sb.String(), nil
}
