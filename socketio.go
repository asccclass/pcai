package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/gorilla/websocket"
)

// StartSignalListener 啟動 Signal 監聽服務
// session: 用於與 AI 進行對話
func StartSignalListener(session *ChatSession) {
	host := os.Getenv("SignalHost")
	if host == "" {
		host = "localhost:8080"
	}
	number := os.Getenv("SignalNumber")
	if number == "" {
		// 如果沒有設定號碼，可以在這裡報錯或使用預設值
		// 為了安全起見，若無號碼則不啟動或提示
		log.Println("Warning: SignalNumber environment variable not set.")
		return
	}

	// 組合 WebSocket URL: ws://<host>/v1/receive/<number>
	url := fmt.Sprintf("wss://%s/v1/receive/%s", host, number)
	fmt.Printf("[Signal] Connecting to: %s\n", url)

	// 建立連線
	c, resp, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		if resp != nil {
			fmt.Printf("握手失敗，伺服器回應狀態碼: %d (%s)\n", resp.StatusCode, resp.Status)
		}
		log.Printf("[Signal] Connection failed: %s\n", err.Error())
		return
	}
	// 注意：這裡是異步啟動，所以在 pcai.go 中要小心不要讓這裡阻塞主程式，
	// 但也不要讓 defer 立即執行。
	// 因為 StartSignalListener 將會被 go routine 呼叫，所以 defer c.Close() 是合理的。
	defer c.Close()

	// 處理優雅關閉 (Ctrl+C) - 雖然 pcai.go 也有，但這裡可以獨立監聽或依賴外部 context
	// 為了簡單起見，我們讓 WebSocket 讀取錯誤自然退出，或是監聽 Interrupt
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	fmt.Println("[Signal] Connected! Listening for messages...")

	// 讀取訊息迴圈
	done := make(chan struct{})

	go func() {
		defer close(done)
		for {
			_, message, err := c.ReadMessage()
			if err != nil {
				log.Println("[Signal] Read error:", err)
				return
			}
			// 處理訊息
			handleSignalMessage(session, message, host, number)
		}
	}()

	// 等待中斷或結束
	select {
	case <-interrupt:
		log.Println("[Signal] Interrupt received, closing connection...")
		c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		select {
		case <-done:
		case <-time.After(time.Second):
		}
	case <-done:
		// 讀取迴圈結束 (可能是連線斷開)
		log.Println("[Signal] Connection closed.")
	}
}

// 解析並處理 Signal 訊息
func handleSignalMessage(session *ChatSession, msgBytes []byte, host, userNumber string) {
	var msg map[string]interface{}
	if err := json.Unmarshal(msgBytes, &msg); err != nil {
		log.Printf("[Signal] JSON unmarshal error: %v", err)
		return
	}

	// 訊息結構通常在 "envelope" -> "dataMessage" -> "message"
	if envelope, ok := msg["envelope"].(map[string]interface{}); ok {
		source, _ := envelope["source"].(string) // 發送者號碼

		// 忽略自己發送的訊息 (如果是同步回來的)
		if source == userNumber {
			return
		}

		if dataMsg, ok := envelope["dataMessage"].(map[string]interface{}); ok {
			text, _ := dataMsg["message"].(string)
			// timestamp := dataMsg["timestamp"]

			if text != "" {
				fmt.Printf("\n[Signal] Received from %s: %s\n", source, text)

				// 1. 呼叫 AI 獲取回應
				// 注意：session.Chat 可能會改變 History，需要注意併發安全。
				// pcai.go 裡面是單執行緒 loop，這裡是在 goroutine。
				// 假設 ChatSession 的操作本身不是 thread-safe，我們可能需要加鎖。
				// 但為了簡單整合，先假設 pcai 主要在 stdin 等待，這裡偶爾插入。
				// (更嚴謹的做法是使用 channel 將訊息送回 main loop 處理，或替 ChatSession 加 Mutex)

				// 為了避免複雜度，這裡直接呼叫，但在 memory.go 可能需要 Mutex
				// (暫時不做 Mutex，假設使用者不會同時打字和傳 Signal)

				response, err := session.Chat(fmt.Sprintf("[From Signal User %s] %s", source, text))
				if err != nil {
					log.Printf("[Signal] AI Error: %v", err)
					sendSignalMessage(host, userNumber, source, "Sorry, I encountered an error: "+err.Error())
					return
				}

				fmt.Printf("[Signal] AI Reply: %s\n", response)

				// 2. 回傳給 Signal 使用者
				if err := sendSignalMessage(host, userNumber, source, response); err != nil {
					log.Printf("[Signal] Send reply failed: %v", err)
				}
			}
		}
	}
}

// 發送 Signal 訊息 (使用 signal-cli-rest-api 的 HTTP 介面)
func sendSignalMessage(host, userNumber, recipient, message string) error {
	// API: POST /v2/send
	// Body: {"message": "...", "number": "+886...", "recipients": ["+886..."]}

	url := fmt.Sprintf("https://%s/v2/send", host)

	payload := map[string]interface{}{
		"message":    message,
		"number":     userNumber, // 寄件者 (我們)
		"recipients": []string{recipient},
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("HTTP error code: %d", resp.StatusCode)
	}

	return nil
}
