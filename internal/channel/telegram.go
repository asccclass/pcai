package channel

import (
	"context" // Added context
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
}

// TelegramChannel 實作了適配器結構
type TelegramChannel struct {
	bot *telego.Bot
}

// NewTelegramChannel 初始化機器人
func NewTelegramChannel(token string) (*TelegramChannel, error) {
	// 使用預設配置初始化
	bot, err := telego.NewBot(token, telego.WithDefaultDebugLogger())
	if err != nil {
		return nil, err
	}
	return &TelegramChannel{bot: bot}, nil
}

// Listen 啟動長輪詢 (Long Polling) 監聽訊息
// 當收到訊息時，會封裝成 Envelope 並丟給傳入的 handler 處理
func (t *TelegramChannel) Listen(handler func(Envelope)) {
	// 取得訊息更新通道
	// UpdatesViaLongPolling requires (ctx, params, options...)
	// Since StopLongPolling is removed, we control it via context.
	// But here we just want it to run forever, so we use Background and don't cancel explicitly.
	updates, err := t.bot.UpdatesViaLongPolling(context.Background(), nil)
	if err != nil {
		log.Fatalf("無法啟動長輪詢: %v", err)
		os.Exit(1)
	}

	// defer t.bot.StopLongPolling() // Removed as it is undefined

	log.Println("Telegram 頻道已啟動，監聽中...")

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
			}

			// 將封裝好的訊息丟給 Dispatcher 層處理
			go handler(env)
		}
	}
}
