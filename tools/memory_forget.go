// 由於刪除時間可能比較慢，所以可以放到背景執行
package tools

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/asccclass/pcai/internal/memory"
	"github.com/asccclass/pcai/internal/scheduler"

	"github.com/ollama/ollama/api"
)

type MemoryForgetTool struct {
	scheduler    *scheduler.Manager // 注入 Scheduler
	manager      *memory.Manager
	markdownPath string
	fileMutex    sync.Mutex // 互斥鎖：確保同一時間只有一個人在改檔案
}

func NewMemoryForgetTool(m *memory.Manager, s *scheduler.Manager, mdPath string) *MemoryForgetTool {
	return &MemoryForgetTool{
		manager:      m,
		scheduler:    s,
		markdownPath: mdPath,
	}
}

func (t *MemoryForgetTool) Name() string {
	return "memory_forget"
}

func (t *MemoryForgetTool) Definition() api.Tool {
	return api.Tool{
		Type: "function",
		Function: api.ToolFunction{
			Name:        "memory_forget",
			Description: "用於永久刪除記憶。這會同時從向量資料庫與原始檔案中移除資料。當使用者要求「忘記」、「刪除」某事時使用。",
			Parameters: func() api.ToolFunctionParameters {
				var props api.ToolPropertiesMap
				js := `{
					"content": {
						"type": "string",
						"description": "要刪除的記憶內容原文。必須儘可能精確匹配原始記憶。"
					}
				}`
				_ = json.Unmarshal([]byte(js), &props)
				return api.ToolFunctionParameters{
					Type:       "object",
					Properties: &props,
					Required:   []string{"content"},
				}
			}(),
		},
	}
}

func (t *MemoryForgetTool) Run(argsJSON string) (string, error) {
	var args struct {
		Content string `json:"content"`
	}
	// 清洗 JSON
	cleanJSON := strings.Trim(argsJSON, "`json\n ")
	if err := json.Unmarshal([]byte(cleanJSON), &args); err != nil {
		return "", fmt.Errorf("參數錯誤: %w", err)
	}

	// 同步執行：刪除向量資料庫 (RAM/JSON)
	// 這一步必須馬上做，確保 Agent 下一句話不會產生幻覺
	_, err := t.manager.DeleteByContent(args.Content)
	if err != nil {
		return "", fmt.Errorf("資料庫刪除錯誤: %w", err)
	}
	// 非同步操作：建立背景任務並派發給 Scheduler
	job := &memory.FileDeletionJob{
		FilePath: t.markdownPath,
		Content:  args.Content,
		Mutex:    &t.fileMutex,
	}

	// 假設 scheduler.Add 接受 tasks.FileDeletionJob
	if err := t.scheduler.AddBackgroundTask(job); err != nil {
		return "已從短期記憶移除，但背景同步任務排程失敗。", nil
	}
	return "我已經忘記這件事了，後台正在同步清理您的原始檔案。", nil
}
