package tools

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/ollama/ollama/api"
)

type FetchURLTool struct{}

func (t *FetchURLTool) Name() string { return "fetch_url" }

func (t *FetchURLTool) Definition() api.Tool {
	return api.Tool{
		Type: "function",
		Function: api.ToolFunction{
			Name:        "fetch_url",
			Description: "獲取指定網址的純文字內容。",
			Parameters: func() api.ToolFunctionParameters {
				var props api.ToolPropertiesMap
				js := `{
					"url": {
						"type": "string"
					}
				}`
				_ = json.Unmarshal([]byte(js), &props)

				return api.ToolFunctionParameters{
					Type:       "object",
					Properties: &props,
					Required:   []string{"url"},
				}
			}(),
		},
	}
}

func (t *FetchURLTool) Run(argsJSON string) (string, error) {
	var args struct {
		URL string `json:"url"`
	}
	json.Unmarshal([]byte(argsJSON), &args)

	resp, err := http.Get(args.URL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	// 注意：這裡直接回傳 HTML，AI 擅長從中擷取重點。
	// 若要更精準可加入 HTML to Text 的處理。
	content := string(body)
	if len(content) > 5000 {
		content = content[:5000] // 避免內容過長
	}
	return content, nil
}
