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
type EmailTool struct{}

type EmailToolArgs struct {
	Query      string `json:"query,omitempty"`
	MaxResults int    `json:"max_results,omitempty"`
}

func (t *EmailTool) Name() string { return "read_email" }

func (t *EmailTool) Definition() api.Tool {
	return api.Tool{
		Type: "function",
		Function: api.ToolFunction{
			Name:        "read_email",
			Description: "使用 gog 工具讀取 Gmail 郵件。當使用者詢問「有沒有新信」、「查看最近的 Email」時使用。",
			Parameters: func() api.ToolFunctionParameters {
				var props api.ToolPropertiesMap
				js := `{
					"query": {
						"type": "string",
						"description": "搜尋關鍵字 (例如: 'from:boss', 'subject:meeting', 'is:unread')。若為空則預設列出最新郵件。"
					},
					"max_results": {
						"type": "integer",
						"description": "要讀取的最大郵件數量 (預設 5)"
					}
				}`
				_ = json.Unmarshal([]byte(js), &props)

				return api.ToolFunctionParameters{
					Type:       "object",
					Properties: &props,
					Required:   []string{},
				}
			}(),
		},
	}
}

func (t *EmailTool) Run(args string) (string, error) {
	// Parse into map[string]interface{} to handle wrapped values
	var rawArgs map[string]interface{}
	if args != "" {
		if err := json.Unmarshal([]byte(args), &rawArgs); err != nil {
			// Fallback: try unmarshalling into struct or just log?
			// Actually if it's a valid JSON object it should work.
			// But if args is just a string? args is JSON string.
		}
	}

	// Extract values
	query := ToString(rawArgs["query"])
	// MaxResults is int in struct. ToString handles numbers gracefully?
	// ToString returns "5" for int 5. We need Atoi or helper.
	// Let's rely on fmt.Sprintf in ToString for now, then Atoi.
	maxResultsStr := ToString(rawArgs["max_results"])
	maxResults := 5
	if maxResultsStr != "" {
		// Try parsing
		var val int
		if _, err := fmt.Sscanf(maxResultsStr, "%d", &val); err == nil {
			maxResults = val
		}
	}

	if maxResults <= 0 {
		maxResults = 5
	}

	a := EmailToolArgs{
		Query:      query,
		MaxResults: maxResults,
	}

	// 1. 決定 gog 執行檔路徑
	binPath := os.Getenv("GOG_PATH")
	found := false

	// Check if configured path exists
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

		// 優先順序:
		// 1. 當前目錄下的 bin/gog.exe
		// 2. 使用者家目錄下的 go/bin/gog.exe (常見 Go 安裝位置)
		// 3. 系統 PATH

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

		for _, p := range possiblePaths {
			if _, err := os.Stat(p); err == nil {
				binPath = p
				found = true
				break
			}
		}

		if !found {
			// Fallback to PATH lookup
			if path, err := exec.LookPath(binName); err == nil {
				binPath = path
			} else {
				binPath = binName // Last resort
			}
		}
	}

	// 2. 組建指令
	// gog gmail search "query" --limit N
	// 若無 query 則預設搜尋 "is:inbox" (收件匣)
	var cmdArgs []string
	cmdArgs = append(cmdArgs, "gmail", "search")

	if a.Query != "" {
		cmdArgs = append(cmdArgs, a.Query)
	} else {
		// Default to inbox if no query provided
		cmdArgs = append(cmdArgs, "is:inbox")
	}

	cmdArgs = append(cmdArgs, "--limit", fmt.Sprintf("%d", a.MaxResults))

	cmd := exec.Command(binPath, cmdArgs...)
	cmd.Env = os.Environ() // Pass env vars for authentication if needed

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("執行 gog 失敗: %v\nOutput: %s", err, string(output))
	}

	res := string(output)
	if strings.TrimSpace(res) == "" {
		return "📭 找不到符合條件的郵件。", nil
	}

	return fmt.Sprintf("📧 **搜尋結果** (via gog):\n%s", res), nil
}
