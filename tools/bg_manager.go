package tools

// 這個模組負責追蹤任務狀態（執行中、成功、失敗），並存放執行結果。

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// TaskStatus 定義任務狀態
type TaskStatus string

const (
	StatusRunning TaskStatus = "執行中"
	StatusSuccess TaskStatus = "成功"
	StatusFailed  TaskStatus = "失敗"
)

// BackgroundTask 儲存單個任務的詳細資訊
type BackgroundTask struct {
	ID        int        `json:"id"`
	Command   string     `json:"command"`
	Status    TaskStatus `json:"status"`
	Result    string     `json:"result"`
	StartTime time.Time  `json:"start_time"`
	EndTime   time.Time  `json:"end_time"` // 新增此欄位
}

// BackgroundManager 管理所有背景任務
type BackgroundManager struct {
	tasks      map[int]*BackgroundTask
	nextID     int
	mu         sync.Mutex
	NotifyChan chan string // 用於推播通知
}

func NewBackgroundManager() *BackgroundManager {
	return &BackgroundManager{
		tasks:      make(map[int]*BackgroundTask),
		nextID:     1,
		NotifyChan: make(chan string, 10),
	}
}

// AddTask 啟動並追蹤一個新任務
func (bm *BackgroundManager) AddTask(command string, execFunc func() (string, error)) int {
	bm.mu.Lock()
	id := bm.nextID
	bm.nextID++
	task := &BackgroundTask{
		ID:        id,
		Command:   command,
		Status:    StatusRunning,
		StartTime: time.Now(),
	}
	bm.tasks[id] = task
	bm.mu.Unlock()

	// 啟動非同步執行
	go func() {
		result, err := execFunc()
		bm.mu.Lock()
		defer bm.mu.Unlock()

		if err != nil {
			task.Status = StatusFailed
			task.Result = err.Error()
		} else {
			task.Status = StatusSuccess
			task.Result = result
		}
		task.EndTime = time.Now() // 任務結束時記錄時間
		// 推播通知訊息
		bm.NotifyChan <- fmt.Sprintf("🔔 [任務 #%d 完成] 指令: %s", id, command)
	}()
	return id
}

// GetTaskSummary 回傳簡短的任務統計，用於健康檢查
func (bm *BackgroundManager) GetTaskSummary() string {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	running := 0
	total := len(bm.tasks)
	for _, t := range bm.tasks {
		if t.Status == StatusRunning {
			running++
		}
	}

	if total == 0 {
		return "無背景任務"
	}
	return fmt.Sprintf("%d 執行中 / %d 總任務", running, total)
}

// GetTaskList 讓 AI 可以查看所有任務
func (bm *BackgroundManager) GetTaskList() string {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if len(bm.tasks) == 0 {
		return "目前沒有背景任務。"
	}
	// 建立 Markdown 表格標頭
	header := "| ID | 狀態 | 指令 | 耗時 | 結果/錯誤 |\n"
	separator := "|---|---|---|---|---|\n"

	var rows string
	for i := 1; i < bm.nextID; i++ {
		t, stringsExist := bm.tasks[i]
		if !stringsExist {
			continue
		}
		duration := time.Since(t.StartTime).Round(time.Second).String()
		if t.Status != StatusRunning {
			// 如果已經結束，計算從開始到結束的總時長 (假設你在 Task 結構有存 EndTime 的話，這裡暫用簡單邏輯)
			duration = "已結束"
		}

		// 處理結果字串：只取前 30 個字元，並移除換行符避免表格破掉
		displayResult := strings.ReplaceAll(t.Result, "\n", " ")
		if len(displayResult) > 30 {
			displayResult = displayResult[:27] + "..."
		}
		if displayResult == "" && t.Status == StatusRunning {
			displayResult = "執行中..."
		}

		rows += fmt.Sprintf("| #%d | %s | `%s` | %s | %s |\n",
			t.ID, t.Status, t.Command, duration, displayResult)
	}
	return "\n### 當前背景任務清單\n" + header + separator + rows
}
