package tools

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/ollama/ollama/api"
)

type KnowledgeSearchTool struct{}

func (t *KnowledgeSearchTool) Name() string { return "knowledge_search" }

func (t *KnowledgeSearchTool) Definition() api.Tool {
	return api.Tool{
		Type: "function",
		Function: api.ToolFunction{
			Name:        "knowledge_search",
			Description: "搜尋長期記憶知識庫 (knowledge.md) 中的內容。",
			Parameters: func() api.ToolFunctionParameters {
				var props api.ToolPropertiesMap
				js := `{
					"query": {
						"type": "string",
						"description": "要搜尋的關鍵字"
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

// ExtractMarkdownBlock 根據關鍵字尋找並回傳完整的區塊
func ExtractMarkdownBlock(filePath string, query string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	var foundBlocks []string
	lastHeaderIndex := -1

	for i, line := range lines {
		// 追蹤最近的標題位置 (假設我們以 ## 或 ### 為區塊基準)
		if strings.HasPrefix(line, "##") {
			lastHeaderIndex = i
		}

		// 發現關鍵字
		if strings.Contains(strings.ToLower(line), strings.ToLower(query)) {
			start := lastHeaderIndex
			if start == -1 {
				start = i // 如果前面沒標題，就從該行開始
			}

			// 尋找區塊結束點 (下一個相同或更高層級的標題)
			end := len(lines)
			for j := i + 1; j < len(lines); j++ {
				if strings.HasPrefix(lines[j], "##") {
					end = j
					break
				}
			}

			// 提取並組合區塊內容
			block := strings.Join(lines[start:end], "\n")

			// 避免重複添加同一個區塊
			isDuplicate := false
			for _, b := range foundBlocks {
				if b == block {
					isDuplicate = true
					break
				}
			}
			if !isDuplicate {
				foundBlocks = append(foundBlocks, block)
			}
		}
	}

	if len(foundBlocks) == 0 {
		return "找不到包含關鍵字的區塊。", nil
	}

	return strings.Join(foundBlocks, "\n\n---\n\n"), nil
}

func (t *KnowledgeSearchTool) Run(argsJSON string) (string, error) {
	var args struct {
		Query string `json:"query"`
	}
	json.Unmarshal([]byte(argsJSON), &args)

	home, _ := os.Getwd() // os.Executable()  返回執行檔案的絕對路徑。
	path := filepath.Join(home, "botmemory", "knowledge", "knowledge.md")
	result, err := ExtractMarkdownBlock(path, args.Query)
	if err != nil {
		return "目前沒有長期記憶紀錄。", nil
	}
	return "找到以下相關知識區塊：\n" + result, nil // "搜尋結果"
}
