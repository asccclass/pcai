package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

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
}

// TaskFunc 是原有的 Cron 任務函式類型
type TaskFunc func()

type ScheduledJob struct {
	EntryID     cron.EntryID `json:"entry_id"`
	TaskName    string       `json:"task_name"`
	CronSpec    string       `json:"cron_spec"`
	Description string       `json:"description"`
}

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

	// --- 新增的 Worker Pool 部分 ---
	bgJobQueue  chan Job       // 即時任務佇列
	workerCount int            // Worker 數量
	quit        chan struct{}  // 關閉訊號
	wg          sync.WaitGroup // 等待群組

	// Heartbeat 相關
	isThinking int32 // 防止重複執行
	brain      HeartbeatBrain
}

// runHeartbeat 是核心邏輯
func (m *Manager) runHeartbeat() { // 1. 併發防護：確保不會有多個心跳同時在「思考」，避免資源浪費或邏輯混亂
	if !atomic.CompareAndSwapInt32(&m.isThinking, 0, 1) {
		fmt.Println("[Scheduler] Heartbeat skipped: Brain is already busy thinking.")
		return
	}
	defer atomic.StoreInt32(&m.isThinking, 0)

	// 設定超時，避免 LLM 響應過久掛起系統
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	fmt.Printf("[Scheduler] Heartbeat started at %s\n", time.Now().Format("15:04:05"))

	// 2. 感知 (Sensing)
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

func NewManager(brain HeartbeatBrain) *Manager {
	// 1. 初始化 Cron
	c := cron.New(cron.WithSeconds()) // 建議維持秒級控制

	m := &Manager{
		cron:     c,
		registry: make(map[string]TaskFunc),
		jobs:     make(map[string]ScheduledJob),
		brain:    brain,

		// 2. 初始化 Worker Pool
		bgJobQueue:  make(chan Job, 100), // 緩衝區 100
		workerCount: 3,                   // 預設 3 個 Worker
		quit:        make(chan struct{}),
	}
	m.cron.Start() // 啟動 Cron 引擎

	// 預設註冊：每 20 分鐘執行一次主動心跳決策 (Heartbeat)
	// 你可以根據需求調整頻率，例如 "@every 5m"
	m.cron.AddFunc("0 */20 * * * *", func() {
		m.runHeartbeat()
	})

	// 3. 啟動背景 Workers
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
	fmt.Printf("[Scheduler] 已啟動 Cron 引擎與 %d 個背景工作執行緒。\n", m.workerCount)
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

	fmt.Println("[Scheduler] 所有排程與背景任務已停止。")
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

// AddJob 加入 Cron 排程任務
func (m *Manager) AddJob(name, spec, taskType string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	fn, ok := m.registry[taskType]
	if !ok {
		return fmt.Errorf("不支援的任務類型: %s", taskType)
	}

	// 這裡使用 AddFunc 加入 Cron
	id, err := m.cron.AddFunc(spec, fn)
	if err != nil {
		return fmt.Errorf("Cron 格式錯誤 (%s): %v", spec, err)
	}

	m.jobs[name] = ScheduledJob{
		EntryID:  id,
		TaskName: taskType,
		CronSpec: spec,
	}
	fmt.Printf("[Scheduler] Cron Job Added: %s (%s)\n", name, spec)
	return nil
}

func (m *Manager) ListJobs() map[string]ScheduledJob {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.jobs
}
