package gateway

import (
	"fmt"
	"sync"

	"github.com/asccclass/pcai/internal/agent"
	"github.com/asccclass/pcai/internal/channel"
	"github.com/asccclass/pcai/internal/core"
	"github.com/asccclass/pcai/internal/history"
)

// AgentAdapter 負責管理多個 Telegram 使用者的 Agent 實例
type AgentAdapter struct {
	agents       map[string]*agent.Agent
	registry     *core.Registry
	modelName    string
	systemPrompt string
	mu           sync.Mutex
}

// NewAgentAdapter 建立新的 Adapter
func NewAgentAdapter(registry *core.Registry, modelName, systemPrompt string) *AgentAdapter {
	return &AgentAdapter{
		agents:       make(map[string]*agent.Agent),
		registry:     registry,
		modelName:    modelName,
		systemPrompt: systemPrompt,
	}
}

// Process 實作 Processor 介面
func (a *AgentAdapter) Process(env channel.Envelope) string {
	// 產生 Session ID (加上前綴以區隔)
	sessionID := fmt.Sprintf("telegram_%s", env.SenderID)

	// 取得或建立 Agent
	myAgent := a.getOrCreateAgent(sessionID)

	// 呼叫 Agent 進行對話
	// 注意：這裡暫時不使用 stream callback (傳 nil)，因為 Telegram API 通常是一次性回覆
	// 若要支援打字中或串流更新，需要更複雜的 channel 整合
	response, err := myAgent.Chat(env.Content, nil)
	if err != nil {
		return fmt.Sprintf("⚠️ 系統錯誤: %v", err)
	}

	// 儲存 Session (Agent 內部已自動維護 Message History，但仍需觸發存檔)
	// 在 Agent.Chat 內部其實沒有顯式呼叫 SaveSession，CLI 是在外層呼叫的
	// 所以這裡我們需要手動存檔
	history.SaveSession(myAgent.Session)

	// 自動執行 RAG 歸納檢查
	// 為了避免每次對話都卡住太久，這部分可以考慮非同步執行，
	// 但為了確保資料一致性，這裡先同步執行
	history.CheckAndSummarize(myAgent.Session, a.modelName, a.systemPrompt)

	return response
}

func (a *AgentAdapter) getOrCreateAgent(sessionID string) *agent.Agent {
	a.mu.Lock()
	defer a.mu.Unlock()

	if ag, exists := a.agents[sessionID]; exists {
		return ag
	}

	// 載入 Session
	session := history.LoadSession(sessionID)

	// 如果是新 Session (只有 ID)，補上 System Prompt
	if len(session.Messages) == 0 {
		// 這裡可以加入 RAG Prompt
		// 為了簡化，暫時只加 System Prompt
		// 若要完整複刻 CLI 行為，應呼叫 history.GetRAGEnhancedPrompt()
		// 但該函數目前未導出或需確認位置
		// 假設我們簡單處理:
		// session.Messages = append(session.Messages, ollama.Message{Role: "system", Content: a.systemPrompt})
	}
	// [Note] NewAgent 內部不會自動加 System Prompt 到 Messages，它只是存起來
	// 真正決定是否加 System Prompt 是在 Session 初始化階段

	// 建立 Agent
	newAgent := agent.NewAgent(a.modelName, a.systemPrompt, session, a.registry)

	// 設定 Callbacks (雖然可以留空，但為了 debug 方便，可以印 log)
	/*
		newAgent.OnToolCall = func(name, args string) {
			log.Printf("[%s] Tool Call: %s(%s)", sessionID, name, args)
		}
	*/

	a.agents[sessionID] = newAgent
	return newAgent
}
