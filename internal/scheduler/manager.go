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
	// GenerateMorningBriefing 讓 Scheduler 知道大腦具備產生簡報的能力
	GenerateMorningBriefing(ctx context.Context) error
	// RunPatrol 執行閒置時的背景巡邏
	RunPatrol(ctx context.Context) error
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
		fmt.Println("[Scheduler] Heartbeat: AI decided to stay quiet. Starting background patrol...")
		if err := m.brain.RunPatrol(ctx); err != nil {
			fmt.Printf("[Scheduler] Patrol Error: %v\n", err)
		}
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

	// 記錄已經載入的 TaskType，用來檢查 DB 中是否有重複類型的任務
	loadedTypes := make(map[string]string) // map[TaskType]JobName

	for _, job := range jobs {
		// 檢查資料庫是否有重複類型的任務
		if existingName, exists := loadedTypes[job.TaskType]; exists {
			log.Printf("⚠️ [Scheduler] 發現重複的任務類型 '%s' (已載入: '%s', 欲載入: '%s')，準備從資料庫移除後者...", job.TaskType, existingName, job.Name)
			// 從資料庫中移除重複的記錄，保留先讀到的一筆
			if err := m.db.RemoveCronJob(ctx, job.Name); err != nil {
				log.Printf("⚠️ [Scheduler] 移除資料庫中重複任務 '%s' 失敗: %v", job.Name, err)
			} else {
				log.Printf("✅ [Scheduler] 已從資料庫移除重複任務: %s", job.Name)
			}
			continue
		}

		// 檢查任務類型是否已註冊
		m.mu.RLock()
		fn, ok := m.registry[job.TaskType]
		m.mu.RUnlock()

		if !ok {
			log.Printf("⚠️ [Scheduler] Warning: Task type '%s' not registered for job '%s'. Skipping.", job.TaskType, job.Name)
			continue
		}

		// 標記該類型已載入
		loadedTypes[job.TaskType] = job.Name

		// 避免重複註冊：如果記憶體中已存在該任務，先從 Cron 引擎中移除舊的
		m.mu.Lock()
		if oldJob, exists := m.jobs[job.Name]; exists {
			m.cron.Remove(oldJob.EntryID)
		}
		m.mu.Unlock()

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
		// 回滾 DB
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

// EnsureSystemJob 確保系統預設任務存在，如果資料庫中已經有該類型的任務，則不重複新增 (避免多筆)
func (m *Manager) EnsureSystemJob(name, spec, taskType, desc string) error {
	// 檢查資料庫是否已經有同類型的任務
	ctx := context.Background()
	jobs, err := m.db.GetCronJobs(ctx)
	if err != nil {
		return fmt.Errorf("failed to get existing jobs: %w", err)
	}

	// 檢查是否有同類型 (TaskType) 的任務。如果有同類型的，表示使用者或系統已經設定過，不再強制寫入新紀錄
	for _, job := range jobs {
		if job.TaskType == taskType {
			// 如果名稱不同，但類型相同，為避免重疊執行，我們視為已設定。
			// 如果名稱也相同，且 Spec 不同，我們以資料庫為準（不覆蓋）。
			return nil
		}
	}

	// 若完全沒有該類型的任務，則作為預設值加入
	log.Printf("ℹ️ [Scheduler] 初始化預設系統排程: %s (%s)", name, spec)
	return m.AddJob(name, spec, taskType, desc)
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

// RunJobNow 立即執行指定的任務
func (m *Manager) RunJobNow(taskName string) error {
	m.mu.RLock()
	job, exists := m.jobs[taskName]
	m.mu.RUnlock()

	if !exists {
		// 嘗試如果不支援的名字，是否是 Type?
		// 暫時只支援已排程的任務名稱
		return fmt.Errorf("job not found: %s", taskName)
	}

	m.mu.RLock()
	fn, ok := m.registry[job.TaskName] // job.TaskName 其實存的是 TaskType... Wait.
	// Check AddJob: m.jobs[name] = ScheduledJob{..., TaskName: taskType, ...}
	// Yes, TaskName field in ScheduledJob struct actually holds the Type.
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("task type '%s' not registered", job.TaskName)
	}

	// Async run to avoid blocking? Or Sync?
	// The user might want to know it finished.
	// But TaskFunc doesn't return error.
	fmt.Printf("🚀 [Scheduler] Manually triggering job: %s\n", taskName)
	go fn()
	return nil
}
