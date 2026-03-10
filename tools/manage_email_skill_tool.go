package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/ollama/ollama/api"
)

type ManageEmailSkillTool struct{}

type ManageEmailSkillArgs struct {
	Action    string `json:"action,omitempty"`
	Args      string `json:"args,omitempty"`
	Query     string `json:"query,omitempty"`
	Limit     string `json:"limit,omitempty"`
	MessageID string `json:"message_id,omitempty"`
	ThreadID  string `json:"thread_id,omitempty"`
}

func (t *ManageEmailSkillTool) Name() string { return "manage_email" }

func (t *ManageEmailSkillTool) IsSkill() bool { return true }

func (t *ManageEmailSkillTool) Definition() api.Tool {
	var props api.ToolPropertiesMap
	js := `{
        "action": {
            "type": "string",
            "description": "Gmail action, such as search, get, or thread get."
        },
        "args": {
            "type": "string",
            "description": "Raw argument string passed directly to gog gmail."
        },
        "query": {
            "type": "string",
            "description": "Gmail search query, for example is:unread."
        },
        "limit": {
            "type": "string",
            "description": "Maximum number of search results, such as 5 or 10."
        },
        "message_id": {
            "type": "string",
            "description": "Message ID used when action is get."
        },
        "thread_id": {
            "type": "string",
            "description": "Thread ID used when action is thread get."
        }
    }`
	_ = json.Unmarshal([]byte(js), &props)

	return api.Tool{
		Type: "function",
		Function: api.ToolFunction{
			Name:        "manage_email",
			Description: "Manage Gmail reads through a stable skill-compatible interface.",
			Parameters: api.ToolFunctionParameters{
				Type:       "object",
				Properties: &props,
			},
		},
	}
}

func (t *ManageEmailSkillTool) Run(args string) (string, error) {
	var parsed ManageEmailSkillArgs
	if args != "" {
		if err := json.Unmarshal([]byte(args), &parsed); err != nil {
			return "", fmt.Errorf("解析參數失敗: %v", err)
		}
	}

	action, rawArgs, err := normalizeManageEmailSkillArgs(parsed)
	if err != nil {
		return "", err
	}

	binPath := resolveGogPath()
	finalCmd := fmt.Sprintf("%s gmail %s %s", binPath, action, rawArgs)

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

	res := strings.TrimSpace(string(output))
	if res == "" {
		return "沒有讀到任何郵件。", nil
	}
	return res, nil
}

func normalizeManageEmailSkillArgs(parsed ManageEmailSkillArgs) (string, string, error) {
	action := strings.TrimSpace(parsed.Action)
	rawArgs := strings.TrimSpace(parsed.Args)
	query := strings.TrimSpace(parsed.Query)
	limit := strings.TrimSpace(parsed.Limit)
	messageID := strings.TrimSpace(parsed.MessageID)
	threadID := strings.TrimSpace(parsed.ThreadID)

	if action == "" {
		switch {
		case messageID != "":
			action = "get"
		case threadID != "":
			action = "thread get"
		default:
			action = "search"
		}
	}

	switch action {
	case "search":
		if rawArgs == "" {
			if query == "" {
				query = "is:unread"
			}
			if limit == "" {
				limit = "10"
			}
			if _, err := strconv.Atoi(limit); err != nil {
				return "", "", fmt.Errorf("limit 必須是數字，收到 %q", limit)
			}
			rawArgs = fmt.Sprintf("\"%s\" --max %s", query, limit)
		}
	case "get":
		if rawArgs == "" {
			if messageID == "" {
				return "", "", fmt.Errorf("action=get 時必須提供 message_id 或 args")
			}
			rawArgs = messageID
		}
	case "thread get":
		if rawArgs == "" {
			if threadID == "" {
				return "", "", fmt.Errorf("action=thread get 時必須提供 thread_id 或 args")
			}
			rawArgs = threadID
		}
	default:
		if rawArgs == "" && query != "" {
			rawArgs = fmt.Sprintf("\"%s\"", query)
		}
	}

	return action, rawArgs, nil
}

func resolveGogPath() string {
	if binPath := os.Getenv("GOG_PATH"); binPath != "" {
		if _, err := os.Stat(binPath); err == nil {
			return binPath
		}
	}

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
			return p
		}
	}

	if path, err := exec.LookPath(binName); err == nil {
		return path
	}
	return binName
}
