package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ollama/ollama/api"
)

// ReportMissingTool 允許 LLM 回報系統缺少的工具或技能
type ReportMissingTool struct{}

func (t *ReportMissingTool) Name() string { return "report_missing_tool" }

func (t *ReportMissingTool) Definition() api.Tool {
	var tool api.Tool
	jsonStr := `{
		"type": "function",
		"function": {
			"name": "report_missing_tool",
			"description": "當使用者的指令需要某個工具或技能，而系統目前沒有該功能時，請呼叫此工具。系統會記錄下來以供未來開發。",
			"parameters": {
				"type": "object",
				"properties": {
					"user_instruction": {
						"type": "string",
						"description": "使用者原始的指令"
					},
					"missing_functionality": {
						"type": "string",
						"description": "缺少的工具或技能描述"
					}
				},
				"required": ["user_instruction", "missing_functionality"]
			}
		}
	}`
	if err := json.Unmarshal([]byte(jsonStr), &tool); err != nil {
		fmt.Printf("⚠️ [ReportMissingTool] Definition JSON error: %v\n", err)
	}
	return tool
}
func (t *ReportMissingTool) Run(argsJSON string) (string, error) {
	var args struct {
		UserInstruction      string `json:"user_instruction"`
		MissingFunctionality string `json:"missing_functionality"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("解析參數失敗: %v", err)
	}

	// 記錄到檔案
	if err := LogMissingToolEvent("ManualReport", args.UserInstruction, args.MissingFunctionality); err != nil {
		return "", fmt.Errorf("記錄失敗: %v", err)
	}

	return fmt.Sprintf("已收到回報。系統目前無法執行此指令，因為缺少功能: %s。已記錄至 notools.log。", args.MissingFunctionality), nil
}

// LogMissingToolEvent 是一個共用的 logging 函式
func LogMissingToolEvent(eventType, instruction, missing string) error {
	home, _ := os.Getwd()
	logPath := filepath.Join(home, "botmemory", "self_test_reports", "notools.log")

	// 確保目錄存在
	logDir := filepath.Dir(logPath)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return err
	}

	entry := map[string]string{
		"timestamp":   time.Now().Format(time.RFC3339),
		"type":        eventType, // "ManualReport" or "Hallucination"
		"instruction": instruction,
		"missing":     missing,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := f.WriteString(string(data) + "\n"); err != nil {
		return err
	}

	return nil
}
