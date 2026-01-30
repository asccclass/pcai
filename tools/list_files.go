package tools

import (
	"os"
	"strings"

	"github.com/ollama/ollama/api"
)

type ListFilesTool struct{}

func (t *ListFilesTool) Name() string { return "list_files" }

func (t *ListFilesTool) Definition() api.Tool {
	return api.Tool{
		Type: "function",
		Function: api.ToolFunction{
			Name:        "list_files",
			Description: "列出當前目錄下的檔案清單",
		},
	}
}

func (t *ListFilesTool) Run(argsJSON string) (string, error) {
	files, err := os.ReadDir(".")
	if err != nil {
		return "", err
	}
	var names []string
	for _, f := range files {
		names = append(names, f.Name())
	}
	return "檔案列表: " + strings.Join(names, ", "), nil
}
