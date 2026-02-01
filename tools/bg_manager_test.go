package tools

import (
	"testing"
	"time"
)

func TestBackgroundManager(t *testing.T) {
	bm := NewBackgroundManager()

	// 測試：新增一個模擬任務 (執行 1 秒)
	cmdStr := "echo 'hello test'"
	id := bm.AddTask(cmdStr, func() (string, error) {
		time.Sleep(1 * time.Second)
		return "success", nil
	})

	// 檢查 ID 是否正確生成
	if id != 1 {
		t.Errorf("預期 ID 為 1，得到 %d", id)
	}

	// 檢查初始狀態
	if bm.tasks[id].Status != StatusRunning {
		t.Errorf("預期狀態為 Running，得到 %s", bm.tasks[id].Status)
	}

	// 等待任務完成並檢查 NotifyChan
	select {
	case msg := <-bm.NotifyChan:
		if bm.tasks[id].Status != StatusSuccess {
			t.Errorf("任務完成後狀態應為 Success")
		}
		t.Logf("收到通知: %s", msg)
	case <-time.After(2 * time.Second):
		t.Fatal("任務逾時，未收到通知")
	}
}
