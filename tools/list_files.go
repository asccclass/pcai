package tools

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/ollama/ollama/api"
)

type ListFilesTool struct {
	Manager *FileSystemManager
}

func (t *ListFilesTool) Name() string { return "list_files" }

func (t *ListFilesTool) Definition() api.Tool {
	return api.Tool{
		Type: "function",
		Function: api.ToolFunction{
			Name:        "list_files",
			Description: "列出當前工作目錄下的檔案清單 (Sandbox Restricted)",
			Parameters: func() api.ToolFunctionParameters {
				var props api.ToolPropertiesMap
				js := `{
					"path": {
						"type": "string",
						"description": "相對路徑 (選填，預設為工作區根目錄)"
					}
				}`
				_ = json.Unmarshal([]byte(js), &props)
				return api.ToolFunctionParameters{
					Type:       "object",
					Properties: &props,
				}
			}(),
		},
	}
}

func (t *ListFilesTool) Run(argsJSON string) (string, error) {
	var args struct {
		Path string `json:"path"`
	}
	// 忽略錯誤，若無參數則預設為空字串
	_ = json.Unmarshal([]byte(argsJSON), &args)

	// 若未提供路徑，預設 '.'
	if args.Path == "" {
		args.Path = "."
	}

	// 安全檢查
	if t.Manager == nil {
		return "", os.ErrPermission // 或者是 "Sandbox not initialized"
	}
	safePath, err := t.Manager.validatePath(args.Path)
	if err != nil {
		return "", err
	}

	files, err := os.ReadDir(safePath)
	if err != nil {
		return "", err
	}
	var names []string
	for _, f := range files {
		suffix := ""
		if f.IsDir() {
			suffix = "/"
		}
		names = append(names, f.Name()+suffix)
	}
	
	if len(names) == 0 {
		return "目錄是空的", nil
	}
	return "檔案列表:\n" + strings.Join(names, "\n"), nil
}
