package notify

import (
	"context"
	"log"
	"time"

	"github.com/asccclass/pcai/internal/database"
)

// Notifier 是所有通知管道必須遵守的協議
type Notifier interface {
	Send(ctx context.Context, message string) error
	Name() string // 用於日誌記錄，例如 "Telegram"
}

// Dispatcher 管理多個 Notifier
type Dispatcher struct {
	notifiers []Notifier
	deduper   *Deduper
	db        *database.DB // 需要存取資料庫來標記簡報
}

// IsSilentMode 檢查目前是否為靜音時段 (23:00 - 07:00)
func (d *Dispatcher) IsSilentMode() bool {
	hour := time.Now().Hour()
	return hour >= 23 || hour < 7
}

func NewDispatcher(coolDown time.Duration) *Dispatcher {
	return &Dispatcher{
		notifiers: make([]Notifier, 0),
		deduper:   NewDeduper(coolDown),
	}
}

func (d *Dispatcher) Register(n Notifier) {
	d.notifiers = append(d.notifiers, n)
}

// Dispatch 同時送出通知
func (d *Dispatcher) Dispatch(ctx context.Context, level string, message string) {
	// 如果希望某些「極度緊急」的訊息（如火災警報、伺服器斷線）不准去重
	if level == "EMERGENCY" {
		return
	}
	// 核心優化：如果訊息重複且在冷卻期內，直接攔截
	if !d.deduper.ShouldSend(message) {
		log.Printf("⏳ [Deduper] 訊息重複，已攔截發送。")
		return
	}
	// 靜音時段邏輯：除非是 URGENT，否則不執行真正的發送
	if d.IsSilentMode() && level != "URGENT" {
		log.Printf("🌙 靜音時段中，訊息已存入資料庫等待晨間簡報。")
		return
	}
	// 使用 WaitGroup 是為了確保在某些需要同步的場景下可以等待
	// 但在 Heartbeat 中我們通常採用「發後不理 (Fire and Forget)」
	for _, n := range d.notifiers {
		// 為了避免 closure 補捉到錯誤的變數，需傳入參數
		go func(notifier Notifier, msg string) {
			// 注意：這裡使用 context.Background() 或是從 ctx 衍生
			// 避免因為主進程的 ctx 取消導致通知發一半中斷
			sendCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			err := notifier.Send(sendCtx, msg)
			if err != nil {
				log.Printf("❌ [%s] 通知發送失敗: %v", notifier.Name(), err)
			} else {
				log.Printf("✅ [%s] 通知發送成功", notifier.Name())
			}
		}(n, message)
	}
}
