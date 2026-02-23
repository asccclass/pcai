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
	// 優先使用環境變數設定
	if envPath := os.Getenv("GOG_PATH"); envPath != "" {
		if _, err := os.Stat(envPath); err == nil {
			return &GoogleTool{
				BinPath: envPath,
			}
		}
	}

	// 預設找當前目錄下的 bin/gog.exe 或 gog
	// 若找不到，則嘗試從系統 PATH 尋找
	binName := "gog"
	if runtime.GOOS == "windows" {
		binName = "gog.exe"
	}

	// 1. Check local bin/
	cwd, _ := os.Getwd()

	possiblePaths := []string{
		filepath.Join(cwd, "bin", binName),
	}

	// 嘗試從環境變數 USERPROFILE 組合路徑
	if home := os.Getenv("USERPROFILE"); home != "" {
		possiblePaths = append(possiblePaths, filepath.Join(home, "go", "bin", binName))
	}
	// 嘗試 GOPATH
	if goPath := os.Getenv("GOPATH"); goPath != "" {
		possiblePaths = append(possiblePaths, filepath.Join(goPath, "bin", binName))
	}

	var finalPath string
	found := false
	for _, p := range possiblePaths {
		if _, err := os.Stat(p); err == nil {
			finalPath = p
			found = true
			break
		}
	}

	if !found {
		// 2. Try to find in PATH
		if path, err := exec.LookPath(binName); err == nil {
			finalPath = path
		} else {
			// Fallback: just use the name and hope for the best at runtime
			finalPath = binName
		}
	}

	return &GoogleTool{
		BinPath: finalPath,
	}
}

func (t *GoogleTool) Name() string { return "google_services" }

func (t *GoogleTool) IsSkill() bool {
	return false
}

func (t *GoogleTool) Definition() api.Tool {
	return api.Tool{
		Type: "function",
		Function: api.ToolFunction{
			Name: "google_services",
			Description: "Access Google Services (Gmail, Calendar, etc).\n" +
				"Examples:\n" +
				"1. Check calendar: service='calendar', command='events', args=['--from', '2025-01-01', '--to', '2025-01-07']\n" +
				"   (NOTE: For reading your daily schedule/agenda, prefer using the 'read_calendars' skill if available, as it scans ALL calendars.)\n" +
				"2. Send email: service='gmail', command='send', args=['--to', 'user@example.com', '--subject', 'Hi', '--body', 'Content']\n" +
				"3. Search emails: service='gmail', command='search', args=['is:unread', '--limit', '10']\n" +
				"IMPORTANT: 'summary' is NOT a valid command. To summarize, use 'search' to get content, then summarize it yourself.\n" +
				"IMPORTANT: For Calendar, always calculate exact dates for --from/--to.",
			Parameters: func() api.ToolFunctionParameters {
				var props api.ToolPropertiesMap
				js := `{
					"service": {
						"type": "string",
						"description": "Service name: 'gmail', 'calendar', 'drive', 'tasks', 'contacts'"
					},
					"command": {
						"type": "string",
						"description": "Command. Gmail: 'search', 'send', 'get', 'labels'. Calendar: 'events' (or 'list' as alias)"
					},
					"args": {
						"type": "array",
						"items": { "type": "string" },
						"description": "Command arguments. for 'events', use '--all' to fetch from ALL calendars. Use '--from'/'--to' for dates."
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
	// Parse into map[string]interface{} to handle wrapped values
	var rawArgs map[string]interface{}
	if err := json.Unmarshal([]byte(argsJSON), &rawArgs); err != nil {
		return "", fmt.Errorf("参数解析失敗: %v", err)
	}

	// Use helper functions to extract values robustly
	service := ToString(rawArgs["service"])
	command := ToString(rawArgs["command"])
	args := ToStringSlice(rawArgs["args"])

	// 檢查執行檔是否存在
	if _, err := os.Stat(t.BinPath); os.IsNotExist(err) {
		return "", fmt.Errorf("找不到 gogcli 執行檔: %s", t.BinPath)
	}

	// 組合指令: gog <service> <command> [args...]
	// 例如: gog gmail search "newer_than:2d"
	// 或是 gog calendar events --today

	// 參數檢查
	if service == "" {
		return "", fmt.Errorf("service 參數不能為空")
	}

	// 建構 exec 參數
	execArgs := []string{service}
	if command != "" {
		execArgs = append(execArgs, command)
	}
	execArgs = append(execArgs, args...)

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

	fmt.Println(string(output))

	return string(output), nil
}
