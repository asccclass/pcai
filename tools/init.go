// 工具的實例化集中處理，這樣主程式只要呼叫 tools.Init() 即可
package tools

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/asccclass/pcai/internal/database"
	"github.com/asccclass/pcai/internal/gmail"
	"github.com/asccclass/pcai/internal/heartbeat"
	"github.com/asccclass/pcai/internal/memory"
	"github.com/asccclass/pcai/internal/scheduler"
	"github.com/asccclass/pcai/skills"
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

	dbPath := filepath.Join(home, "botmemory", "pcai.db")
	sqliteDB, err := database.NewSQLite(dbPath)
	if err != nil {
		log.Fatalf("無法啟動資料庫: %v", err)
	}
	// Note: We do NOT close the DB here because it needs to persist for the lifetime of the application.
	// defer sqliteDB.Close() 

	// 2. 初始化大腦 (注入資料庫連線)
	signalURL := "http://localhost:8080/v1/receive/+886912345678"

	// 初始化排程管理器(Hybrid Manager)
	myBrain := heartbeat.NewPCAIBrain(sqliteDB, signalURL)
	// hb := heartbeat.NewHeartbeatProcessor{client: client, tools: bgMgr.tools, memory: bgMgr.memManager}
	schedMgr := scheduler.NewManager(myBrain, sqliteDB)

	// 註冊 Cron 類型的任務 (週期性), 這裡定義 LLM 可以觸發的背景動作
	schedMgr.RegisterTaskType("read_email", func() {
		cfg := gmail.FilterConfig{
			AllowedSenders: []string{"edu.tw", "service", "justgps", "andyliu"},
			KeyPhrases:     []string{"通知", "重要", "會議"},
			MaxResults:     5,
		}
		// 重構後：使用 Skill 層的 Adapter
		myGmailSkill := skills.NewGmailSkill(client, "llama3.3")
		myGmailSkill.Execute(cfg)
	})
	schedMgr.RegisterTaskType("backup_knowledge", func() {
		msg, err := AutoBackupKnowledge()
		if err != nil {
			log.Printf("Backup failed: %v", err)
		} else {
			log.Println(msg)
		}
	})

	// 關鍵修正：在所有 TaskType 註冊完成後，才載入資料庫中的排程
	if err := schedMgr.LoadJobs(); err != nil {
		log.Printf("[Scheduler] Failed to load persistent jobs: %v", err)
	}

	// 初始化記憶體管理器 (RAG)
	// 定義路徑
	kbDir := filepath.Join(home, "botmemory", "knowledge")
	jsonPath := filepath.Join(kbDir, "memory_store.json") // 向量資料庫
	mdPath := filepath.Join(kbDir, "knowledge.md")        // 原始 Markdown 檔案

	// 建立 Embedder
	embedder := memory.NewOllamaEmbedder(os.Getenv("PCAI_OLLAMA_URL"), "mxbai-embed-large")

	// 建立 Manager
	memManager := memory.NewManager(jsonPath, embedder)

	// SyncMemory 應該讀取 Markdown 檔案，而不是 JSON 檔案
	SyncMemory(memManager, mdPath)

	// 初始化並註冊工具
	registry := NewRegistry()

	// 基礎工具
	registry.Register(&ListFilesTool{})
	registry.Register(&ShellExecTool{Mgr: bgMgr}) // 傳入背景管理器
	registry.Register(&KnowledgeSearchTool{})
	registry.Register(&FetchURLTool{})
	registry.Register(&ListTasksTool{Mgr: bgMgr, SchedMgr: schedMgr}) // 傳入背景管理器與排程管理器
	registry.Register(&KnowledgeAppendTool{})
	registry.Register(&VideoConverterTool{})

	// 記憶相關工具
	registry.Register(NewMemoryTool(memManager))             // 搜尋工具
	registry.Register(NewMemorySaveTool(memManager, mdPath)) // 儲存工具 (存入 Markdown)
	// 遺忘工具 (注入 memManager, schedMgr, mdPath)	// 這樣它才能同時操作資料庫並排程刪除檔案
	registry.Register(NewMemoryForgetTool(memManager, schedMgr, mdPath))

	// 排程工具 (讓 LLM 可以設定 Cron)
	registry.Register(&SchedulerTool{Mgr: schedMgr})

	// --- 可繼續新增：相關技能工具 ---
	return registry
}
