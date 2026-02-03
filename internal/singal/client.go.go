package signal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// 輔助方法：抓取 Signal (與你之前的 REST API 設計銜接)
type signalResponse struct {
	Source  string `json:"source"`
	Content string `json:"content"`
}

func fetchSignalMessages(ctx context.Context) ([]signalResponse, error) {
	apiURL := "https://msg.justdrink.com.tw/v2/messages"

	resp, err := http.Get(apiURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var messages []signalResponse
	if err := json.NewDecoder(resp.Body).Decode(&messages); err != nil {
		return nil, err
	}

	return messages, nil
}

func SendNotification(recipient, message string) error {
	apiURL := "https://msg.justdrink.com.tw/v2/send"

	payload := map[string]interface{}{
		"message":    message,
		"number":     "+886921609364", // 你的 Signal 註冊門號
		"recipients": []string{recipient},
	}

	jsonData, _ := json.Marshal(payload)
	resp, err := http.Post(apiURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API 回傳錯誤狀態碼: %d", resp.StatusCode)
	}

	return nil
}
