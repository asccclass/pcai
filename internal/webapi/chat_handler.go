package webapi

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/asccclass/pcai/internal/agent"
	"github.com/asccclass/pcai/internal/history"
	"github.com/asccclass/pcai/llms/ollama"
)

// ChatHandler 負責處理 Bot-to-Bot 的 API 溝通
type ChatHandler struct {
	modelName    string
	systemPrompt string
	registry     map[string]*agent.Agent // 暫存 Agent 實例
	logger       *agent.SystemLogger
}

// NewChatHandler 建立新的 Chat Handler
func NewChatHandler(modelName, systemPrompt string, logger *agent.SystemLogger) *ChatHandler {
	return &ChatHandler{
		modelName:    modelName,
		systemPrompt: systemPrompt,
		registry:     make(map[string]*agent.Agent),
		logger:       logger,
	}
}

// AddRoutes 註冊 API 路由
func (h *ChatHandler) AddRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/chat", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.handleChat(w, r)
	})
}

// handleChat 處理接收到的聊天訊息
func (h *ChatHandler) handleChat(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SenderID string `json:"sender_id"` // 識別來源 Bot 或 User
		Message  string `json:"message"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Message == "" {
		http.Error(w, "message is required", http.StatusBadRequest)
		return
	}

	if req.SenderID == "" {
		req.SenderID = "anonymous_bot"
	}

	// 1. 建立或載入專屬的 Session (以發送者 ID 為區分)
	sessionID := "api_" + req.SenderID
	sess := history.LoadSession(sessionID)

	// 確保新 Session 包含 System Prompt + RAG
	if len(sess.Messages) == 0 {
		ragPrompt := history.GetRAGEnhancedPrompt()
		sess.Messages = append(sess.Messages, ollama.Message{Role: "system", Content: h.systemPrompt + ragPrompt})
	}

	// 2. 取得或建立 Agent
	// 注意：為了簡化範例，這裡沒有傳入完整的 Tools Registry。
	// 如果需要讓這個 API-based Agent 也能呼叫本地工具，需要從外部依賴注入 tools.Registry。
	// 在第一階段實作中，我們先建立一個基礎的 Agent。
	myAgent, exists := h.registry[sessionID]
	if !exists {
		// [TODO] 這裡目前沒有 tools.Registry，如果需要工具能力，必須在 server.go 初始化並傳入
		myAgent = agent.NewAgent(h.modelName, h.systemPrompt, sess, nil, h.logger)
		h.registry[sessionID] = myAgent
	} else {
		// 更新 session 指標
		myAgent.Session = sess
	}

	// 顯示收到的訊息
	fmt.Printf("\n[API] 收到來自 [%s] 的訊息: %s\n", req.SenderID, req.Message)

	// 3. 交給 Agent 處理 (同步或非同步均可，這裡使用同步等待回應)
	// 如果需要非同步立刻回傳 200 OK，可以用 goroutine 跑 Chat，但目前的設計通常需要回傳 AI 的回應

	// 設定一個超時 Context 以防 LLM 卡住 (在這裡沒有直接傳入給 Chat，我們依賴下層本身的防護)
	// (或者如果 Agent.Chat 支援 Context 再調整，目前查閱 CLI 也是傳 nil)

	reply, err := myAgent.Chat(req.Message, nil)

	// 儲存對話
	history.SaveSession(sess)
	history.CheckAndSummarize(sess, h.modelName, h.systemPrompt)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	// 4. 回傳 AI 的回覆
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"reply":   reply,
	})
}
