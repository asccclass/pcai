// 工具的實例化集中處理，這樣主程式只要呼叫 tools.Init() 即可
package tools

import (
	"log"

	"github.com/asccclass/pcai/internal/gmail"
	"github.com/asccclass/pcai/internal/scheduler"
	"github.com/ollama/ollama/api"
)

// 全域註冊表實例
var DefaultRegistry = NewRegistry()

// Init 初始化工具註冊表
func InitRegistry(bgMgr *BackgroundManager) *Registry {
	// 建立 Ollama API 客戶端
	client, err := api.ClientFromEnvironment()
	if err != nil {
		log.Fatal(err)
	}

	registry := NewRegistry()
	registry.Register(&ListFilesTool{})
	registry.Register(&ShellExecTool{Mgr: bgMgr}) // 傳入背景管理器
	registry.Register(&KnowledgeSearchTool{})
	registry.Register(&FetchURLTool{})
	registry.Register(&ListTasksTool{Mgr: bgMgr}) // 傳入背景管理器
	registry.Register(&KnowledgeAppendTool{})     // 加入這行
	// --- 可繼續新增：相關技能工具 ---
	// 初始化排程管理器
	schedMgr := scheduler.NewManager()
	// 註冊具體執行邏輯 (Task Types), 這裡定義 LLM 可以觸發的背景動作
	schedMgr.RegisterTaskType("read_email", func() {
		cfg := gmail.FilterConfig{
			AllowedSenders: []string{"edu.tw", "service", "justgps", "andyliu"},
			KeyPhrases:     []string{"通知", "重要", "會議"},
			MaxResults:     5,
		}
		gmail.SyncGmailToKnowledge(client, "llama3.3", cfg)
	})
	registry.Register(&SchedulerTool{Mgr: schedMgr})
	return registry
}
