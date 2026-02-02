// 工具的實例化集中處理，這樣主程式只要呼叫 tools.Init() 即可
package tools

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/asccclass/pcai/internal/gmail"
	"github.com/asccclass/pcai/internal/memory"
	"github.com/asccclass/pcai/internal/scheduler"
	"github.com/ollama/ollama/api"
)

// SyncMemory 讀取 Markdown 檔案，將「新出現」的內容加入記憶庫
func SyncMemory(mem *memory.Manager, filePath string) {
	fmt.Printf("[Sync] 正在檢查檔案變更: %s ...\n", filePath)

	file, err := os.Open(filePath)
	if os.IsNotExist(err) {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var buffer strings.Builder
	newCount := 0

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			content := strings.TrimSpace(buffer.String())

			// 關鍵修改：先檢查 Exists，不存在才 Add
			if content != "" && !mem.Exists(content) {
				fmt.Printf(" [發現新資料] 正在嵌入: %s...\n", content[:10])
				err := mem.Add(content, []string{"file_sync"})
				if err != nil {
					fmt.Println("嵌入失敗:", err)
				} else {
					newCount++
				}
			}
			buffer.Reset()
		} else {
			buffer.WriteString(line + "\n")
		}
	}
	// 處理最後一段
	if buffer.Len() > 0 {
		content := strings.TrimSpace(buffer.String())
		if content != "" && !mem.Exists(content) {
			mem.Add(content, []string{"file_sync"})
			newCount++
		}
	}

	if newCount > 0 {
		fmt.Printf("[Sync] 同步完成，新增了 %d 筆記憶。\n", newCount)
	} else {
		fmt.Println("[Sync] 檔案無變更，記憶庫已是最新狀態。")
	}
}

// 全域註冊表實例
var DefaultRegistry = NewRegistry()

// Init 初始化工具註冊表
func InitRegistry(bgMgr *BackgroundManager) *Registry {
	home, _ := os.Getwd()
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
	registry.Register(&KnowledgeAppendTool{})
	// --- 可繼續新增：相關技能工具 ---
	// 初始化記憶體管理器
	embedder := memory.NewOllamaEmbedder(os.Getenv("PCAI_OLLAMA_URL"), "mxbai-embed-large")
	dir := filepath.Join(home, "botmemory", "knowledge", "memory_store.json")
	memManager := memory.NewManager(dir, embedder)
	dir = filepath.Join(home, "botmemory", "knowledge", "knowledge.md")
	SyncMemory(memManager, dir)
	mgt := NewMemoryTool(memManager)
	svt := NewMemorySaveTool(memManager, dir)
	forgetTool := NewMemoryForgetTool(memManager)
	registry.Register(mgt)
	registry.Register(svt)
	registry.Register(forgetTool)
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
	schedMgr.RegisterTaskType("backup_knowledge", func() {
		msg, err := AutoBackupKnowledge()
		if err != nil {
			log.Printf("Backup failed: %v", err)
		} else {
			log.Println(msg)
		}
	})

	registry.Register(&SchedulerTool{Mgr: schedMgr})
	registry.Register(&VideoConverterTool{})
	return registry
}
