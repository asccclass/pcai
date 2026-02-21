package webapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/asccclass/pcai/internal/database"
	"github.com/asccclass/pcai/internal/memory"
)

// MemoryHandler 記憶管理 HTTP Handler
type MemoryHandler struct {
	toolkit *memory.ToolKit
	db      *database.DB
}

// NewMemoryHandler 建立新的記憶管理 Handler
func NewMemoryHandler(tk *memory.ToolKit, db *database.DB) *MemoryHandler {
	return &MemoryHandler{toolkit: tk, db: db}
}

// AddRoutes 註冊 API 路由
func (h *MemoryHandler) AddRoutes(mux *http.ServeMux) {
	// ==================== Long-Term Memory (RAG) ====================
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

	// ==================== Short-Term Memory (SQLite) ====================
	mux.HandleFunc("/api/short-memory", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			h.handleShortTermList(w, r)
		case http.MethodPost:
			h.handleShortTermCreate(w, r)
		case http.MethodDelete: // 針對帶 ID 的路由
			http.Error(w, "Method not allowed, use /api/short-memory/{id}", http.StatusMethodNotAllowed)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// 刪除特定 ID 的短期記憶 (使用類似 /api/short-memory/?id=123 或路徑解析)
	// 因為標準 ServeMux (Go 1.22 以下) 不支援 /api/short-memory/{id} 語法，我們自建解析或用 query string
	mux.HandleFunc("/api/short-memory/delete", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.handleShortTermDelete(w, r)
	})

	mux.HandleFunc("/api/short-memory/search", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.handleShortTermSearch(w, r)
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

// ==================== Short-Term Memory Handlers ====================

func (h *MemoryHandler) handleShortTermList(w http.ResponseWriter, r *http.Request) {
	if h.db == nil {
		http.Error(w, "Database not configured", http.StatusInternalServerError)
		return
	}

	limit := 100 // 預設 100 筆
	ctx := context.Background()
	entries, err := h.db.GetRecentShortTermMemory(ctx, limit)
	if err != nil {
		http.Error(w, "Failed to get short-term memory", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"entries": entries,
		"count":   len(entries),
	})
}

func (h *MemoryHandler) handleShortTermSearch(w http.ResponseWriter, r *http.Request) {
	if h.db == nil {
		http.Error(w, "Database not configured", http.StatusInternalServerError)
		return
	}

	query := r.URL.Query().Get("q")
	if query == "" {
		http.Error(w, "missing query parameter 'q'", http.StatusBadRequest)
		return
	}

	ctx := context.Background()
	entries, err := h.db.SearchShortTermMemory(ctx, query, 50)
	if err != nil {
		http.Error(w, "Failed to search short-term memory", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"entries": entries,
		"count":   len(entries),
	})
}

func (h *MemoryHandler) handleShortTermCreate(w http.ResponseWriter, r *http.Request) {
	if h.db == nil {
		http.Error(w, "Database not configured", http.StatusInternalServerError)
		return
	}

	var req struct {
		Source  string `json:"source"`
		Content string `json:"content"`
		TTLDays int    `json:"ttl_days"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Content == "" {
		http.Error(w, "content is required", http.StatusBadRequest)
		return
	}

	if req.Source == "" {
		req.Source = "webui"
	}
	if req.TTLDays <= 0 {
		req.TTLDays = 7 // 預設 7 天
	}

	ctx := context.Background()
	if err := h.db.AddShortTermMemory(ctx, req.Source, req.Content, req.TTLDays); err != nil {
		http.Error(w, "Failed to add short-term memory", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "短期記憶已寫入",
	})
}

func (h *MemoryHandler) handleShortTermDelete(w http.ResponseWriter, r *http.Request) {
	if h.db == nil {
		http.Error(w, "Database not configured", http.StatusInternalServerError)
		return
	}

	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		http.Error(w, "missing query parameter 'id'", http.StatusBadRequest)
		return
	}

	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	ctx := context.Background()
	if err := h.db.DeleteShortTermMemory(ctx, id); err != nil {
		http.Error(w, "Failed to delete short-term memory", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "短期記憶已刪除",
	})
}
