package memory

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"sync"
)

// FileDeletionJob 定義刪除任務的資料結構
type FileDeletionJob struct {
	FilePath string
	Content  string
	// 互斥鎖通常建議放在全域或 Manager 層級，
	// 但如果 Job 是由 Tool 產生的，我們可以共用 Tool 裡的鎖
	Mutex *sync.Mutex
}

// Name 任務名稱 (供 Scheduler 識別或 Log 使用)
func (j *FileDeletionJob) Name() string {
	return "MemoryFileCleanupTask"
}

// Execute 這是 Scheduler 會去呼叫的方法
func (j *FileDeletionJob) Execute() error {
	if j.FilePath == "" || j.Content == "" {
		return nil
	}

	// 這裡放入原本 deleteLineFromFile 的邏輯
	// 因為是非同步執行，必須確保有 Lock 保護檔案完整性
	if j.Mutex != nil {
		j.Mutex.Lock()
		defer j.Mutex.Unlock()
	}

	// --- 檔案操作邏輯開始 ---
	input, err := os.Open(j.FilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("讀取檔案失敗: %w", err)
	}

	var newLines []string
	found := false
	targetTrimmed := strings.TrimSpace(j.Content)

	// 3. 逐行掃描並過濾
	scanner := bufio.NewScanner(input)
	for scanner.Scan() {
		line := scanner.Text()
		trimmedLine := strings.TrimSpace(line)

		// 比對邏輯：如果這行內容等於要刪除的內容，則跳過 (即刪除)
		if trimmedLine != "" && trimmedLine == targetTrimmed {
			found = true
			continue
		}
		newLines = append(newLines, line)
	}
	input.Close() // 讀取完畢即關閉，準備寫入
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("讀取檔案過程發生錯誤: %w", err)
	}

	if !found {
		return nil // 沒找到就不寫回，節省 I/O
	}

	// 5. 將過濾後的內容重新寫回檔案 (覆蓋寫入)
	// 使用 0644 權限 (rw-r--r--)
	err = os.WriteFile(j.FilePath, []byte(strings.Join(newLines, "\n")+"\n"), 0644)
	if err != nil {
		return fmt.Errorf("寫回記憶檔案失敗: %w", err)
	}

	return nil
}
