package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/asccclass/pcai/tools"
	"github.com/ollama/ollama/api"
)

// 設定記憶視窗大小 (只保留最新的 N 則訊息 + 系統提示詞)
const MaxWindowSize = 10 // 約 5 組對話 (User + AI)

// ChatSession 用來保存對話狀態
type ChatSession struct {
	Client   *api.Client
	History  []api.Message // 這是核心：用來儲存所有的對話紀錄
	Registry *tools.ToolRegistry
}

// pruneHistory 負責修剪歷史紀錄，僅保留 System Prompt + 最新的 MaxWindowSize 則訊息
func (s *ChatSession) pruneHistory() {
	// 如果訊息總數 (含 System) 沒有超過限制，就不需要修剪
	if len(s.History) <= MaxWindowSize+1 {
		return
	}

	// 始終保留第一個 (System Prompt)
	systemPrompt := s.History[0]

	// 取得最新的 MaxWindowSize 則訊息
	// History: [System, Old1, Old2, ..., New1, New2, New3]
	// 我們要: [System, New1, New2, New3]
	startIndex := len(s.History) - MaxWindowSize
	recentMessages := s.History[startIndex:]

	// 重組 History
	newHistory := make([]api.Message, 0, len(recentMessages)+1)
	newHistory = append(newHistory, systemPrompt)
	newHistory = append(newHistory, recentMessages...)

	s.History = newHistory
	fmt.Printf("🧹 History pruned. Current size: %d (System + %d recent messages)\n", len(s.History), len(recentMessages))
}

// Chat 負責發送訊息並更新歷史紀錄，且自動處理 Tool Calls
func (s *ChatSession) Chat(userQuery string) (string, error) {
	// 1. 將使用者的問題加入歷史紀錄
	s.History = append(s.History, api.Message{
		Role:    "user",
		Content: userQuery,
	})

	// 2. 修剪歷史紀錄 (滑動視窗)
	s.pruneHistory()

	ctx := context.Background()
	stream := false

	// 使用迴圈處理可能的多次 Tool Call (AI -> Tool -> AI -> Tool ...)
	for {
		req := &api.ChatRequest{
			Model:    os.Getenv("AIModel"),
			Messages: s.History, // 重點：把整串歷史傳給 Ollama
			Tools:    s.Registry.GetDefinitions(),
			Stream:   &stream,
		}

		// 3. 發送請求
		var resp api.ChatResponse
		err := s.Client.Chat(ctx, req, func(r api.ChatResponse) error {
			resp = r
			return nil
		})
		if err != nil {
			return "", err
		}

		// 4. 將 AI 的回答也加入歷史紀錄
		s.History = append(s.History, resp.Message)

		// 5. 檢查是否有 Tool Calls
		if len(resp.Message.ToolCalls) > 0 {
			fmt.Println("🤖 AI Using Tools (Round " + fmt.Sprint(len(s.History)) + ")...")
			for _, toolCall := range resp.Message.ToolCalls {
				fmt.Printf("   -> %s\n", toolCall.Function.Name)

				// 準備參數
				argsBytes, err := json.Marshal(toolCall.Function.Arguments)
				if err != nil {
					fmt.Printf("Error marshaling args: %v\n", err)
					continue
				}

				// 執行工具
				result, err := s.Registry.Execute(toolCall.Function.Name, string(argsBytes))
				if err != nil {
					result = fmt.Sprintf("Error executing tool %s: %v", toolCall.Function.Name, err)
				}

				fmt.Printf("      Result: %s\n", result) // 顯示工具回傳結果

				// 將工具執行結果加入歷史紀錄
				s.History = append(s.History, api.Message{
					Role:    "tool",
					Content: result,
				})
			}
			// 執行完工具後，迴圈會繼續，讓 AI 看到工具結果並產生新的回應
			continue
		}

		// 若沒有 Tool Calls，表示 AI 已經完成回答，回傳內容
		return resp.Message.Content, nil
	}
}

// NewChatSession 初始化一個新的對話，並設定好系統提示詞
func NewChatSession(registry *tools.ToolRegistry) (*ChatSession, error) {
	client, err := api.ClientFromEnvironment()
	if err != nil {
		return nil, fmt.Errorf("無法連接 Ollama: %v", err)
	}

	// 取得 Modelfile 定義的 System Prompt (這裡簡化直接寫在程式碼，或是讀取 Modelfile)
	// 假設我們希望有一個統一的 System Prompt
	systemPrompt := "你是一個專業的繁體中文 AI 助手。無論使用者用什麼語言提問，你都必須使用台灣繁體中文（Traditional Chinese, Taiwan）進行回答。請語氣親切、專業。"

	initialHistory := []api.Message{
		{
			Role:    "system",
			Content: systemPrompt,
		},
	}

	return &ChatSession{
		Client:   client,
		History:  initialHistory,
		Registry: registry,
	}, nil
}
