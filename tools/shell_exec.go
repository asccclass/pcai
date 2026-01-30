package tools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"

	"github.com/ollama/ollama/api"
)

type ShellExecTool struct{}

func (t *ShellExecTool) Name() string { return "shell_exec" }

func (t *ShellExecTool) Definition() api.Tool {
	return api.Tool{
		Type: "function",
		Function: api.ToolFunction{
			Name:        "shell_exec",
			Description: "在本地系統執行 shell 指令。",
			// 關鍵修正點：加上型別轉型
			Parameters: func() api.ToolFunctionParameters {
				var props api.ToolPropertiesMap
				// ToolPropertiesMap has unexported fields, so we initialize it via JSON
				js := `{
					"command": {
						"type": "string",
						"description": "要執行的完整指令，例如 'ls -la'"
					}
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

func (t *ShellExecTool) Run(argsJSON string) (string, error) {
	var args struct {
		Command string `json:"command"`
	}
	json.Unmarshal([]byte(argsJSON), &args)

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/C", args.Command)
	} else {
		cmd = exec.Command("sh", "-c", args.Command)
	}

	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	err := cmd.Run()

	if err != nil {
		return "", fmt.Errorf("執行錯誤: %v, 錯誤輸出: %s", err, stderr.String())
	}
	return out.String(), nil
}
