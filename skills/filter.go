package skills

import (
	"context"
	"fmt"

	"github.com/asccclass/pcai/internal/database"
)

// FilterSkill 定義了過濾規則操作的工具
type FilterSkill struct {
	db *database.DB
}

func NewFilterSkill(db *database.DB) *FilterSkill {
	return &FilterSkill{db: db}
}

// Params 定義了 AI 需要從對話中提取的參數
type FilterParams struct {
	Pattern     string `json:"pattern"`     // 例如: +886900%
	Action      string `json:"action"`      // URGENT, NORMAL, IGNORE
	Description string `json:"description"` // 規則原因
}

// Execute 執行寫入資料庫的動作
func (s *FilterSkill) Execute(ctx context.Context, p FilterParams) (string, error) {
	query := `INSERT INTO filters (pattern, action, description) VALUES (?, ?, ?)`

	_, err := s.db.ExecContext(ctx, query, p.Pattern, p.Action, p.Description)
	if err != nil {
		return "", fmt.Errorf("無法儲存過濾規則: %w", err)
	}

	successMsg := fmt.Sprintf("✅ 已更新過濾規則：匹配 [%s] 的訊息將被標記為 [%s]。", p.Pattern, p.Action)
	return successMsg, nil
}
