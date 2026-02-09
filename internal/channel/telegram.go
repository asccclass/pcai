package channel

import (
	"context" // Added context
	"fmt"
	"log"
	"os"

	"time"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
	"github.com/valyala/fasthttp"
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
	bot *telego.Bot
}

// NewTelegramChannel 初始化機器人
func NewTelegramChannel(token string) (*TelegramChannel, error) {
	// 建立自定義的 FastHTTP 客戶端，設定較長的超時時間
	// 這是為了配合長輪詢 (Long Polling)
	client := &fasthttp.Client{
		ReadTimeout:  70 * time.Second, // 比長輪詢的 60 秒稍長
		WriteTimeout: 70 * time.Second,
	}

	// 使用自定義的 HTTP 客戶端初始化 Bot
	bot, err := telego.NewBot(token, telego.WithFastHTTPClient(client))
	if err != nil {
		return nil, err
	}
	return &TelegramChannel{bot: bot}, nil
}

// Listen 啟動長輪詢 (Long Polling) 監聽訊息
// 當收到訊息時，會封裝成 Envelope 並丟給傳入的 handler 處理
func (t *TelegramChannel) Listen(handler func(Envelope)) {
	// 設定長輪詢參數
	// Timeout 設定為 60 秒，告訴 Telegram 伺服器若無新訊息則保持連線 60 秒
	updates, err := t.bot.UpdatesViaLongPolling(context.Background(), &telego.GetUpdatesParams{
		Timeout: 60,
	})
	if err != nil {
		log.Fatalf("⚠️ [Telegram] 無法啟動長輪詢: %v", err)
		os.Exit(1)
	}

	// defer t.bot.StopLongPolling() // Removed as it is undefined

	log.Println("✅ [Telegram] 頻道已啟動，監聽中...")

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
}
