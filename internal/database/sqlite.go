package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

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
	// 創建 cron_jobs 表格
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
	);
	CREATE TABLE IF NOT EXISTS cron_jobs (
		name TEXT PRIMARY KEY,
		cron_spec TEXT NOT NULL,
		task_type TEXT NOT NULL,
		description TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS short_term_memory (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		source TEXT NOT NULL,
		content TEXT NOT NULL,
		expires_at DATETIME NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`

	_, err := db.Exec(query)
	if err != nil {
		return fmt.Errorf("failed to run migration: %w", err)
	}
	fmt.Println("✅ [Database] Tables initialized successfully.")
	return nil
}

// CronJobModel 定義資料庫中的 Cron Job 結構
type CronJobModel struct {
	Name        string `json:"name"`
	CronSpec    string `json:"cron_spec"`
	TaskType    string `json:"task_type"`
	Description string `json:"description"`
	CreatedAt   string `json:"created_at"`
}

// AddCronJob 新增或更新 Cron Job
func (db *DB) AddCronJob(ctx context.Context, name, spec, taskType, desc string) error {
	query := `INSERT INTO cron_jobs (name, cron_spec, task_type, description) VALUES (?, ?, ?, ?)
			  ON CONFLICT(name) DO UPDATE SET cron_spec=excluded.cron_spec, task_type=excluded.task_type, description=excluded.description`
	_, err := db.ExecContext(ctx, query, name, spec, taskType, desc)
	return err
}

// GetCronJobs 取得所有 Cron Jobs
func (db *DB) GetCronJobs(ctx context.Context) ([]CronJobModel, error) {
	rows, err := db.QueryContext(ctx, "SELECT name, cron_spec, task_type, description, created_at FROM cron_jobs")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []CronJobModel
	for rows.Next() {
		var j CronJobModel
		if err := rows.Scan(&j.Name, &j.CronSpec, &j.TaskType, &j.Description, &j.CreatedAt); err == nil {
			jobs = append(jobs, j)
		}
	}
	return jobs, nil
}

// RemoveCronJob 移除 Cron Job
func (db *DB) RemoveCronJob(ctx context.Context, name string) error {
	_, err := db.ExecContext(ctx, "DELETE FROM cron_jobs WHERE name = ?", name)
	return err
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

// ==================== Short-Term Memory ====================

// ShortTermMemoryEntry 短期記憶條目
type ShortTermMemoryEntry struct {
	ID        int    `json:"id"`
	Source    string `json:"source"`
	Content   string `json:"content"`
	ExpiresAt string `json:"expires_at"`
	CreatedAt string `json:"created_at"`
}

// AddShortTermMemory 新增短期記憶
func (db *DB) AddShortTermMemory(ctx context.Context, source, content string, ttlDays int) error {
	expiresAt := time.Now().AddDate(0, 0, ttlDays).Format("2006-01-02 15:04:05")
	query := `INSERT INTO short_term_memory (source, content, expires_at) VALUES (?, ?, ?)`
	_, err := db.ExecContext(ctx, query, source, content, expiresAt)
	return err
}

// GetRecentShortTermMemory 取得最近的未過期短期記憶
func (db *DB) GetRecentShortTermMemory(ctx context.Context, limit int) ([]ShortTermMemoryEntry, error) {
	query := `SELECT id, source, content, expires_at, created_at 
			  FROM short_term_memory 
			  WHERE expires_at > datetime('now') 
			  ORDER BY created_at DESC LIMIT ?`
	rows, err := db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []ShortTermMemoryEntry
	for rows.Next() {
		var e ShortTermMemoryEntry
		if err := rows.Scan(&e.ID, &e.Source, &e.Content, &e.ExpiresAt, &e.CreatedAt); err == nil {
			entries = append(entries, e)
		}
	}
	return entries, nil
}

// GetShortTermMemoryBySource 按來源查詢短期記憶
func (db *DB) GetShortTermMemoryBySource(ctx context.Context, source string, limit int) ([]ShortTermMemoryEntry, error) {
	query := `SELECT id, source, content, expires_at, created_at 
			  FROM short_term_memory 
			  WHERE source = ? AND expires_at > datetime('now') 
			  ORDER BY created_at DESC LIMIT ?`
	rows, err := db.QueryContext(ctx, query, source, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []ShortTermMemoryEntry
	for rows.Next() {
		var e ShortTermMemoryEntry
		if err := rows.Scan(&e.ID, &e.Source, &e.Content, &e.ExpiresAt, &e.CreatedAt); err == nil {
			entries = append(entries, e)
		}
	}
	return entries, nil
}

// CleanExpiredMemory 刪除已過期的短期記憶
func (db *DB) CleanExpiredMemory(ctx context.Context) (int64, error) {
	result, err := db.ExecContext(ctx, "DELETE FROM short_term_memory WHERE expires_at <= datetime('now')")
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// SearchShortTermMemory 關鍵字搜尋短期記憶
func (db *DB) SearchShortTermMemory(ctx context.Context, keyword string, limit int) ([]ShortTermMemoryEntry, error) {
	query := `SELECT id, source, content, expires_at, created_at 
			  FROM short_term_memory 
			  WHERE content LIKE ? AND expires_at > datetime('now') 
			  ORDER BY created_at DESC LIMIT ?`
	rows, err := db.QueryContext(ctx, query, "%"+keyword+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []ShortTermMemoryEntry
	for rows.Next() {
		var e ShortTermMemoryEntry
		if err := rows.Scan(&e.ID, &e.Source, &e.Content, &e.ExpiresAt, &e.CreatedAt); err == nil {
			entries = append(entries, e)
		}
	}
	return entries, nil
}

// GetLastHeartbeatAction 取得最後一次執行特定動作的時間
func (db *DB) GetLastHeartbeatAction(ctx context.Context, actionPrefix string) (time.Time, error) {
	// actionPrefix 例如 "ACTION: SELF_TEST"
	query := `SELECT created_at FROM heartbeat_logs WHERE decision LIKE ? ORDER BY created_at DESC LIMIT 1`
	var createdAtStr string
	err := db.QueryRowContext(ctx, query, actionPrefix+"%").Scan(&createdAtStr)
	if err != nil {
		if err == sql.ErrNoRows {
			return time.Time{}, nil // 從未執行過 (回傳 Zero Time)
		}
		return time.Time{}, err
	}

	// SQLite 預設時間格式 "2006-01-02 15:04:05" (UTC)
	// 嘗試解析 (注意：這裡假設 DB 存的是 UTC，若有時區需調整)
	t, err := time.Parse("2006-01-02 15:04:05", createdAtStr)
	if err != nil {
		// 容錯：嘗試 RFC3339
		t, err = time.Parse(time.RFC3339, createdAtStr)
	}
	return t, err
}
