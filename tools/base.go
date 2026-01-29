package tools

/*
定義標準介面 (tools/base.go)
首先，定義一個所有工具都必須遵守的「合約」。
*/

import (
	"github.com/ollama/ollama/api"
)

// AgentTool 是所有工具都必須實作的介面
type AgentTool interface {
	// Name 回傳工具名稱 (例如 "list_files")
	Name() string

	// Definition 回傳給 Ollama 看的 JSON Schema
	Definition() api.Tool

	// Run 接收 JSON 格式的參數字串，並回傳執行結果
	Run(argsJSON string) (string, error)
}
