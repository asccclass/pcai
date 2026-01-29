package ollama

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

type Request struct {
	Model   string `json:"model"`
	Prompt  string `json:"prompt"`
	System  string `json:"system,omitempty"`
	Stream  bool   `json:"stream"`
	Context []int  `json:"context,omitempty"` // 加入 Context 支援
}

type ResponseChunk struct {
	Response string `json:"response"`
	Done     bool   `json:"done"`
	Context  []int  `json:"context"` // 最終塊會回傳新的 Context
}

func ChatStream(modelName, systemPrompt, userPrompt string, prevContext []int, callback func(string)) ([]int, error) {
	reqBody := Request{
		Model:   modelName,
		Prompt:  userPrompt,
		System:  systemPrompt,
		Stream:  true,
		Context: prevContext,
	}

	jsonData, _ := json.Marshal(reqBody)

	// 增加檢查：確認請求位址
	resp, err := http.Post("http://172.18.124.210:11434/api/generate", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("連線至 Ollama 失敗: %v (請檢查 Ollama 是否啟動)", err)
	}
	defer resp.Body.Close()

	// 檢查 HTTP 狀態碼 (例如 404 代表模型不存在)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Ollama 回傳錯誤碼: %d (請檢查模型名稱 '%s' 是否正確)", resp.StatusCode, modelName)
	}

	var lastContext []int
	scanner := bufio.NewScanner(resp.Body)

	hasData := false
	for scanner.Scan() {
		hasData = true
		var chunk ResponseChunk
		if err := json.Unmarshal(scanner.Bytes(), &chunk); err != nil {
			continue
		}
		callback(chunk.Response)
		if chunk.Done {
			lastContext = chunk.Context
			break
		}
	}

	if !hasData {
		return nil, fmt.Errorf("API 成功連線但沒有收到任何資料流")
	}

	return lastContext, nil
}
