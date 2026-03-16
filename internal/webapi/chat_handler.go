package webapi

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/asccclass/pcai/internal/agent"
	"github.com/asccclass/pcai/internal/core"
	"github.com/asccclass/pcai/internal/history"
	"github.com/asccclass/pcai/llms/ollama"
)

// ChatHandler handles the bot-to-bot chat API.
type ChatHandler struct {
	modelName    string
	systemPrompt string
	agents       map[string]*agent.Agent
	toolRegistry *core.Registry
	logger       *agent.SystemLogger
}

// NewChatHandler creates a chat handler with a shared tool registry.
func NewChatHandler(modelName, systemPrompt string, toolRegistry *core.Registry, logger *agent.SystemLogger) *ChatHandler {
	return &ChatHandler{
		modelName:    modelName,
		systemPrompt: systemPrompt,
		agents:       make(map[string]*agent.Agent),
		toolRegistry: toolRegistry,
		logger:       logger,
	}
}

// AddRoutes registers API routes.
func (h *ChatHandler) AddRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/chat", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.handleChat(w, r)
	})
}

func (h *ChatHandler) handleChat(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SenderID string `json:"sender_id"`
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

	sessionID := "api_" + req.SenderID
	sess := history.LoadSession(sessionID)

	if len(sess.Messages) == 0 {
		ragPrompt := history.GetRAGEnhancedPrompt()
		sess.Messages = append(sess.Messages, ollama.Message{Role: "system", Content: h.systemPrompt + ragPrompt})
	}

	myAgent, exists := h.agents[sessionID]
	if !exists {
		if h.toolRegistry == nil {
			http.Error(w, "tool registry is not configured", http.StatusInternalServerError)
			return
		}
		myAgent = agent.NewAgent(h.modelName, h.systemPrompt, sess, h.toolRegistry, h.logger)
		h.agents[sessionID] = myAgent
	} else {
		myAgent.Session = sess
	}

	fmt.Printf("\n[API] Received [%s] message: %s\n", req.SenderID, req.Message)

	reply, err := myAgent.Chat(req.Message, nil)

	history.SaveSession(sess)
	history.CheckAndSummarize(sess, h.modelName, h.systemPrompt)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"reply":   reply,
	})
}
