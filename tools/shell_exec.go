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
	Mgr     *BackgroundManager // 對接到背景管理器
	Manager *FileSystemManager // Sandbox Manager
}

// sanitizeCommand 確保指令不會因為多餘的轉義而失效
func (t *ShellExecTool) sanitizeCommand(cmd string) string {
	// 先將被轉義的引號還原 (把 \" 變回 ")
	cmd = strings.ReplaceAll(cmd, `\"`, `"`)
	// 移除最外層可能被 AI 多包的雙引號或單引號。例如 "ls -la" 變成 ls -la
	cmd = strings.Trim(cmd, `"'`)
	// 處理常見的刪除指令修正
	if strings.HasPrefix(cmd, "delete ") {
		cmd = strings.Replace(cmd, "delete ", "rm -f ", 1)
	}

	// [FIX] OS 感知指令翻譯：避免 Linux 指令在 Windows 上失敗
	if runtime.GOOS == "windows" {
		linuxToWindows := map[string]string{
			"ls":     "dir",
			"ls -l":  "dir",
			"ls -a":  "dir /a",
			"ls -la": "dir /a",
			"ls -al": "dir /a",
			"pwd":    "cd",
			"cat":    "type",
			"rm":     "del",
			"cp":     "copy",
			"mv":     "move",
			"clear":  "cls",
		}
		trimmed := strings.TrimSpace(cmd)
		// 先嘗試完整匹配
		if winCmd, ok := linuxToWindows[trimmed]; ok {
			cmd = winCmd
		} else {
			// 嘗試只匹配指令部分 (第一個 token)
			parts := strings.SplitN(trimmed, " ", 2)
			if winCmd, ok := linuxToWindows[parts[0]]; ok {
				if len(parts) > 1 {
					cmd = winCmd + " " + parts[1]
				} else {
					cmd = winCmd
				}
			}
		}
	}

	// 最後做一次前後空白修剪
	return strings.TrimSpace(cmd)
}

func (t *ShellExecTool) Name() string { return "shell_exec" }

func (t *ShellExecTool) Definition() api.Tool {
	return api.Tool{
		Type: "function",
		Function: api.ToolFunction{
			Name:        "shell_exec",
			Description: "CRITICAL: 執行本地 Shell 指令。注意：所有操作將被限制在工作區目錄中 (Sandbox)。",
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
		// [FIX] Force UTF-8 encoding (chcp 65001) to avoid garbled text (mojibake)
		cmd = exec.Command("cmd", "/C", "chcp 65001 && "+command)
	} else {
		cmd = exec.Command("sh", "-c", command)
	}

	// 強制設定工作目錄為 Sandbox Root
	if t.Manager != nil {
		cmd.Dir = t.Manager.RootPath
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
