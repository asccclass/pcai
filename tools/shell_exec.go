package tools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/ollama/ollama/api"
)

type ShellExecTool struct {
	Mgr *BackgroundManager // 對接到背景管理器
}

// sanitizeCommand 確保指令不會因為多餘的轉義而失效
func (t *ShellExecTool) sanitizeCommand(cmd string) string {
	// 移除頭尾可能出現的各種引號 (AI 有時會自作聰明加引號)
	cmd = strings.Trim(cmd, "\"`' ")

	// 修正 AI 常見的指令錯誤
	cmd = strings.ReplaceAll(cmd, "delete ", "rm -f ")

	// 如果 AI 傳入了轉義過的引號 (如 \"), 將其轉回正常引號
	cmd = strings.ReplaceAll(cmd, `\"`, `"`)

	return cmd
}

func (t *ShellExecTool) Name() string { return "shell_exec" }

func (t *ShellExecTool) Definition() api.Tool {
	return api.Tool{
		Type: "function",
		Function: api.ToolFunction{
			Name:        "shell_exec",
			Description: "CRITICAL: 執行本地 Shell 指令。當使用者要求檔案操作、目錄查看或系統指令時，必須使用此工具。對於耗時任務（如編譯、長時間監控、大檔下載），務必將 async 設為 true 以便在背景執行。",
			// 關鍵修正點：加上型別轉型
			Parameters: func() api.ToolFunctionParameters {
				var props api.ToolPropertiesMap
				// ToolPropertiesMap has unexported fields, so we initialize it via JSON
				js := `{
					"command": {
						"type": "string",
						"description": "完整的指令字串。如果是 Linux 請用 rm, ls, cp；Windows 請用 del, dir, copy。"
					},
					"async": {
						"type":        "boolean",
						"description": "是否在背景執行。如果任務預計超過 3 秒，請設為 true。",
					},
					"required": []string{"command"},
				}`
				_ = json.Unmarshal([]byte(js), &props)

				return api.ToolFunctionParameters{
					Type:       "object",
					Properties: &props,
					Required:   []string{"command"},
				}
			}(),
		},
	}
}

// 實際執行 shell 的內部函式
func (t *ShellExecTool) execute(command string) (string, error) {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/C", command)
	} else {
		cmd = exec.Command("sh", "-c", command)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("執行錯誤: %v, 輸出: %s", err, stderr.String())
	}

	outStr := stdout.String()
	if outStr == "" {
		return "指令執行成功 (無輸出內容)。", nil
	}
	return outStr, nil
}

func (t *ShellExecTool) Run(argsJSON string) (string, error) {
	var args struct {
		Command string `json:"command"`
		Async   any    `json:"async"` // 新增參數
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", err
	}
	// 指令清洗：處理 AI 可能多包的一層引號或轉義字元
	cleanCommand := t.sanitizeCommand(args.Command)

	// 非同步判斷 (相容性處理)
	isAsync := false
	switch v := args.Async.(type) {
	case bool:
		isAsync = v
	case string:
		isAsync = strings.ToLower(v) == "true"
	}

	// 判斷是否為非同步執行
	if isAsync && t.Mgr != nil {
		taskID := t.Mgr.AddTask(cleanCommand, func() (string, error) {
			return t.execute(cleanCommand)
		})
		return fmt.Sprintf("✅ 任務已在背景啟動 (ID: #%d)。你可以繼續跟我聊天，完成後我會通知你。", taskID), nil
	}
	// 同步執行
	return t.execute(cleanCommand)
}
