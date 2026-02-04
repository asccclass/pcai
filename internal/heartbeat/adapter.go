package heartbeat

import (
	"context"
	"fmt"
)

// BrainAdapter 實作 gateway.Processor 介面
// 它負責將 Gateway 收到的文字訊息轉發給 PCAIBrain 處理
type BrainAdapter struct {
	brain *PCAIBrain
}

func NewBrainAdapter(brain *PCAIBrain) *BrainAdapter {
	return &BrainAdapter{brain: brain}
}

// Process 處理來自 Gateway 的訊息
func (a *BrainAdapter) Process(input string) string {
	// 使用背景 Context，或考慮傳入 Context
	ctx := context.Background()

	response, err := a.brain.HandleUserChat(ctx, input)
	if err != nil {
		return fmt.Sprintf("⚠️ 處理訊息時發生錯誤: %v", err)
	}

	return response
}
