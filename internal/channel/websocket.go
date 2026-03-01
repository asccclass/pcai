package channel

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/gorilla/websocket"
)

// WSMessage 定義與中繼伺服器通訊的 JSON 格式
type WSMessage struct {
	Channel string `json:"channel"`
	UserID  string `json:"user_id"`
	Message string `json:"message"`
	Type    string `json:"type"`
}

// WebSocketChannel 實作 WebSocket 客戶端適配器
type WebSocketChannel struct {
	url         string
	conn        *websocket.Conn
	stopContext context.Context
	cancel      context.CancelFunc
}

// NewWebSocketChannel 初始化 WebSocket Channel
func NewWebSocketChannel(url string) (*WebSocketChannel, error) {
	ctx, cancel := context.WithCancel(context.Background())
	return &WebSocketChannel{
		url:         url,
		stopContext: ctx,
		cancel:      cancel,
	}, nil
}

// Listen 啟動 WebSocket 監聽
func (w *WebSocketChannel) Listen(handler func(Envelope)) {
	reconnectDelay := 2 * time.Second

	for {
		// 檢查是否已停止
		select {
		case <-w.stopContext.Done():
			fmt.Println("🛑 [WebSocket] 頻道已停止")
			return
		default:
		}

		fmt.Printf("🔄 [WebSocket] 嘗試連線至 %s ...\n", w.url)
		conn, _, err := websocket.DefaultDialer.Dial(w.url, nil)
		if err != nil {
			log.Printf("⚠️ [WebSocket] 連線失敗: %v，%v 後重試...", err, reconnectDelay)
			time.Sleep(reconnectDelay)
			if reconnectDelay < 30*time.Second {
				reconnectDelay *= 2
			}
			continue
		}

		// 連線成功
		w.conn = conn
		reconnectDelay = 2 * time.Second // 重置重試時間
		fmt.Println("✅ [WebSocket] 頻道已連線，監聽中...")

		// 開啟一個 goroutine 來 ping 保持連線活絡 (如果有需要)
		go func(c *websocket.Conn) {
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-w.stopContext.Done():
					return
				case <-ticker.C:
					if err := c.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(10*time.Second)); err != nil {
						log.Printf("⚠️ [WebSocket] Ping 失敗: %v", err)
						return
					}
				}
			}
		}(conn)

		// 收訊迴圈
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				log.Printf("⚠️ [WebSocket] 讀取錯誤: %v", err)
				break // 跳出內部迴圈，觸發重連
			}

			// 解析 JSON
			var wsMsg WSMessage
			if err := json.Unmarshal(message, &wsMsg); err != nil {
				log.Printf("⚠️ [WebSocket] JSON 解析失敗: %v，原始訊息: %s", err, string(message))
				continue
			}

			// 我們只處理 channel 為 "pcai" 的訊息
			// 忽略回覆類型的訊息，避免自己收到自己發送的訊息導致無限迴圈
			if wsMsg.Channel != "pcai" || wsMsg.Type == "response" {
				continue
			}

			if wsMsg.Message == "" || wsMsg.UserID == "" {
				continue // 忽略空訊息
			}

			// 將訊息封裝給 Dispatcher
			env := Envelope{
				SenderID: wsMsg.UserID,
				Content:  wsMsg.Message,
				Platform: "websocket",
				Reply: func(text string) error {
					replyMsg := WSMessage{
						Channel: "pcai", // 回傳時也帶上 channel
						UserID:  wsMsg.UserID,
						Message: text,
						Type:    "response",
					}
					replyData, err := json.Marshal(replyMsg)
					if err != nil {
						return err
					}
					// 透過現存連線回傳 (需注意併發寫入問題)
					return w.conn.WriteMessage(websocket.TextMessage, replyData)
				},
				MarkProcessing: func() error {
					// WebSocket 視伺服器定義，這裡可以留空或發送一個 typing 狀態
					return nil
				},
			}

			// 丟給 Dispatcher 處理 (Dispatcher 會啟動 goroutine)
			go handler(env)
		}

		conn.Close()
		w.conn = nil
	}
}

// Stop 停止 WebSocket 連線
func (w *WebSocketChannel) Stop() {
	if w.cancel != nil {
		fmt.Println("🛑 [WebSocket] 停止連線中...")
		w.cancel()
		if w.conn != nil {
			w.conn.Close()
		}
	}
}
