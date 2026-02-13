package webapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/asccclass/pcai/internal/memory"
)

// MemoryHandler 提供記憶管理 REST API
type MemoryHandler struct {
	Manager *memory.Manager
}

// NewMemoryHandler 建立新的 Handler
func NewMemoryHandler(m *memory.Manager) *MemoryHandler {
	return &MemoryHandler{Manager: m}
}

// --- 回應輔助 ---

func jsonResponse(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func jsonError(w http.ResponseWriter, status int, msg string) {
	jsonResponse(w, status, map[string]string{"error": msg})
}

// --- API 請求結構 ---

type createRequest struct {
	Content string   `json:"content"`
	Tags    []string `json:"tags"`
}

type updateRequest struct {
	Content string   `json:"content"`
	Tags    []string `json:"tags"`
}

// --- Route 註冊 ---

// AddRoutes 將 API 路由加入 ServeMux
func (h *MemoryHandler) AddRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/memory", h.handleList)
	mux.HandleFunc("POST /api/memory", h.handleCreate)
	mux.HandleFunc("PUT /api/memory/{id}", h.handleUpdate)
	mux.HandleFunc("DELETE /api/memory/{id}", h.handleDelete)
}

// --- Handlers ---

// handleList GET /api/memory
func (h *MemoryHandler) handleList(w http.ResponseWriter, r *http.Request) {
	entries := h.Manager.ListAll()

	// 為前端提供精簡版 (去掉 Vector 以減少傳輸量)
	type entryDTO struct {
		ID          string   `json:"id"`
		Content     string   `json:"content"`
		ContentHash string   `json:"content_hash"`
		Timestamp   string   `json:"timestamp"`
		Tags        []string `json:"tags"`
	}

	dtos := make([]entryDTO, 0, len(entries))
	for _, e := range entries {
		dtos = append(dtos, entryDTO{
			ID:          e.ID,
			Content:     e.Content,
			ContentHash: e.ContentHash,
			Timestamp:   e.Timestamp.Format("2006-01-02 15:04:05"),
			Tags:        e.Tags,
		})
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"count":   len(dtos),
		"entries": dtos,
	})
}

// handleCreate POST /api/memory
func (h *MemoryHandler) handleCreate(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "無效的 JSON 格式")
		return
	}

	if strings.TrimSpace(req.Content) == "" {
		jsonError(w, http.StatusBadRequest, "content 不可為空")
		return
	}

	if err := h.Manager.Add(req.Content, req.Tags); err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("新增失敗: %v", err))
		return
	}

	jsonResponse(w, http.StatusCreated, map[string]string{"message": "記憶已新增"})
}

// handleUpdate PUT /api/memory/{id}
func (h *MemoryHandler) handleUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		jsonError(w, http.StatusBadRequest, "缺少 ID")
		return
	}

	var req updateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "無效的 JSON 格式")
		return
	}

	if strings.TrimSpace(req.Content) == "" {
		jsonError(w, http.StatusBadRequest, "content 不可為空")
		return
	}

	if err := h.Manager.UpdateByID(id, req.Content, req.Tags); err != nil {
		jsonError(w, http.StatusNotFound, fmt.Sprintf("更新失敗: %v", err))
		return
	}

	jsonResponse(w, http.StatusOK, map[string]string{"message": "記憶已更新"})
}

// handleDelete DELETE /api/memory/{id}
func (h *MemoryHandler) handleDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		jsonError(w, http.StatusBadRequest, "缺少 ID")
		return
	}

	deleted, err := h.Manager.DeleteByID(id)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("刪除失敗: %v", err))
		return
	}

	if !deleted {
		jsonError(w, http.StatusNotFound, "找不到指定的記憶")
		return
	}

	jsonResponse(w, http.StatusOK, map[string]string{"message": "記憶已刪除"})
}
