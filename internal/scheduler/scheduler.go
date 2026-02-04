package scheduler

import (
	"log"

	"github.com/robfig/cron/v3"
)

type CronEngine struct {
	scheduler *cron.Cron
}

func NewCronEngine() *CronEngine {
	// 使用 Seconds 模式 (可選) 或標準 5 欄位模式
	return &CronEngine{
		scheduler: cron.New(), // cron.WithSeconds()),
	}
}

func (e *CronEngine) Start() {
	e.scheduler.Start()
	log.Println("Cron Engine started...")
}

func (e *CronEngine) Stop() {
	e.scheduler.Stop()
}

// AddTask 封裝添加任務的邏輯
func (e *CronEngine) AddTask(spec string, task func()) {
	_, err := e.scheduler.AddFunc(spec, task)
	if err != nil {
		log.Printf("Failed to add task: %v", err)
	}
}
