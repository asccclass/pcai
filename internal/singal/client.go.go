package signal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

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
