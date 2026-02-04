// 一個具備「自我診斷」能力的架構
package advisor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ollama/ollama/api"
)

// Classification 定義 AI 回傳的架構建議格式
type Classification struct {
	Category    string   `json:"category"`    // "Skill" 或 "Tool"
	Reasoning   string   `json:"reasoning"`   // 判定理由
	Persistence bool     `json:"persistence"` // 是否需要資料庫
	Components  []string `json:"components"`  // 建議開發的組件
}

// Analyze 透過 LLM 進行架構分析
func Analyze(ctx context.Context, client *api.Client, modelName, desc string) (*Classification, error) {
	prompt := fmt.Sprintf(`你是 PCAI 架構專家。請分析需求並判斷應實作為 Skill 還是 Tool。
標準：
* Tool (工具)：底層執行單元
  - 定義：Tool 是最基礎的「執行能力」，通常是對應到 AI 模型（如 Claude 的 Tool Use 或 OpenAI 的 Function Calling）可以直接調用的具體函式。
  - 性質：它定義了輸入參數（JSON Schema）、執行邏輯和回傳格式。
  - 範例：browser.screenshot（截圖）、system.run（執行指令）、gmail.send（寄信）。
  - 位置：通常實作在 packages/ 或核心代碼中，定義了 AI 如何與系統硬體、網路環境或特定 API 進行最直接的互動。
* Skill (技能)：高層封裝與功能組合
  - 定義：Skill 是一組 Tool 的集合 或 特定場景的能力封裝。它更像是一個「外掛模組」或「功能包」。
  - 性質：
    - 模組化：OpenClaw 有一個專門的 skills/ 目錄，允許使用者以插件的形式安裝、開啟或關閉特定功能（例如：GitHub 技能、搜尋技能）。
    - 權限與配置：Skill 通常包含了自己的設定（Config）、安裝引導（Onboarding）以及多個相關的 Tools。
    - 自主性：Skill 有時會包含背景任務（如 Cron Job）或特定的系統提示詞（System Prompts），讓 AI 知道在什麼時候該使用這一組工具。
    - 範例：如果你安裝了「GitHub Skill」，它會為 AI 增加一整套工具（如讀取 Repo、建立 Issue、提交代碼）。

「Tool」是 AI 的手和腳（例如：拿筆寫字、開門）；而「Skill」則是 AI 學會的一項專業（例如：寫作、當個管家），這項專業裡面包含了使用多種工具的能力。
在你開發時，如果你想增加一個基礎的 API 呼叫，你會去定義一個 Tool；如果你想為 PCAI 增加一個完整的新功能區塊（包含多種功能與獨立設定），你應該開發一個 Skill。

    需求描述: %s
    請回傳 JSON 格式。`, desc)

	req := &api.GenerateRequest{
		Model:  modelName,
		Prompt: prompt,
		Stream: new(bool), // false
		Format: json.RawMessage(`"json"`),
	}

	var responseText string
	err := client.Generate(ctx, req, func(resp api.GenerateResponse) error {
		responseText = resp.Response
		return nil
	})
	if err != nil {
		return nil, err
	}

	var res Classification
	// 嘗試解析 JSON (Ollama 有時會回傳 markdown code block)
	cleaned := strings.TrimSpace(responseText)
	cleaned = strings.TrimPrefix(cleaned, "```json")
	cleaned = strings.TrimSuffix(cleaned, "```")

	if err := json.Unmarshal([]byte(cleaned), &res); err != nil {
		return nil, fmt.Errorf("解析失敗: %v, 原始回應: %s", err, responseText)
	}
	return &res, nil
}
