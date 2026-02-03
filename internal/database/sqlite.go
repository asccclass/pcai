package database

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	_ "modernc.org/sqlite" // 無 CGO 版本驅動
)

type DB struct {
	*sql.DB
}

// NewSQLite 初始化並建立資料庫連線
func NewSQLite(path string) (*DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite: %w", err)
	}

	// 設定連線池，避免 SQLite 併發寫入衝突
	db.SetMaxOpenConns(1)

	instance := &DB{db}
	if err := instance.migrate(); err != nil {
		return nil, err
	}

	return instance, nil
}

// migrate 負責建立必要的表格
func (db *DB) migrate() error {
	query := `
	CREATE TABLE IF NOT EXISTS filters (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		pattern TEXT NOT NULL,       -- 例如 "+886900%"
		action TEXT NOT NULL,        -- URGENT, NORMAL, IGNORE
		description TEXT,            -- 用戶設定時的備註
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS heartbeat_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		snapshot TEXT,               -- 當時感知的環境快照
		decision TEXT,               -- AI 的最終決定 (IDLE/NOTIFY/LOGGED)
		reason TEXT,                 -- AI 給出的理由
		score INTEGER DEFAULT 100,   -- AI 的信心分數
		raw_response TEXT,           -- Ollama 的原始 JSON 回覆（備份用）
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`

	_, err := db.Exec(query)
	if err != nil {
		return fmt.Errorf("failed to run migration: %w", err)
	}
	log.Println("[Database] Tables initialized successfully.")
	return nil
}

// 新增一個方法來儲存日誌
func (db *DB) CreateHeartbeatLog(ctx context.Context, snapshot, decision, reason string, score int, raw string) error {
	query := `INSERT INTO heartbeat_logs (snapshot, decision, reason, score, raw_response) VALUES (?, ?, ?, ?, ?)`
	_, err := db.ExecContext(ctx, query, snapshot, decision, reason, score, raw)
	return err
}

// GetFilters 供 CollectEnv 呼叫使用
func (db *DB) GetFilters(ctx context.Context) ([]map[string]string, error) {
	rows, err := db.QueryContext(ctx, "SELECT pattern, action FROM filters")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]string
	for rows.Next() {
		var p, a string
		if err := rows.Scan(&p, &a); err == nil {
			results = append(results, map[string]string{"pattern": p, "action": a})
		}
	}
	return results, nil
}

// GetRecentLogs 獲取最近的 10 筆紀錄
func (db *DB) GetRecentLogs(ctx context.Context, limit int) ([]map[string]interface{}, error) {
	query := `SELECT id, decision, reason, score, snapshot, created_at FROM heartbeat_logs ORDER BY created_at DESC LIMIT ?`
	rows, err := db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []map[string]interface{}
	for rows.Next() {
		var id int
		var decision, reason, snapshot, createdAt string
		var score int
		if err := rows.Scan(&id, &decision, &reason, &score, &snapshot, &createdAt); err == nil {
			logs = append(logs, map[string]interface{}{
				"ID":        id,
				"Decision":  decision,
				"Reason":    reason,
				"Score":     score,
				"Snapshot":  snapshot,
				"CreatedAt": createdAt,
			})
		}
	}
	return logs, nil
}
