package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/asccclass/pcai/internal/memory"
	"github.com/ollama/ollama/api"
)

// MemoryConfirmTool 確認或拒絕待寫入的記憶
type MemoryConfirmTool struct {
	manager      *memory.Manager
	pending      *memory.PendingStore
	markdownPath string
}

// NewMemoryConfirmTool 建立新的確認工具
func NewMemoryConfirmTool(m *memory.Manager, ps *memory.PendingStore, mdPath string) *MemoryConfirmTool {
	return &MemoryConfirmTool{
		manager:      m,
		pending:      ps,
		markdownPath: mdPath,
	}
}

func (t *MemoryConfirmTool) Name() string {
	return "memory_confirm"
}

func (t *MemoryConfirmTool) Definition() api.Tool {
	return api.Tool{
		Type: "function",
		Function: api.ToolFunction{
			Name:        "memory_confirm",
			Description: "確認或拒絕待寫入的記憶。當使用者回覆「確認」、「好」、「是」時執行 confirm；回覆「取消」、「不要」時執行 reject。也可以使用 confirm_all / reject_all 批次操作。",
			Parameters: func() api.ToolFunctionParameters {
				var props api.ToolPropertiesMap
				js := `{
					"action": {
						"type": "string",
						"description": "操作類型：confirm (確認寫入), reject (拒絕), confirm_all (全部確認), reject_all (全部拒絕)",
						"enum": ["confirm", "reject", "confirm_all", "reject_all"]
					},
					"pending_id": {
						"type": "string",
						"description": "待確認記憶的 ID (confirm/reject 時需要，confirm_all/reject_all 不需要)"
					}
				}`
				_ = json.Unmarshal([]byte(js), &props)
				return api.ToolFunctionParameters{
					Type:       "object",
					Properties: &props,
					Required:   []string{"action"},
				}
			}(),
		},
	}
}

func (t *MemoryConfirmTool) Run(argsJSON string) (string, error) {
	var args struct {
		Action    string `json:"action"`
		PendingID string `json:"pending_id"`
	}
	cleanJSON := strings.Trim(argsJSON, "`json\n ")
	if err := json.Unmarshal([]byte(cleanJSON), &args); err != nil {
		return "", fmt.Errorf("參數錯誤: %w", err)
	}

	switch args.Action {
	case "confirm":
		if args.PendingID == "" {
			return "錯誤：confirm 操作需要提供 pending_id", nil
		}
		entry, err := t.pending.Confirm(args.PendingID)
		if err != nil {
			return fmt.Sprintf("確認失敗: %v", err), nil
		}
		return t.saveEntry(entry)

	case "reject":
		if args.PendingID == "" {
			return "錯誤：reject 操作需要提供 pending_id", nil
		}
		if err := t.pending.Reject(args.PendingID); err != nil {
			return fmt.Sprintf("拒絕失敗: %v", err), nil
		}
		return "已取消該筆記憶寫入。", nil

	case "confirm_all":
		entries := t.pending.ConfirmAll()
		if len(entries) == 0 {
			return "目前沒有待確認的記憶。", nil
		}
		var results []string
		for _, entry := range entries {
			msg, err := t.saveEntry(entry)
			if err != nil {
				results = append(results, fmt.Sprintf("❌ 寫入失敗: %v", err))
			} else {
				results = append(results, msg)
			}
		}
		return strings.Join(results, "\n"), nil

	case "reject_all":
		count := t.pending.RejectAll()
		if count == 0 {
			return "目前沒有待確認的記憶。", nil
		}
		return fmt.Sprintf("已取消全部 %d 筆待確認記憶。", count), nil

	default:
		return fmt.Sprintf("不支援的操作: %s (支援: confirm, reject, confirm_all, reject_all)", args.Action), nil
	}
}

// saveEntry 將確認的記憶寫入向量庫和 Markdown
func (t *MemoryConfirmTool) saveEntry(entry *memory.PendingEntry) (string, error) {
	// 1. 寫入向量資料庫
	if err := t.manager.Add(entry.Content, entry.Tags); err != nil {
		return "", fmt.Errorf("寫入記憶庫失敗: %w", err)
	}

	// 2. 寫入 Markdown 檔案
	if t.markdownPath != "" {
		f, err := os.OpenFile(t.markdownPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err == nil {
			defer f.Close()
			if _, err := f.WriteString("\n\n" + entry.Content); err != nil {
				fmt.Printf("警告: 無法寫入 Markdown 檔案: %v\n", err)
			}
		}
	}

	return fmt.Sprintf("✅ 已確認並儲存記憶: \"%s\"", entry.Content), nil
}
