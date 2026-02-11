package channel

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
)

// Envelope 封裝了跨平台的統一訊息格式
type Envelope struct {
	SenderID string
	Content  string
	Platform string
	// Reply 讓 Dispatcher 不需要知道如何調用 Telegram API 就能回覆
	Reply func(text string) error
	// MarkProcessing 顯示「正在輸入中...」或類似狀態
	MarkProcessing func() error
}

// TelegramChannel 實作了適配器結構
type TelegramChannel struct {
	bot         *telego.Bot
	stopPolling context.CancelFunc
}

// NewTelegramChannel 初始化機器人
func NewTelegramChannel(token string, debug bool) (*TelegramChannel, error) {
	// 使用預設設定初始化 Bot
	options := []telego.BotOption{}
	if debug {
		options = append(options, telego.WithDefaultDebugLogger())
	}

	bot, err := telego.NewBot(token, options...)
	if err != nil {
		return nil, err
	}
	return &TelegramChannel{bot: bot}, nil
}

// Listen 啟動長輪詢 (Long Polling) 監聽訊息
func (t *TelegramChannel) Listen(handler func(Envelope)) {
	// 建立可取消的 Context
	ctx, cancel := context.WithCancel(context.Background())
	t.stopPolling = cancel

	// 設定長輪詢參數
	updates, err := t.bot.UpdatesViaLongPolling(ctx, &telego.GetUpdatesParams{
		Timeout: 60,
	})
	if err != nil {
		log.Fatalf("⚠️ [Telegram] 無法啟動長輪詢: %v", err)
		os.Exit(1)
	}

	fmt.Println("✅ [Telegram] 頻道已啟動，監聽中...")

	for update := range updates {
		// 我們只處理文字訊息
		if update.Message != nil && update.Message.Text != "" {
			msg := update.Message
			chatID := msg.Chat.ID

			// 建立封裝對象
			env := Envelope{
				SenderID: fmt.Sprintf("%d", chatID),
				Content:  msg.Text,
				Platform: "telegram",
				Reply: func(text string) error {
					// 封裝發送邏輯
					_, err := t.bot.SendMessage(context.Background(), tu.Message(
						tu.ID(chatID),
						text,
					))
					return err
				},
				MarkProcessing: func() error {
					// 發送 "正在輸入" 狀態
					return t.bot.SendChatAction(context.Background(), tu.ChatAction(
						tu.ID(chatID),
						telego.ChatActionTyping,
					))
				},
			}

			// 將封裝好的訊息丟給 Dispatcher 層處理
			go handler(env)
		}
	}
	fmt.Println("🛑 [Telegram] 長輪詢已結束")
}

// Stop 停止長輪詢
func (t *TelegramChannel) Stop() {
	if t.stopPolling != nil {
		fmt.Println("🛑 [Telegram] 正在停止頻道...")
		t.stopPolling()
	}
}
