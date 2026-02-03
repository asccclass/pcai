// internal/heartbeat/processor.go
package heartbeat

import (
	"fmt"

	"github.com/asccclass/pcai/internal/memory"
	"github.com/asccclass/pcai/llms/ollama"
	"github.com/asccclass/pcai/tools"
)

type HeartbeatProcessor struct {
	llmClient *ollama.Client // 假設你使用 Ollama
	tools     []tools.AgentTool
	memory    *memory.Manager
}

func (h *HeartbeatProcessor) Pulse() {
	// 1. 收集環境數據 (這裡可以整合你的 Gmail 或 Signal 讀取工具)
	envSnap := h.collectEnvironmentSnapshot()

	// 2. 構建 Prompt
	prompt := fmt.Sprintf(`
        你現在是 PCAI 的核心大腦。
        目前環境狀態: %s
        請分析是否需要主動提醒用戶或處理任務。
        若不需要，回傳 "IDLE"。
        若需要，請回傳對應的工具呼叫。`, envSnap)

	// 3. 呼叫 Ollama 進行思考
	resp, _ := h.llmClient.Generate(prompt)

	if resp != "IDLE" {
		// 執行對應的工具邏輯
		h.executeAction(resp)
	}
}
