package gateway

import (
	"log"
	"strings"
	"sync"

	"github.com/asccclass/pcai/internal/channel" // 引用剛剛建立的 Envelope 結構
)

// Processor 介面，未來可以由 AIProcessor 實作
type Processor interface {
	Process(env channel.Envelope) string
}

// Dispatcher 負責調度訊息與權限控管
type Dispatcher struct {
	processor Processor
	// 使用 Map 存儲授權用戶，並用 RWMutex 保證並發安全
	authorizedUsers sync.Map
	adminID         string
}

// NewDispatcher 初始化調度器
func NewDispatcher(p Processor, adminID string) *Dispatcher {
	d := &Dispatcher{
		processor: p,
		adminID:   adminID,
	}
	// 預設將管理員加入白名單
	d.authorizedUsers.Store(adminID, true)
	return d
}

// HandleMessage 是主要的進入點，會被各個 Channel 調用
func (d *Dispatcher) HandleMessage(env channel.Envelope) {
	log.Printf("[%s] 收到訊息 (來自 %s): %s", env.Platform, env.SenderID, env.Content)

	// 1. 權限檢查
	if !d.isAuthorized(env.SenderID) {
		log.Printf("拒絕存取：用戶 %s 未在白名單中", env.SenderID)
		_ = env.Reply("⚠️ 您尚未獲得授權，請聯繫管理員。您的 ID 是: " + env.SenderID)
		return
	}

	// 2. 指令解析 (如果是核心系統指令)
	if strings.HasPrefix(env.Content, "/") {
		if d.handleSystemCommand(env) {
			return // 如果是系統指令且處理完成，則直接返回
		}
	}

	// 3. 業務邏輯處理 (交給 Processor，例如 AI 或 CMD 工具)
	// 這裡可以做非同步處理，避免阻塞下一個訊息接收
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[Dispatcher] Panic recovered: %v", r)
			}
		}()

		log.Printf("[Dispatcher] Processing message from %s...", env.SenderID)
		response := d.processor.Process(env)
		log.Printf("[Dispatcher] Got response for %s (len: %d)", env.SenderID, len(response))

		if response != "" {
			err := env.Reply(response)
			if err != nil {
				log.Printf("[Dispatcher] 回覆發送失敗: %v", err)
			} else {
				log.Printf("[Dispatcher] Reply sent successfully to %s", env.SenderID)
			}
		} else {
			log.Printf("[Dispatcher] Empty response, skipping reply.")
		}
	}()
}

// isAuthorized 檢查用戶是否在白名單
func (d *Dispatcher) isAuthorized(userID string) bool {
	_, ok := d.authorizedUsers.Load(userID)
	return ok
}

// handleSystemCommand 處理網關層級的指令（例如增加白名單）
func (d *Dispatcher) handleSystemCommand(env channel.Envelope) bool {
	cmd := strings.Fields(env.Content)
	if len(cmd) == 0 {
		return false
	}

	switch cmd[0] {
	case "/auth": // 範例：管理員手動授權 /auth 123456
		if env.SenderID != d.adminID {
			_ = env.Reply("只有管理員可以使用此指令。")
			return true
		}
		if len(cmd) > 1 {
			targetID := cmd[1]
			d.authorizedUsers.Store(targetID, true)
			_ = env.Reply("✅ 已授權用戶: " + targetID)
			return true
		}
	case "/status":
		_ = env.Reply("🟢 網關運行中，平台: " + env.Platform)
		return true
	}

	return false // 不是系統指令，交給下層 Processor
}
