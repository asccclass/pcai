package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/asccclass/pcai/internal/database"
	"github.com/robfig/cron/v3"
)

// HeartbeatBrain 定義了 AI 如何感知環境並做出決策
type HeartbeatBrain interface {
	// CollectEnv 收集當前的環境快照（如未讀訊息、系統狀態、時間）
	CollectEnv(ctx context.Context) string
	// Think 根據快照做出判斷，回傳決策結果（IDLE, LOGGED, 或 Tool Call）
	Think(ctx context.Context, snapshot string) (string, error)
	// ExecuteDecision 執行 Think 產生的結果
	ExecuteDecision(ctx context.Context, decision string) error
	// 讓 Scheduler 知道大腦具備產生簡報的能力
	GenerateMorningBriefing(ctx context.Context) error
}

type ScheduledJob struct {
	EntryID     cron.EntryID `json:"entry_id"`
	TaskName    string       `json:"task_name"`
	CronSpec    string       `json:"cron_spec"`
	Description string       `json:"description"`
}

// TaskFunc 是原有的 Cron 任務函式類型
type TaskFunc func()

// ==========================================
// 1. 新增：即時任務介面 (用於一次性背景工作)
// ==========================================
type Job interface {
	Name() string
	Execute() error
}

type Manager struct {
	// --- 原有的 Cron 部分 ---
	cron     *cron.Cron
	registry map[string]TaskFunc     // 註冊可用的 Cron 任務
	jobs     map[string]ScheduledJob // 存放已排程的 Cron 任務
	mu       sync.RWMutex
	db       *database.DB // 資料庫連線

	// --- 新增的 Worker Pool 部分 ---
	bgJobQueue  chan Job       // 即時任務佇列
	workerCount int            // Worker 數量
	quit        chan struct{}  // 關閉訊號
	wg          sync.WaitGroup // 等待群組

	// Heartbeat 相關
	isThinking int32 // 防止重複執行
	brain      HeartbeatBrain

	// UI Callback
	OnCompletion func()
}

// runHeartbeat 是核心邏輯
func (m *Manager) runHeartbeat() {
	// 1. 併發防護：確保不會有多個心跳同時在「思考」，避免資源浪費或邏輯混亂
	if !atomic.CompareAndSwapInt32(&m.isThinking, 0, 1) {
		fmt.Println("[Scheduler] Heartbeat skipped: Brain is already busy thinking.")
		return
	}
	defer atomic.StoreInt32(&m.isThinking, 0)

	// 設定超時，避免 LLM 響應過久掛起系統
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	fmt.Printf("[Scheduler] Heartbeat started at %s\n", time.Now().Format("15:04:05"))

	// 確保無論如何結束都會嘗試恢復提示符 (但要小心不要與其他輸出衝突，這裡主要針對 Heartbeat 結束後的狀態)
	if m.OnCompletion != nil {
		defer m.OnCompletion()
	}

	// 2. 感知 (Sensing)S
	snapshot := m.brain.CollectEnv(ctx)
	if snapshot == "" {
		fmt.Println("[Scheduler] Heartbeat: Nothing to sense, skipping.")
		return
	}

	// 3. 思考 (Thinking)
	decision, err := m.brain.Think(ctx, snapshot)
	if err != nil {
		fmt.Printf("[Scheduler] Heartbeat Error during thinking: %v\n", err)
		return
	}

	// 4. 執行 (Execution)
	if decision == "STATUS: IDLE" {
		fmt.Println("[Scheduler] Heartbeat: AI decided to stay quiet.")
		return
	}

	err = m.brain.ExecuteDecision(ctx, decision)
	if err != nil {
		fmt.Printf("[Scheduler] Heartbeat Execution Error: %v\n", err)
	}
}

func NewManager(brain HeartbeatBrain, db *database.DB) *Manager {
	// 1. 初始化 Cron
	c := cron.New() // cron.WithSeconds()) // 建議維持秒級控制

	m := &Manager{
		cron:     c,
		registry: make(map[string]TaskFunc),
		jobs:     make(map[string]ScheduledJob),
		brain:    brain,
		db:       db,

		// 2. 初始化 Worker Pool
		bgJobQueue:  make(chan Job, 100), // 緩衝區 100
		workerCount: 3,                   // 預設 3 個 Worker
		quit:        make(chan struct{}),
	}
	m.cron.Start() // 啟動 Cron 引擎

	// 預設註冊：每 20 分鐘執行一次主動心跳決策 (Heartbeat)
	// 你可以根據需求調整頻率，例如 "@every 5m"
	m.cron.AddFunc("*/20 * * * *", func() {
		m.runHeartbeat()
	})

	// 新增任務：每天早上 07:00 執行晨間簡報
	// Cron 格式: "分 時 日 月 週"
	_, err := m.cron.AddFunc("0 7 * * *", func() {
		fmt.Println("✅[Scheduler] 正在產生晨間簡報...")
		ctx := context.Background()
		// 呼叫我們之前實作的簡報功能
		err := m.brain.GenerateMorningBriefing(ctx)
		if err != nil {
			fmt.Printf("⚠️ [Scheduler] 晨間簡報執行失敗: %v\n", err)
		}
	})

	if err != nil {
		fmt.Printf("⚠️ [Scheduler] 註冊簡報任務失敗: %v\n", err)
	}
	// m.startWorkers()

	return m
}

// ==========================================
// 2. 新增：Worker Pool 邏輯 (處理刪除檔案等任務)
// ==========================================

func (m *Manager) startWorkers() {
	for i := 0; i < m.workerCount; i++ {
		m.wg.Add(1)
		go m.workerLoop(i + 1)
	}
	fmt.Printf("✅ [Scheduler] 已啟動 Cron 引擎與 %d 個背景工作執行緒。\n", m.workerCount)
}

func (m *Manager) workerLoop(id int) {
	defer m.wg.Done()
	for {
		select {
		case job, ok := <-m.bgJobQueue:
			if !ok {
				return
			}
			// 執行任務並捕捉 Panic
			func() {
				defer func() {
					if r := recover(); r != nil {
						log.Printf("[Worker-%d] 任務 Panic: %v", id, r)
					}
				}()
				if err := job.Execute(); err != nil {
					log.Printf("[Worker-%d] 任務失敗 (%s): %v", id, job.Name(), err)
				}
			}()
		case <-m.quit:
			return
		}
	}
}

// AddBackgroundTask 用於新增「即時執行」的任務 (例如：刪除檔案)
func (m *Manager) AddBackgroundTask(job Job) error {
	select {
	case m.bgJobQueue <- job:
		return nil
	default:
		return errors.New("background job queue is full")
	}
}

// Stop 優雅關閉 (同時停止 Cron 和 Workers)
func (m *Manager) Stop() {
	// 1. 停止 Cron
	ctx := m.cron.Stop()
	<-ctx.Done() // 等待正在執行的 Cron Job 結束

	// 2. 停止 Workers
	close(m.quit)
	m.wg.Wait()

	fmt.Println("✅ [Scheduler] 所有排程與背景任務已停止。")
}

// ==========================================
// 3. 原有的 Cron 邏輯 (保持不變或微調)
// ==========================================

// RegisterTaskType 讓你在啟動時註冊哪些功能可以被排程 (Cron 用)
func (m *Manager) RegisterTaskType(name string, fn TaskFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.registry[name] = fn
}

// LoadJobs 從資料庫載入任務
func (m *Manager) LoadJobs() error {
	ctx := context.Background()
	jobs, err := m.db.GetCronJobs(ctx)
	if err != nil {
		return err
	}

	for _, job := range jobs {
		// 檢查任務類型是否已註冊
		m.mu.RLock()
		fn, ok := m.registry[job.TaskType]
		m.mu.RUnlock()

		if !ok {
			log.Printf("⚠️ [Scheduler] Warning: Task type '%s' not registered for job '%s'. Skipping.", job.TaskType, job.Name)
			continue
		}

		// 加入 Cron
		id, err := m.cron.AddFunc(job.CronSpec, fn)
		if err != nil {
			log.Printf("⚠️ [Scheduler] Error restoring job '%s' with spec '%s': %v", job.Name, job.CronSpec, err)
			continue
		}

		// 更新記憶體狀態
		m.mu.Lock()
		m.jobs[job.Name] = ScheduledJob{
			EntryID:     id,
			TaskName:    job.TaskType,
			CronSpec:    job.CronSpec,
			Description: job.Description,
		}
		m.mu.Unlock()
		fmt.Printf("✅ [Scheduler] Restored job: %s (%s)\n", job.Name, job.CronSpec)
	}
	return nil
}

// AddJob 加入 Cron 排程任務 (包含持久化)
func (m *Manager) AddJob(name, spec, taskType, desc string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	fn, ok := m.registry[taskType]
	if !ok {
		return fmt.Errorf("不支援的任務類型: %s", taskType)
	}

	// 1. 先寫入資料庫
	if err := m.db.AddCronJob(context.Background(), name, spec, taskType, desc); err != nil {
		return fmt.Errorf("failed to persist job: %w", err)
	}

	// 2. 如果已存在，先移除舊的 Cron Entry
	if oldJob, exists := m.jobs[name]; exists {
		m.cron.Remove(oldJob.EntryID)
	}

	// 3. 加入新的 Cron Entry
	id, err := m.cron.AddFunc(spec, fn)
	if err != nil {
		// 回滾 DB (這裡簡化，不刪除 DB，但這會導致資料不一致，實務上應更嚴謹)
		// 如果 Cron 格式錯誤，DB 已經存了，下次啟動也會錯誤。
		// 更好的做法是先驗證 Spec，再存 DB。
		// 但 Cron 庫驗證 Spec 比較麻煩，這裡我們假設 Spec 在前端或業務層已驗證，或接受這種短暫不一致。
		// 為求穩健，這裡嘗試刪除 DB entry
		_ = m.db.RemoveCronJob(context.Background(), name)
		return fmt.Errorf("Cron 格式錯誤 (%s): %v", spec, err)
	}

	m.jobs[name] = ScheduledJob{
		EntryID:     id,
		TaskName:    taskType,
		CronSpec:    spec,
		Description: desc,
	}
	fmt.Printf("[Scheduler] Cron Job Added: %s (%s)\n", name, spec)
	return nil
}

// RemoveJob 移除排程任務
func (m *Manager) RemoveJob(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	job, exists := m.jobs[name]
	if !exists {
		return fmt.Errorf("job not found: %s", name)
	}

	// 1. 移除 DB
	if err := m.db.RemoveCronJob(context.Background(), name); err != nil {
		return fmt.Errorf("failed to remove from db: %w", err)
	}

	// 2. 移除 Cron Entry
	m.cron.Remove(job.EntryID)
	delete(m.jobs, name)
	fmt.Printf("[Scheduler] Job Removed: %s\n", name)
	return nil
}

func (m *Manager) ListJobs() map[string]ScheduledJob {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.jobs
}
