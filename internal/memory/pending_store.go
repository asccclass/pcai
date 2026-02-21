package memory

import (
	"fmt"
	"sync"
	"time"
)

// PendingEntry 暫存待確認的記憶
type PendingEntry struct {
	ID        string    `json:"id"`
	Content   string    `json:"content"`
	Category  string    `json:"category"`
	Mode      string    `json:"mode"`
	CreatedAt time.Time `json:"created_at"`
}

// PendingStore 管理待確認的記憶寫入
type PendingStore struct {
	mu      sync.Mutex
	entries map[string]*PendingEntry
	ttl     time.Duration // 過期時間
}

// NewPendingStore 建立新的 PendingStore
func NewPendingStore(ttl time.Duration) *PendingStore {
	ps := &PendingStore{
		entries: make(map[string]*PendingEntry),
		ttl:     ttl,
	}

	// 啟動背景清理
	go ps.cleanupLoop()

	return ps
}

// Add 暫存一筆待確認的記憶，回傳 pending ID
func (ps *PendingStore) Add(content string, category string, mode string) string {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	id := fmt.Sprintf("pending_%d", time.Now().UnixNano())
	ps.entries[id] = &PendingEntry{
		ID:        id,
		Content:   content,
		Category:  category,
		Mode:      mode,
		CreatedAt: time.Now(),
	}

	return id
}

// Confirm 取出並確認一筆記憶，回傳內容與標籤
func (ps *PendingStore) Confirm(id string) (*PendingEntry, error) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	entry, exists := ps.entries[id]
	if !exists {
		return nil, fmt.Errorf("登記 ID %s 不存在或已過期", id)
	}

	delete(ps.entries, id)
	return entry, nil
}

// ConfirmAll 確認所有待確認記憶
func (ps *PendingStore) ConfirmAll() []*PendingEntry {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	var result []*PendingEntry
	for _, entry := range ps.entries {
		result = append(result, entry)
	}
	ps.entries = make(map[string]*PendingEntry)
	return result
}

// Reject 拒絕（丟棄）一筆記憶
func (ps *PendingStore) Reject(id string) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if _, exists := ps.entries[id]; !exists {
		return fmt.Errorf("登記 ID %s 不存在或已過期", id)
	}

	delete(ps.entries, id)
	return nil
}

// RejectAll 拒絕所有待確認記憶
func (ps *PendingStore) RejectAll() int {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	count := len(ps.entries)
	ps.entries = make(map[string]*PendingEntry)
	return count
}

// List 列出所有待確認項目
func (ps *PendingStore) List() []*PendingEntry {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	var result []*PendingEntry
	for _, entry := range ps.entries {
		result = append(result, entry)
	}
	return result
}

// Count 回傳待確認數量
func (ps *PendingStore) Count() int {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	return len(ps.entries)
}

// cleanupLoop 背景清理過期項目
func (ps *PendingStore) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		ps.mu.Lock()
		now := time.Now()
		for id, entry := range ps.entries {
			if now.Sub(entry.CreatedAt) > ps.ttl {
				delete(ps.entries, id)
			}
		}
		ps.mu.Unlock()
	}
}
