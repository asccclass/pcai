package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/ollama/ollama/api"
)

type GoogleTool struct {
	BinPath string
}

func NewGoogleTool() *GoogleTool {
	// 預設找當前目錄下的 bin/gog.exe 或 gog
	// 也可以從環境變數 GOG_BIN 讀取
	binName := "gog"
	if runtime.GOOS == "windows" {
		binName = "gog.exe"
	}

	// 假設 bin 在工作目錄的 bin/ 下
	cwd, _ := os.Getwd()
	binPath := filepath.Join(cwd, "bin", binName)

	return &GoogleTool{
		BinPath: binPath,
	}
}

func (t *GoogleTool) Name() string { return "google_services" }

func (t *GoogleTool) Definition() api.Tool {
	return api.Tool{
		Type: "function",
		Function: api.ToolFunction{
			Name: "google_services",
			Description: "使用 Google Services (Gmail, Calendar, Drive, etc) 進行操作。\n" +
				"常用範例：\n" +
				"1. 檢查今日行事曆 (預設): service='calendar', command='events', args=['--today']\n" +
				"2. 檢查特定人行事曆: service='calendar', command='events', args=['user@example.com', '--today']\n" +
				"3. 寄信: service='gmail', command='send', args=['--to', 'user@example.com', '--subject', 'Hi', '--body', 'Content']\n" +
				"4. 搜尋郵件: service='gmail', command='search', args=['newer_than:1d']\n" +
				"注意：gogcli 的 <calendarId> 是位置參數，請直接放在 args 的第一個元素，不要用 --email 標籤。",
			Parameters: func() api.ToolFunctionParameters {
				var props api.ToolPropertiesMap
				js := `{
					"service": {
						"type": "string",
						"description": "服務名稱，例如 'gmail', 'calendar', 'drive', 'tasks', 'contacts'"
					},
					"command": {
						"type": "string",
						"description": "指令，例如 'events' (行事曆), 'send' (寄信), 'search' (搜尋), 'list' (列表)"
					},
					"args": {
						"type": "array",
						"items": { "type": "string" },
						"description": "指令參數列表。注意：位置參數(如 calendarId)應直接列出，flag 參數(如 --today)則作為獨立字串列出。"
					}
				}`
				_ = json.Unmarshal([]byte(js), &props)
				return api.ToolFunctionParameters{
					Type:       "object",
					Properties: &props,
					Required:   []string{"service", "command"},
				}
			}(),
		},
	}
}

// sanitizeCommand 簡單過濾危險字元 (雖然是透過 exec.Command 呼叫，但還是小心點)
func (t *GoogleTool) sanitize(input string) string {
	return input // 暫時不做過多處理，依賴 exec.Command 的參數分離特性
}

func (t *GoogleTool) Run(argsJSON string) (string, error) {
	var args struct {
		Service string   `json:"service"`
		Command string   `json:"command"`
		Args    []string `json:"args"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("参數解析失敗: %v", err)
	}

	// 檢查執行檔是否存在
	if _, err := os.Stat(t.BinPath); os.IsNotExist(err) {
		return "", fmt.Errorf("找不到 gogcli 執行檔: %s", t.BinPath)
	}

	// 組合指令: gog <service> <command> [args...]
	// 例如: gog gmail search "newer_than:2d"
	// 或是 gog calendar events --today

	// 參數檢查
	if args.Service == "" {
		return "", fmt.Errorf("service 參數不能為空")
	}

	// 建構 exec 參數
	execArgs := []string{args.Service}
	if args.Command != "" {
		execArgs = append(execArgs, args.Command)
	}
	execArgs = append(execArgs, args.Args...)

	// 準備執行 (加上 GOG_JSON=1 強制輸出 JSON 格式方便解析，或者保持原樣讓 LLM 閱讀?)
	// 考慮到 LLM 閱讀能力，人類可讀的格式可能更好。gogcli 預設就是人類可讀的表格。
	// 如果需要 JSON 可以透過 args 傳入 --json

	cmd := exec.Command(t.BinPath, execArgs...)

	// 設定環境變數，確保編碼正確 (Windows 尤其重要)
	cmd.Env = os.Environ()
	// 如果需要，可以設定 GOG_ACCOUNT 等環境變數

	// 執行
	output, err := cmd.CombinedOutput()
	if err != nil {
		// gogcli 有時會回傳非 0 狀態碼但輸出有用的錯誤訊息
		return fmt.Sprintf("執行失敗 (Code %v):\n%s", err, string(output)), nil
	}

	return string(output), nil
}
