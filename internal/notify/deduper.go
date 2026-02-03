// 實作去重過濾器 (internal/notify/deduper.go), 我們建立一個 Deduper 結構，用來記錄已經發送過的訊息特徵（Hash）
package notify

import (
	"crypto/md5"
	"fmt"
	"sync"
	"time"
)

type Deduper struct {
	mu           sync.Mutex
	sentMessages map[string]time.Time
	expiration   time.Duration
}

func NewDeduper(expiry time.Duration) *Deduper {
	return &Deduper{
		sentMessages: make(map[string]time.Time),
		expiration:   expiry,
	}
}

// ShouldSend 檢查該訊息是否在冷卻期內已經發送過
func (d *Deduper) ShouldSend(message string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	// 1. 產生訊息的唯一特徵 (Hash)
	hash := fmt.Sprintf("%x", md5.Sum([]byte(message)))

	// 2. 檢查是否存在且未過期
	lastSent, exists := d.sentMessages[hash]
	if exists && time.Since(lastSent) < d.expiration {
		return false // 還在冷卻期內，不要重複發送
	}

	// 3. 更新發送時間
	d.sentMessages[hash] = time.Now()

	// 4. 定期清理過期資料（避免 Map 無限增長）
	go d.cleanUp()

	return true
}

func (d *Deduper) cleanUp() {
	d.mu.Lock()
	defer d.mu.Unlock()
	for h, t := range d.sentMessages {
		if time.Since(t) > d.expiration {
			delete(d.sentMessages, h)
		}
	}
}
