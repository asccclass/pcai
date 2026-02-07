// 工具的實例化集中處理，這樣主程式只要呼叫 tools.Init() 即可
package tools

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/asccclass/pcai/internal/channel"
	"github.com/asccclass/pcai/internal/config"
	"github.com/asccclass/pcai/internal/core"
	"github.com/asccclass/pcai/internal/database"
	"github.com/asccclass/pcai/internal/gateway"
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
var DefaultRegistry = core.NewRegistry()

// Init 初始化工具註冊表
func InitRegistry(bgMgr *BackgroundManager) *core.Registry {
	home, _ := os.Getwd()
	// 載入設定
	cfg := config.LoadConfig()

	// 建立 Ollama API 客戶端
	// [FIX] 強制使用 PCAI_OLLAMA_URL 覆蓋 OLLAMA_HOST，確保連線到正確的遠端伺服器
	if pcaiURL := os.Getenv("PCAI_OLLAMA_URL"); pcaiURL != "" {
		os.Setenv("OLLAMA_HOST", pcaiURL)
	}

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

	// 初始化排程管理器(Hybrid Manager)
	myBrain := heartbeat.NewPCAIBrain(sqliteDB, cfg.OllamaURL, cfg.Model)

	schedMgr := scheduler.NewManager(myBrain, sqliteDB)

	// 註冊 Cron 類型的任務 (週期性), 這裡定義 LLM 可以觸發的背景動作
	schedMgr.RegisterTaskType("read_email", func() {
		gmailCfg := gmail.FilterConfig{
			AllowedSenders: []string{"edu.tw", "service", "justgps", "andyliu"},
			KeyPhrases:     []string{"通知", "重要", "會議"},
			MaxResults:     5,
		}
		// 重構後：使用 Skill 層的 Adapter
		myGmailSkill := skills.NewGmailSkill(client, cfg.Model, cfg.TelegramToken, cfg.TelegramAdminID)
		myGmailSkill.Execute(gmailCfg)
	})
	schedMgr.RegisterTaskType("backup_knowledge", func() {
		msg, err := AutoBackupKnowledge()
		if err != nil {
			log.Printf("Backup failed: %v", err)
		} else {
			log.Println(msg)
		}
	})

	// 建立 Skills
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

	// 檔案系統管理器，設定 "Sandbox" 根目錄
	workspacePath := os.Getenv("WORKSPACE_PATH")
	log.Printf("[Init] WORKSPACE_PATH env: '%s'", workspacePath)
	if workspacePath == "" {
		workspacePath = home
		log.Printf("[Init] WORKSPACE_PATH is empty, defaulting to home: %s", home)
	}
	fsManager, err := NewFileSystemManager(workspacePath)
	if err != nil {
		log.Fatalf("無法初始化檔案系統: %v", err)
	}

	// 初始化並註冊工具
	registry := core.NewRegistry()

	// 注入工具執行器到大腦
	myBrain.SetTools(registry)

	// 基礎工具
	registry.Register(&ShellExecTool{Mgr: bgMgr, Manager: fsManager}) // 傳入背景管理器 與 Sandbox Manager
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

	// 註冊檔案系統工具
	registry.Register(&FsMkdirTool{Manager: fsManager})
	registry.Register(&FsWriteFileTool{Manager: fsManager})
	registry.Register(&FsListDirTool{Manager: fsManager})
	registry.Register(&FsRemoveTool{Manager: fsManager})
	registry.Register(&FsReadFileTool{
		Manager:     fsManager,
		MaxReadSize: 32 * 1024, // 預設 32KB
	})
	registry.Register(&FsAppendFileTool{Manager: fsManager})

	// --- 可繼續新增：相關技能工具 ---
	// 新增 Advisor Skill
	advisorSkill := skills.NewAdvisorSkill(client, cfg.Model)
	registry.Register(advisorSkill.CreateTool())

	// [NEW] 載入動態技能 (skills.md)
	skillsDir := filepath.Join(home, "skills")
	dynamicSkills, err := skills.LoadSkills(skillsDir)
	if err != nil {
		log.Printf("[Skills] 無法載入 skills.md: %v", err)
	} else {
		for _, ds := range dynamicSkills {
			toolStr := skills.NewDynamicTool(ds)
			registry.Register(toolStr)
			log.Printf("[Skills] 已註冊動態技能: %s", toolStr.Name())
		}
	}

	// 新增 Skill 腳手架建立工具 (Meta-Tool)
	registry.Register(&CreateSkillTool{})

	// --- 新增：Telegram 整合 ---
	// [FIX] 移動到這裡，確保 registry 已經註冊完所有工具
	if cfg.TelegramToken != "" {
		// 1. 建立 Agent Adapter (支援多用戶 Session)
		// 注意：這裡改成使用傳入區域變數 registry (已經包含所有工具)
		adapter := gateway.NewAgentAdapter(registry, cfg.Model, cfg.SystemPrompt, cfg.TelegramDebug)

		// 2. 建立 Dispatcher
		dispatcher := gateway.NewDispatcher(adapter, cfg.TelegramAdminID)

		// 3. 建立 Telegram Channel
		tgChannel, err := channel.NewTelegramChannel(cfg.TelegramToken)
		if err != nil {
			log.Printf("⚠️ 無法啟動 Telegram Channel: %v", err)
		} else {
			// 4. 啟動監聽 (非同步)
			go tgChannel.Listen(dispatcher.HandleMessage)
			log.Println("✅ Telegram Channel 已啟動並連接至 Gateway")
		}
	}

	return registry
}
