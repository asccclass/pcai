package notify

import (
	"context"
	"fmt"

	"github.com/go-resty/resty/v2"
)

// Telegram 實作
type TelegramNotifier struct {
	Token  string
	ChatID string
	Client *resty.Client
}

func (t *TelegramNotifier) Send(ctx context.Context, message string) error {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.Token)
	resp, err := t.Client.R().
		SetContext(ctx).
		SetBody(map[string]string{
			"chat_id": t.ChatID,
			"text":    message,
		}).
		Post(url)

	if err != nil {
		return err
	}
	if resp.IsError() {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode(), resp.String())
	}
	return nil
}

func (t *TelegramNotifier) Name() string { return "Telegram" }
