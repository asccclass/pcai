package notify

import (
	"context"
	"fmt"

	"github.com/go-resty/resty/v2"
)

// LINE Notify 實作
type LineNotifier struct {
	Token  string
	Client *resty.Client
}

func (l *LineNotifier) Send(ctx context.Context, message string) error {
	resp, err := l.Client.R().
		SetContext(ctx).
		SetHeader("Authorization", "Bearer "+l.Token).
		SetFormData(map[string]string{"message": message}).
		Post("https://notify-api.line.me/api/notify")

	if err != nil {
		return err
	}
	if resp.IsError() {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode(), resp.String())
	}
	return nil
}

func (l *LineNotifier) Name() string { return "LineNotify" }
