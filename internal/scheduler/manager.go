// 核心排程管理器 (internal/scheduler/manager.go)，這個組件負責管理 robfig/cron 實體與任務對應。
package scheduler

import (
	"fmt"
	"sync"

	"github.com/robfig/cron/v3"
)

// TaskFunc 是我們定義的任務函式類型
type TaskFunc func()

type ScheduledJob struct {
	EntryID     cron.EntryID `json:"entry_id"`
	TaskName    string       `json:"task_name"`
	CronSpec    string       `json:"cron_spec"`
	Description string       `json:"description"`
}

type Manager struct {
	cron     *cron.Cron
	registry map[string]TaskFunc     // 註冊可用的任務類型 (如 "read_gmail")
	jobs     map[string]ScheduledJob // 存放已排程的任務
	mu       sync.RWMutex
}

func NewManager() *Manager {
	c := cron.New() //New(cron.WithSeconds()) // 支援到秒等級
	c.Start()
	return &Manager{
		cron:     c,
		registry: make(map[string]TaskFunc),
		jobs:     make(map[string]ScheduledJob),
	}
}

// RegisterTaskType 讓你在啟動時註冊哪些功能可以被排程
func (m *Manager) RegisterTaskType(name string, fn TaskFunc) {
	m.registry[name] = fn
}

func (m *Manager) AddJob(name, spec, taskType string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 1. 檢查任務類型是否存在
	fn, ok := m.registry[taskType]
	if !ok {
		return fmt.Errorf("不支援的任務類型: %s", taskType)
	}

	// 2. 如果名稱重複，可以先移除舊的（避免同名任務堆積）
	// if oldJob, exists := m.jobs[name]; exists {
	//     m.cron.Remove(oldJob.EntryID)
	// }

	// 3. 加入排程
	id, err := m.cron.AddFunc(spec, fn)
	if err != nil {
		return fmt.Errorf("Cron 格式錯誤 (%s): %v", spec, err) // 直接回傳給 AgentTool.Run，讓 AI 知道錯誤原因
	}

	// 建立結構體實例再賦值
	m.jobs[name] = ScheduledJob{
		EntryID:  id, // 儲存 cron 的 EntryID 方便日後刪除
		TaskName: taskType,
		CronSpec: spec, // 這裡存入 spec 字串
	}
	fmt.Printf("Job Added: %s\n", name)
	return nil
}

func (m *Manager) ListJobs() map[string]ScheduledJob {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.jobs
}
