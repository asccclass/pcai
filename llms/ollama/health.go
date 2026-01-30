package ollama

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

// CheckService 檢查 Ollama API 是否在線
func CheckService(url string) bool {
	resp, err := http.Get(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// IsModelPulled 檢查特定模型是否已經下載
func IsModelPulled(url, modelName string) (bool, error) {
	resp, err := http.Get(url + "/api/tags")
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	var tags struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		return false, err
	}

	for _, m := range tags.Models {
		// Ollama 回傳的名稱可能帶有 :latest 標籤
		if m.Name == modelName || m.Name == modelName+":latest" {
			return true, nil
		}
	}
	return false, nil
}

// PullModel 下載模型
func PullModel(url, modelName string) error {
	// 構建 Pull API 的 URL
	// 格式: http://localhost:11434/api/pull
	pullURL := url + "/api/pull"

	// 構建請求體 (Payload)
	// 格式: {"name": "model_name"}
	payload := map[string]string{"name": modelName}
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("序列化模型名稱失敗: %v", err)
	}

	// 建立 POST 請求
	req, err := http.NewRequest("POST", pullURL, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return fmt.Errorf("建立請求失敗: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// 發送請求
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("連接 Ollama 服務失敗: %v", err)
	}
	defer resp.Body.Close()

	// 檢查 HTTP 狀態碼
	if resp.StatusCode != http.StatusOK {
		// 讀取錯誤訊息
		var errorResp map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&errorResp)
		return fmt.Errorf("下載失敗: HTTP %d - %v", resp.StatusCode, errorResp["error"])
	}

	// 成功處理
	fmt.Printf("模型 %s 已開始下載。\n", modelName)
	return nil
}
