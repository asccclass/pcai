package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ollama/ollama/api"
)

// EmailTool 讓 LLM 可以主動查詢一般郵件 (透過 gog CLI)
// 注意：此物件主要為了相容於舊有註冊流程，現在已改為支援 action 與 args
type EmailTool struct{}

type EmailToolArgs struct {
	Action string `json:"action"`
	Args   string `json:"args,omitempty"`
}

func (t *EmailTool) Name() string { return "manage_email" }

func (t *EmailTool) IsSkill() bool {
	return false
}

func (t *EmailTool) Definition() api.Tool {
	return api.Tool{
		Type: "function",
		Function: api.ToolFunction{
			Name:        "manage_email",
			Description: "使用 gog 工具讀取 Gmail 郵件。當使用者詢問「有沒有新信」、「查看最近的 Email」時使用。",
			Parameters: func() api.ToolFunctionParameters {
				var props api.ToolPropertiesMap
				js := `{
					"action": {
						"type": "string",
						"description": "要執行的操作 (例如 search, get, send 等)。"
					},
					"args": {
						"type": "string",
						"description": "傳遞給該指令的其他參數與數值 (選填)。請注意，如果是 query 字串本身，在 Windows 下務必用雙引號包覆，例如 '\"is:unread\" --max 10'。"
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

func (t *EmailTool) Run(args string) (string, error) {
	var a EmailToolArgs
	if args != "" {
		if err := json.Unmarshal([]byte(args), &a); err != nil {
			return "", fmt.Errorf("解析參數失敗: %v", err)
		}
	}

	if a.Action == "" {
		a.Action = "search"
	}

	// 1. 決定 gog 執行檔路徑
	binPath := os.Getenv("GOG_PATH")
	found := false

	if binPath != "" {
		if _, err := os.Stat(binPath); err == nil {
			found = true
		}
	}

	if !found {
		binName := "gog"
		if runtime.GOOS == "windows" {
			binName = "gog.exe"
		}
		cwd, _ := os.Getwd()

		possiblePaths := []string{
			filepath.Join(cwd, "bin", binName),
		}

		if home := os.Getenv("USERPROFILE"); home != "" {
			possiblePaths = append(possiblePaths, filepath.Join(home, "go", "bin", binName))
		}
		if goPath := os.Getenv("GOPATH"); goPath != "" {
			possiblePaths = append(possiblePaths, filepath.Join(goPath, "bin", binName))
		}

		for _, p := range possiblePaths {
			if _, err := os.Stat(p); err == nil {
				binPath = p
				found = true
				break
			}
		}

		if !found {
			if path, err := exec.LookPath(binName); err == nil {
				binPath = path
			} else {
				binPath = binName // Last resort
			}
		}
	}

	// 2. 組建指令 (利用 shell 執行以正確解析雙引號/單引號等複雜參數)
	// command = gog gmail <action> <args>
	// 為了避免引號解析錯誤，我們直接用 cmd /c 來執行
	finalCmd := fmt.Sprintf("%s gmail %s %s", binPath, a.Action, a.Args)
	fmt.Printf("🔧 [EmailTool] Executing: %s\n", finalCmd)

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/C", finalCmd)
	} else {
		cmd = exec.Command("sh", "-c", finalCmd)
	}

	cmd.Env = os.Environ()

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("執行 %s 失敗: %v\nOutput: %s", finalCmd, err, string(output))
	}

	res := string(output)
	if strings.TrimSpace(res) == "" {
		return "📭 執行成功，但無輸出內容。", nil
	}

	return fmt.Sprintf("📧 **執行結果** (via gog):\n%s", res), nil
}
