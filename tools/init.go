// 工具的實例化集中處理，這樣主程式只要呼叫 tools.Init() 即可
package tools

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/asccclass/pcai/internal/agent"
	"github.com/asccclass/pcai/internal/channel"
	"github.com/asccclass/pcai/internal/config"
	"github.com/asccclass/pcai/internal/core"
	"github.com/asccclass/pcai/internal/database"
	"github.com/asccclass/pcai/internal/gateway"
	"github.com/asccclass/pcai/internal/heartbeat"
	"github.com/asccclass/pcai/internal/history"
	"github.com/asccclass/pcai/internal/memory"
	"github.com/asccclass/pcai/internal/scheduler"
	"github.com/asccclass/pcai/llms/ollama"
	"github.com/asccclass/pcai/skills"
	dclient "github.com/docker/docker/client"
	"github.com/go-resty/resty/v2"
	"github.com/ollama/ollama/api"
)

// SyncMemory 讀取 Markdown 檔案，將「新出現」的內容加入記憶庫
func SyncMemory(mem *memory.Manager, filePath string) {
	fmt.Printf("  ↳ [Sync] 正在檢查檔案變更: %s ...\n", filePath)

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
				fmt.Printf("    ↳ [New] 正在嵌入: %s...\n", content[:10])
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
		fmt.Printf("  ↳ [Sync] 同步完成，新增了 %d 筆記憶。\n", newCount)
	} else {
		fmt.Println("  ↳ [Sync] 檔案無變更，記憶庫已是最新狀態。")
	}
}

// 全域註冊表實例
var DefaultRegistry = core.NewRegistry()

// InitRegistry 初始化工具註冊表
// InitRegistry 初始化工具註冊表, 回傳 Registry 和 Cleanup Function
func InitRegistry(bgMgr *BackgroundManager, cfg *config.Config, logger *agent.SystemLogger, onAsyncEvent func()) (*core.Registry, func()) {
	home, _ := os.Getwd() // 程式碼根目錄

	// 建立 Ollama API 客戶端
	if pcaiURL := os.Getenv("OLLAMA_HOST"); pcaiURL == "" {
		fmt.Printf("⚠️ [InitRegistry] OLLAMA_HOST is empty, please set it in envfile\n")
	}
	client, err := api.ClientFromEnvironment() // ollama client 使用 OLLAMA_HOST 作為環境變數
	if err != nil {
		fmt.Printf("⚠️ [InitRegistry] OLLAMA_HOST is empty, please set it in envfile\n")
	}

	dbPath := filepath.Join(home, "botmemory", "pcai.db")
	sqliteDB, err := database.NewSQLite(dbPath)
	if err != nil {
		fmt.Printf("⚠️ [InitRegistry] 無法啟動資料庫: %v\n", err)
	}
	// Note: We do NOT close the DB here because it needs to persist for the lifetime of the application.
	// defer sqliteDB.Close()

	// 初始化排程管理器(Hybrid Manager)
	myBrain := heartbeat.NewPCAIBrain(sqliteDB, cfg.OllamaURL, cfg.Model, cfg.TelegramToken, cfg.TelegramAdminID)
	schedMgr := scheduler.NewManager(myBrain, sqliteDB)
	if onAsyncEvent != nil {
		schedMgr.OnCompletion = onAsyncEvent // 當排程任務完成輸出後，恢復提示符
	}

	// 註冊 Cron 類型的任務 (週期性), 這裡定義 LLM 可以觸發的背景動作

	// 定期檢查行事曆變動 (每小時)
	schedMgr.RegisterTaskType("calendar_watcher", func() {
		watcher := skills.NewCalendarWatcherSkill(cfg.TelegramToken, cfg.TelegramAdminID)
		watcher.Execute(7) // 檢查未來 7 天
	})
	// 預設加入排程 (如果 db 沒資料)
	// 注意：這裡只註冊 Type，實際排程由 DB 或使用者設定。
	// 但為了符合需求 "主動通知"，我們應該在這裡確保它會跑。
	// 由於 schedMgr.LoadJobs() 會載入 DB，如果 DB 沒這 job，我們得 add 一個。
	// 這裡簡單做：在 init 時檢查是否已存在，若無則加入?
	// 或者直接寫死在 Code 裡讓它是 "System Task" (不存 DB)?
	// 目前架構是 Hybrid，這裡註冊 TaskType，然後用 CronSchedule 加入。

	schedMgr.RegisterTaskType("backup_knowledge", func() {
		msg, err := AutoBackupKnowledge()
		if err != nil {
			log.Printf("Backup failed: %v", err)
		} else {
			log.Println(msg)
		}
	})

	// 註冊每日簡報任務 (可以是 read_calendars 或 daily_calendar_report)
	schedMgr.RegisterTaskType("read_calendars", func() {
		watcher := skills.NewCalendarWatcherSkill(cfg.TelegramToken, cfg.TelegramAdminID)
		if briefing, err := watcher.GenerateDailyBriefing(client, cfg.Model); err != nil {
			log.Printf("[Scheduler] Daily briefing failed: %v", err)
		} else {
			// 直接發送
			fmt.Println("✅ [Scheduler] 發送每日行事曆簡報...")
			if cfg.TelegramToken != "" && cfg.TelegramAdminID != "" {
				resty.New().R().
					SetBody(map[string]string{
						"chat_id":    cfg.TelegramAdminID,
						"text":       briefing,
						"parse_mode": "Markdown",
					}).
					Post(fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", cfg.TelegramToken))
			}
		}
	})

	schedMgr.RegisterTaskType("daily_calendar_report", func() {
		watcher := skills.NewCalendarWatcherSkill(cfg.TelegramToken, cfg.TelegramAdminID)
		if briefing, err := watcher.GenerateDailyBriefing(client, cfg.Model); err != nil {
			log.Printf("[Scheduler] Daily briefing failed: %v", err)
		} else {
			// 直接發送 (GenerateDailyBriefing 內部只存檔，不發送？不，它有 return string，我們這裡發送)
			// 但 GenerateDailyBriefing 應該只負責產生和存檔，發送交給上面？
			// 不，為了簡單，我们在這裡調用 sendTelegram // wait, Watcher struct has private sendTelegram.
			// Let's modify GenerateDailyBriefing to return string, and we send it here using helper?
			// Or better, let Watcher handle sending if we expose it?
			// Actually, I added a private `sendTelegram` to Watcher.
			// I should probably make `GenerateDailyBriefing` send it too, or expose `SendTelegram`.
			// Let's assume for now I will use the return string to send via `dispatcher` or just implement a sender here.
			// Oops, `calendar_watcher_skill.go` already implemented `sendTelegram` but it is private.
			// Let's reuse the internal config to send.

			// Packer: I can create a new adapter/notifier here.
			// Or simply:
			fmt.Println("✅ [Scheduler] 發送每日簡報...")
			// watcher.SendTelegram(briefing) // if exposed.
			// Since I cannot change `watcher` easily in this block without another edit,
			// I will use a simple REST call here or assume watcher does it?
			// Wait, the plan said "Sends result to Telegram".

			// Let's implement sending here using the existing variables.
			if cfg.TelegramToken != "" && cfg.TelegramAdminID != "" {
				resty.New().R().
					SetBody(map[string]string{
						"chat_id":    cfg.TelegramAdminID,
						"text":       briefing,
						"parse_mode": "Markdown",
					}).
					Post(fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", cfg.TelegramToken))
			}
		}
	})

	// 註冊行程提醒任務 (每 5 分鐘執行)
	schedMgr.RegisterTaskType("calendar_notifier", func() {
		watcher := skills.NewCalendarWatcherSkill(cfg.TelegramToken, cfg.TelegramAdminID)
		if err := watcher.CheckUpcoming(30 * time.Minute); err != nil {
			log.Printf("[Scheduler] Calendar notifier check failed: %v", err)
		}
	})

	// 建立 Skills
	// 在所有 TaskType 註冊完成後，才載入資料庫中的排程
	if err := schedMgr.LoadJobs(); err != nil {
		fmt.Printf("⚠️ [Scheduler] Failed to load persistent jobs: %v\n", err)
	}

	// 初始化記憶體管理器 (RAG)
	// 定義路徑
	kbDir := filepath.Join(home, "botmemory", "knowledge")
	jsonPath := filepath.Join(kbDir, "memory_store.json") // 向量資料庫
	mdPath := filepath.Join(kbDir, "knowledge.md")        // 原始 Markdown 檔案

	// 建立 Embedder
	embedder := memory.NewOllamaEmbedder(os.Getenv("OLLAMA_HOST"), "mxbai-embed-large")

	// 建立 Manager
	memManager := memory.NewManager(jsonPath, embedder)

	// 建立 PendingStore (暫存待確認記憶，30 分鐘過期)
	pendingStore := memory.NewPendingStore(30 * time.Minute)

	// SyncMemory 應該讀取 Markdown 檔案，而不是 JSON 檔案
	fmt.Println("✅ [Scheduler] 正在初始化記憶庫同步...")
	SyncMemory(memManager, mdPath)

	// 1. 初始化記憶模組
	memorySkillsDir := filepath.Join(home, "skills", "memory_skills")
	// Ensure dir exists
	_ = os.MkdirAll(memorySkillsDir, 0755)

	memSkillMgr := memory.NewSkillManager(memorySkillsDir)
	if err := memSkillMgr.LoadSkills(); err != nil {
		fmt.Printf("⚠️ [Memory] Failed to load memory skills: %v\n", err)
	}

	// Wrapper for ChatStream to match LLMProvider signature
	memExecutor := memory.NewMemoryExecutor(ollama.ChatStream, cfg.Model)

	memController := memory.NewController(memManager, memSkillMgr, memExecutor)

	// Inject into history package
	history.GlobalMemoryController = memController
	fmt.Printf("✅ [Memory] Controller initialized with %d skills\n", len(memSkillMgr.Skills))

	// 檔案系統管理器，設定 "Sandbox" 根目錄
	workspacePath := os.Getenv("WORKSPACE_PATH")
	if workspacePath == "" {
		workspacePath = home
		log.Printf("⚠️ [Init] WORKSPACE_PATH is empty, defaulting to home: %s", home)
	}
	fmt.Printf("✅ [Init] Set WORKSPACE_PATH env is: '%s'\n", workspacePath)
	// 讀取工具白名單字串
	envTools := os.Getenv("PCAI_ENABLED_TOOLS")
	var enabledTools []string
	if envTools != "" {
		// 將 "fs_read_file,fs_list_dir" 拆解為 slice
		rawList := strings.Split(envTools, ",")
		for _, t := range rawList {
			// 清除可能存在的空格 (例如 " fs_read_file" -> "fs_read_file")
			trimmed := strings.TrimSpace(t)
			if trimmed != "" {
				enabledTools = append(enabledTools, trimmed)
			}
		}
	}
	// 初始化檔案管理員
	fsManager, err := NewFileSystemManager(workspacePath)
	if err != nil {
		log.Fatalf("⚠️ 無法初始化檔案系統: %v", err)
	}
	// 根據白名單載入工具

	// 初始化並註冊工具
	registry := core.NewRegistry()

	// 基礎工具
	registry.Register(&ShellExecTool{Mgr: bgMgr, Manager: fsManager}) // 傳入背景管理器 與 Sandbox Manager
	registry.Register(&KnowledgeSearchTool{})
	registry.Register(&WebFetchTool{})
	registry.Register(&WebSearchTool{})
	registry.Register(&ListTasksTool{Mgr: bgMgr, SchedMgr: schedMgr}) // 傳入背景管理器與排程管理器
	registry.Register(&ListSkillsTool{Registry: registry})            // 列出所有技能
	registry.Register(&KnowledgeAppendTool{})
	registry.Register(&VideoConverterTool{})
	registry.Register(&EmailTool{})
	registry.Register(NewGoogleTool())
	registry.Register(&GitAutoCommitTool{}) // Git 自動提交工具

	// Python Sandbox Tool
	if pyTool, err := NewPythonSandboxTool(workspacePath, home); err != nil {
		fmt.Printf("⚠️ [Tools] Python Sandbox not available: %v\n", err)
	} else {
		registry.Register(pyTool)
	}

	// 記憶相關工具
	registry.Register(NewMemoryTool(memManager))                              // 搜尋工具
	registry.Register(NewMemorySaveTool(memManager, pendingStore, mdPath))    // 儲存工具 (暫存待確認)
	registry.Register(NewMemoryConfirmTool(memManager, pendingStore, mdPath)) // 確認/拒絕工具
	// 遺忘工具 (注入 memManager, schedMgr, mdPath)	// 這樣它才能同時操作資料庫並排程刪除檔案
	registry.Register(NewMemoryForgetTool(memManager, schedMgr, mdPath))

	// 排程工具 (讓 LLM 可以設定 Cron)
	registry.Register(&SchedulerTool{Mgr: schedMgr})

	// 任務規劃工具
	registry.Register(NewPlannerTool())

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
	// 新增 Advisor Skill (高優先級)
	advisorSkill := skills.NewAdvisorSkill(client, cfg.Model)
	registry.RegisterWithPriority(advisorSkill.CreateTool(), 10)

	// [NEW] 載入動態技能 (skills.md)
	// 初始化 Docker Client (分享給所有 Dynamic Skills)
	dockerCli, err := dclient.NewClientWithOpts(dclient.FromEnv, dclient.WithAPIVersionNegotiation())
	if err != nil {
		log.Printf("⚠️ [Skills] 無法初始化 Docker Client: %v (Sidecar 模式將無法使用)", err)
		dockerCli = nil
	}

	skillsDir := filepath.Join(home, "skills")

	// 初始化 SkillManager (負責持久化與 Registry 載入)
	skillRegistryPath := filepath.Join(home, "botmemory", "skillregistry.json")
	skillManager := NewSkillManager(skillsDir, skillRegistryPath, registry, dockerCli)

	// 1. 從 Registry 回憶已安裝的技能 (持久化清單)
	if err := skillManager.LoadAll(); err != nil {
		log.Printf("⚠️ [Skills] LoadAll failed: %v", err)
	}

	// 2. 掃描目錄載入手動新增的 SKILL.md (向下相容)
	dynamicSkills, err := skills.LoadSkills(skillsDir)
	if err != nil {
		log.Printf("⚠️ [Skills] 無法載入 skills.md: %v", err)
	} else {
		for _, ds := range dynamicSkills {
			toolStr := skills.NewDynamicTool(ds, registry, dockerCli)
			registry.RegisterWithPriority(toolStr, 10) // Skills 優先於 Tools
			fmt.Printf("✅ [Skills] Loaded (priority): %s (%s)\n", ds.Name, ds.Description)
		}
	}

	// 新增 Skill 腳手架建立工具 (Meta-Tool)
	registry.Register(&CreateSkillTool{})

	// 註冊 GitHub Skill Installer
	registry.Register(&SkillInstaller{
		Manager: skillManager,
		BaseDir: skillsDir,
	})

	// --- 新增：Telegram 整合 ---
	var tgChannel *channel.TelegramChannel // 宣告在外部以供 cleanup 存取

	// [FIX] 移動到這裡，確保 registry 已經註冊完所有工具
	if cfg.TelegramToken != "" {
		// 1. 建立 Agent Adapter
		adapter := gateway.NewAgentAdapter(registry, cfg.Model, cfg.SystemPrompt, cfg.TelegramDebug, logger)

		// 2. 建立 Dispatcher
		dispatcher := gateway.NewDispatcher(adapter, cfg.TelegramAdminID)
		if onAsyncEvent != nil {
			dispatcher.OnCompletion = onAsyncEvent
		}

		// 3. 建立 Telegram Channel
		var err error
		tgChannel, err = channel.NewTelegramChannel(cfg.TelegramToken, cfg.TelegramDebug)
		if err != nil {
			log.Printf("⚠️ 無法啟動 Telegram Channel: %v", err)
		} else {
			// 4. 啟動監聽 (非同步)
			go tgChannel.Listen(dispatcher.HandleMessage)
			// log.Println("✅ Telegram Channel 已啟動並連接至 Gateway") // Listen 內部會印
		}
	}

	// 注入工具執行器到大腦
	myBrain.SetTools(registry)

	// 建立 Cleanup Function
	cleanup := func() {
		if tgChannel != nil {
			tgChannel.Stop()
		}
	}

	return registry, cleanup
}
