package channel

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/asccclass/pcai/internal/agent"
	"github.com/asccclass/pcai/internal/database"
	"github.com/mdp/qrterminal/v3"
	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"

	_ "modernc.org/sqlite" // 無 CGO 版本驅動
)

// WhatsAppChannel 負責與 WhatsApp 互動
type WhatsAppChannel struct {
	client     *whatsmeow.Client
	db         *database.DB
	logger     *agent.SystemLogger
	shutdown   chan struct{}
	selfID     types.JID
	store      *sqlstore.Container
	mu         sync.Mutex
	configPath string
}

// NewWhatsAppChannel 初始化 WhatsApp 頻道
func NewWhatsAppChannel(dbPath string, logger *agent.SystemLogger) (*WhatsAppChannel, error) {
	// 設定 whatsmeow 日誌
	dbLog := waLog.Stdout("Database", "ERROR", true)

	// 初始化 Store (使用 SQLite)
	// OpenClaw/Wacli 使用 file:store.db?_foreign_keys=on
	// fix: modernc.org/sqlite uses _pragma=foreign_keys(1)
	// fix: Enable WAL mode and busy timeout to avoid SQLITE_BUSY
	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)", dbPath)
	container, err := sqlstore.New(context.Background(), "sqlite", dsn, dbLog)
	if err != nil {
		return nil, fmt.Errorf("failed to open store: %w", err)
	}

	// 取得或建立 Device
	deviceStore, err := container.GetFirstDevice(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get device: %w", err)
	}

	// 初始化 Client
	clientLog := waLog.Stdout("Client", "INFO", true)
	client := whatsmeow.NewClient(deviceStore, clientLog)

	wc := &WhatsAppChannel{
		client:     client,
		store:      container,
		logger:     logger,
		shutdown:   make(chan struct{}),
		configPath: dbPath,
	}

	// 設定事件處理
	return wc, nil
}

// Listen 啟動監聽並處理認證
func (wc *WhatsAppChannel) Listen(handler func(Envelope)) error { // Update signature
	if wc.client.Store.ID == nil {
		// 未登入，顯示 QR Code
		qrChan, _ := wc.client.GetQRChannel(context.Background())
		err := wc.client.Connect()
		if err != nil {
			return fmt.Errorf("failed to connect: %w", err)
		}

		fmt.Println("請使用 WhatsApp 掃描 QR Code 登入:")
		for evt := range qrChan {
			if evt.Event == "code" {
				qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
			} else {
				fmt.Println("Login event:", evt.Event)
			}
		}
	} else {
		// 已登入，直接連線
		err := wc.client.Connect()
		if err != nil {
			return fmt.Errorf("failed to connect: %w", err)
		}
		wc.selfID = wc.client.Store.ID.ToNonAD()
		fmt.Printf("✅ WhatsApp Connected as %s\n", wc.selfID)
	}

	// 註冊事件處理器，這裡需要轉接給外部 handler
	wc.client.AddEventHandler(func(evt interface{}) {
		wc.handleEvent(evt, handler)
	})

	<-wc.shutdown
	return nil
}

// Stop 停止服務
func (wc *WhatsAppChannel) Stop() {
	if wc.client != nil {
		wc.client.Disconnect()
	}
	close(wc.shutdown)
}

// SendMessage 發送訊息
func (wc *WhatsAppChannel) SendMessage(chatID string, content string) error {
	// 解析 JID
	jid, err := types.ParseJID(chatID)
	if err != nil {
		// 嘗試補全 JID (如果是純數字)
		if strings.Contains(chatID, "@") {
			return fmt.Errorf("invalid JID: %w", err)
		}
		// 假設是個人號碼
		jid = types.NewJID(chatID, types.DefaultUserServer)
	}

	// 建構訊息
	msg := &waProto.Message{
		Conversation: proto.String(content),
	}

	// 發送
	_, err = wc.client.SendMessage(context.Background(), jid, msg)
	return err
}

// handleEvent 處理 whatsmeow 事件
func (wc *WhatsAppChannel) handleEvent(evt interface{}, handler func(Envelope)) {
	switch v := evt.(type) {
	case *events.Message:
		// 忽略自己的訊息
		if v.Info.IsFromMe {
			return
		}

		// 取得訊息內容 (目前只支援文字)
		var text string
		if v.Message.Conversation != nil {
			text = *v.Message.Conversation
		} else if v.Message.ExtendedTextMessage != nil && v.Message.ExtendedTextMessage.Text != nil {
			text = *v.Message.ExtendedTextMessage.Text
		} else {
			// 暫不處理非文字訊息
			return
		}

		chatID := v.Info.Chat.String()

		// 封裝成 Envelope 格式傳給 Dispatcher
		env := Envelope{
			Platform: "whatsapp",
			SenderID: chatID, // WhatsApp use JID as ID
			Content:  text,
			Reply: func(replyText string) error {
				return wc.SendMessage(chatID, replyText)
			},
			MarkProcessing: func() error {
				// WhatsApp 顯示 "正在輸入..."
				return wc.client.SendChatPresence(context.Background(), v.Info.Chat, types.ChatPresenceComposing, types.ChatPresenceMediaText)
			},
		}

		if handler != nil {
			handler(env)
		}
	}
}

// SetAdmin 設定管理者 ID (用於權限判斷，目前簡易實作)
func (wc *WhatsAppChannel) SetAdmin(adminID string) {
	// TODO: Store admin ID
}
