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

// EmailDraftTool 讓 LLM 可以主動建立信件草稿 (透過 gog CLI)
type EmailDraftTool struct{}

type EmailDraftToolArgs struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

func (t *EmailDraftTool) Name() string { return "email_draft_create" }

func (t *EmailDraftTool) IsSkill() bool {
	return false
}

func (t *EmailDraftTool) Definition() api.Tool {
	return api.Tool{
		Type: "function",
		Function: api.ToolFunction{
			Name:        "email_draft_create",
			Description: "使用 gog 工具建立 Gmail 郵件草稿。當使用者要求「草擬信件」、「寫一封信」時使用。",
			Parameters: func() api.ToolFunctionParameters {
				var props api.ToolPropertiesMap
				js := `{
					"to": {
						"type": "string",
						"description": "收件者 Email 地址"
					},
					"subject": {
						"type": "string",
						"description": "信件主旨"
					},
					"body": {
						"type": "string",
						"description": "信件內容"
					}
				}`
				_ = json.Unmarshal([]byte(js), &props)

				return api.ToolFunctionParameters{
					Type:       "object",
					Properties: &props,
					Required:   []string{"to", "subject", "body"},
				}
			}(),
		},
	}
}

func (t *EmailDraftTool) Run(args string) (string, error) {
	var parsedArgs EmailDraftToolArgs
	if err := json.Unmarshal([]byte(args), &parsedArgs); err != nil {
		return "", fmt.Errorf("解析參數失敗: %v", err)
	}

	// 1. 決定 gog 執行檔路徑 (與 email_tool.go 邏輯相同)
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

	// 2. 組建指令
	cmdArgs := []string{
		"gmail", "drafts", "create",
		"--to", parsedArgs.To,
		"--subject", parsedArgs.Subject,
		"--body", parsedArgs.Body,
	}

	cmd := exec.Command(binPath, cmdArgs...)
	cmd.Env = os.Environ()

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("建立草稿失敗: %v\nOutput: %s", err, string(output))
	}

	return fmt.Sprintf("✅ 成功建立郵件草稿：\n收件者: %s\n主旨: %s\n(可至 Gmail 草稿匣查看)", parsedArgs.To, parsedArgs.Subject), nil
}
