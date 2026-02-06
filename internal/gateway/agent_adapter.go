package gateway

import (
	"fmt"
	"sync"

	"github.com/asccclass/pcai/internal/agent"
	"github.com/asccclass/pcai/internal/channel"
	"github.com/asccclass/pcai/internal/core"
	"github.com/asccclass/pcai/internal/history"
	"github.com/asccclass/pcai/llms/ollama"
)

// AgentAdapter 負責管理多個 Telegram 使用者的 Agent 實例
// AgentAdapter 負責管理多個 Telegram 使用者的 Agent 實例
type AgentAdapter struct {
	agents       map[string]*agent.Agent
	registry     *core.Registry
	modelName    string
	systemPrompt string
	mu           sync.Mutex
	router       *Router // [NEW] 引入路由
	debug        bool    // [NEW] Debug 旗標
}

// NewAgentAdapter 建立新的 Adapter
func NewAgentAdapter(registry *core.Registry, modelName, systemPrompt string, debug bool) *AgentAdapter {
	return &AgentAdapter{
		agents:       make(map[string]*agent.Agent),
		registry:     registry,
		modelName:    modelName,
		systemPrompt: systemPrompt,
		router:       NewRouter(modelName), // 初始化路由，並以傳入的 model 作為預設
		debug:        debug,
	}
}

// Process 實作 Processor 介面
func (a *AgentAdapter) Process(env channel.Envelope) string {
	// 產生 Session ID (加上前綴以區隔)
	sessionID := fmt.Sprintf("telegram_%s", env.SenderID)

	// 取得或建立 Agent
	myAgent := a.getOrCreateAgent(sessionID)

	// [NEW] 動態路由決策
	// 在每次對話前，先問 Router 這次該用誰
	routeResult, err := a.router.Route(env.Content)
	if err == nil {
		// 動態切換 Agent 的腦袋
		// 這裡假設 Agent 是同一個實例，但在這一輪對話中臨時切換配置
		// 注意：如果底層 history 是共用的，這樣做沒問題。
		myAgent.SetModelConfig(routeResult.ModelName, routeResult.Provider)
	}

	// 呼叫 Agent 進行對話
	// 呼叫 Agent 進行對話
	if a.debug {
		fmt.Printf("[Telegram DEBUG] (%s) Sending prompt to Agent: %s\n", sessionID, env.Content)
	}

	// 注意：這裡暫時不使用 stream callback (傳 nil)，因為 Telegram API 通常是一次性回覆
	// 若要支援打字中或串流更新，需要更複雜的 channel 整合
	response, err := myAgent.Chat(env.Content, nil)
	if err != nil {
		fmt.Printf("[Telegram DEBUG] (%s) Agent Chat Error: %v\n", sessionID, err)
		return fmt.Sprintf("⚠️ 系統錯誤: %v", err)
	}
	if a.debug {
		fmt.Printf("[Telegram DEBUG] (%s) Agent Response Length: %d\n", sessionID, len(response))
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
		// 這裡必須注入 System Prompt，否則 Telegram 用戶無法得知工具定義與使用規範
		session.Messages = append(session.Messages, ollama.Message{Role: "system", Content: a.systemPrompt})
	}
	// [Note] NewAgent 內部不會自動加 System Prompt 到 Messages，它只是存起來
	// 真正決定是否加 System Prompt 是在 Session 初始化階段

	// 建立 Agent
	newAgent := agent.NewAgent(a.modelName, a.systemPrompt, session, a.registry)

	// 設定 Callbacks (為了 debug)
	// 設定 Callbacks (為了 debug)
	if a.debug {
		newAgent.OnToolCall = func(name, args string) {
			fmt.Printf("[Telegram DEBUG] (%s) Tool Call: %s args: %s\n", sessionID, name, args)
		}
		newAgent.OnModelMessageComplete = func(content string) {
			fmt.Printf("[Telegram DEBUG] (%s) AI Message Complete: %s...\n", sessionID, content[:min(len(content), 50)])
		}
	}

	a.agents[sessionID] = newAgent
	return newAgent
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
