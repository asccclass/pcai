package webapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/asccclass/pcai/internal/memory"
)

// MemoryHandler 記憶管理 HTTP Handler
type MemoryHandler struct {
	toolkit *memory.ToolKit
}

// NewMemoryHandler 建立新的記憶管理 Handler
func NewMemoryHandler(tk *memory.ToolKit) *MemoryHandler {
	return &MemoryHandler{toolkit: tk}
}

// AddRoutes 註冊 API 路由
func (h *MemoryHandler) AddRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/memory", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			h.handleList(w, r)
		case http.MethodPost:
			h.handleCreate(w, r)
		case http.MethodDelete:
			h.handleDelete(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/memory/search", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.handleSearch(w, r)
	})
}

// handleList 列出記憶（讀取 MEMORY.md）
func (h *MemoryHandler) handleList(w http.ResponseWriter, r *http.Request) {
	content, err := h.toolkit.MemoryGet("MEMORY.md", 0, 0)
	if err != nil {
		content = "尚無記憶檔案。"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"content": content,
		"chunks":  h.toolkit.ChunkCount(),
	})
}

// handleSearch 搜尋記憶
func (h *MemoryHandler) handleSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		http.Error(w, "missing query parameter 'q'", http.StatusBadRequest)
		return
	}

	ctx := context.Background()
	resp, err := h.toolkit.MemorySearch(ctx, query)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"results":  resp.Results,
		"backend":  resp.Backend,
		"provider": resp.Provider,
	})
}

// handleCreate 建立新記憶
func (h *MemoryHandler) handleCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Content  string `json:"content"`
		Category string `json:"category"`
		Mode     string `json:"mode"` // "daily" | "long_term"
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Content == "" {
		http.Error(w, "content is required", http.StatusBadRequest)
		return
	}

	var err error
	mode := req.Mode
	if mode == "" {
		mode = "long_term"
	}

	switch mode {
	case "daily":
		err = h.toolkit.WriteToday(req.Content)
	case "long_term":
		cat := req.Category
		if cat == "" {
			cat = "general"
		}
		err = h.toolkit.WriteLongTerm(cat, req.Content)
	default:
		http.Error(w, fmt.Sprintf("unsupported mode: %s", mode), http.StatusBadRequest)
		return
	}

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("記憶已寫入 (%s)", mode),
	})
}

// handleDelete 刪除記憶（尚未完全實作）
func (h *MemoryHandler) handleDelete(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": false,
		"message": "刪除功能目前僅支援透過 memory_forget 工具操作",
	})
}
