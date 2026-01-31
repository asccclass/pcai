package tools

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ollama/ollama/api"
)

type KnowledgeAppendTool struct{}

func (t *KnowledgeAppendTool) Name() string { return "knowledge_append" }

func (t *KnowledgeAppendTool) Definition() api.Tool {
	return api.Tool{
		Type: "function",
		Function: api.ToolFunction{
			Name:        "knowledge_append",
			Description: "記錄使用者的個人事實、偏好或工作筆記。請根據內容自動選擇最合適的分類標籤。這將永久存入長期記憶庫。",
			Parameters: func() api.ToolFunctionParameters {
				var props api.ToolPropertiesMap
				js := `
					"content": {
						"type":        "string",
						"description": "要記錄的的事實內容，請使用繁體中文，並以簡短事實的形式呈現。",
					},
					"category": {
						"type":        "string",
						"description": "分類標籤。可選：個人資訊、工作紀錄、偏好設定、生活雜記、技術開發。",
					}
				`
				_ = json.Unmarshal([]byte(js), &props)

				return api.ToolFunctionParameters{
					Type:       "object",
					Properties: &props,
					Required:   []string{"content", "category"},
				}
			}(),
		},
	}
}

func (t *KnowledgeAppendTool) Run(argsJSON string) (string, error) {
	var args struct {
		Content  string `json:"content"`
		Category string `json:"category"`
	}
	// 解析 JSON
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", err
	}

	// 定義檔案路徑 (與 RAG 讀取路徑一致)
	home, _ := os.Getwd()
	dir := filepath.Join(home, "botmemory", "knowledge")
	filePath := filepath.Join(dir, "knowledge.md")

	// 確保目錄存在
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("無法建立目錄: %v", err)
	}

	// 格式化要寫入的內容 (加上時間戳記)
	// 格式化寫入內容：包含日期與分類標籤
	// 格式範例：- [2026-01-31] #個人資訊: 使用者姓名為劉智漢。
	entry := fmt.Sprintf("\n- [%s] #%s: %s",
		time.Now().Format("2006-01-02"),
		args.Category,
		args.Content,
	)
	// 以 Append 模式開啟檔案
	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return "", fmt.Errorf("無法開啟記憶檔案: %v", err)
	}
	defer f.Close()

	if _, err := f.WriteString(entry); err != nil {
		return "", fmt.Errorf("寫入記憶失敗: %v", err)
	}
	return fmt.Sprintf("✅ 已分類為 #%s 並存入記憶庫。", args.Category), nil
}

// GetKnowledgeStats 統計知識庫中的標籤分佈
func GetKnowledgeStats() string {
	home, _ := os.Getwd()
	filePath := filepath.Join(home, "botmemory", "knowledge", "knowledge.md")

	file, err := os.Open(filePath)
	if err != nil {
		return "尚未建立記憶"
	}
	defer file.Close()

	stats := make(map[string]int)
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Text()
		// 尋找符合 #標籤: 格式的行
		if strings.Contains(line, "#") {
			parts := strings.Split(line, "#")
			if len(parts) > 1 {
				// 取得標籤名稱（到冒號或空格為止）
				tagPart := strings.Split(parts[1], ":")[0]
				tagPart = strings.TrimSpace(tagPart)
				if tagPart != "" {
					stats[tagPart]++
				}
			}
		}
	}

	if len(stats) == 0 {
		return "無分類標籤"
	}

	var result []string
	for tag, count := range stats {
		result = append(result, fmt.Sprintf("#%s(%d)", tag, count))
	}
	return strings.Join(result, ", ")
}
