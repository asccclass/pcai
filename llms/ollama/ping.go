package ollama

import (
	"net/http"
	"time"
)

// GetPingMs 測試 Ollama 伺服器延遲，回傳毫秒數
func GetPingMs(url string) (int64, bool) {
	client := http.Client{
		Timeout: 2 * time.Second, // 設定 2 秒逾時
	}

	start := time.Now()
	resp, err := client.Get(url + "/api/tags")
	if err != nil {
		return 0, false
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		elapsed := time.Since(start).Milliseconds()
		return elapsed, true
	}

	return 0, false
}
