// 撌亙?祕靘??葉??嚗見銝餌?撘閬??tools.Init() ?喳
package tools

import (
	"context"
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
	"github.com/asccclass/pcai/llms"
	"github.com/asccclass/pcai/llms/ollama"
	"github.com/asccclass/pcai/skills"
	browserskill "github.com/asccclass/pcai/skills/browser"
	dclient "github.com/docker/docker/client"
	"github.com/go-resty/resty/v2"
	"github.com/ollama/ollama/api"
)

// GlobalMemoryToolKit ?典?閮撌亙憟辣嚗? history ??雿輻嚗?
var GlobalMemoryToolKit *memory.ToolKit

// GlobalDB ?典? SQLite 鞈?摨怠祕靘?靘???嗆?撠?雿輻嚗?
var GlobalDB *database.DB

// ?典?閮餃?銵典祕靘?
var DefaultRegistry = core.NewRegistry()

// InitRegistry ???極?瑁酉?”
// InitRegistry ???極?瑁酉?”, ? Registry ??Cleanup Function
func InitRegistry(bgMgr *BackgroundManager, cfg *config.Config, logger *agent.SystemLogger, onAsyncEvent func()) (*core.Registry, func()) {
	home, _ := os.Getwd() // 蝔?蝣潭?桅?

	// 撱箇? Ollama API 摰Ｘ蝡?
	if pcaiURL := os.Getenv("OLLAMA_HOST"); pcaiURL == "" {
		fmt.Printf("?? [InitRegistry] OLLAMA_HOST is empty, please set it in envfile\n")
	}
	client, err := api.ClientFromEnvironment() // ollama client 雿輻 OLLAMA_HOST 雿?啣?霈
	if err != nil {
		fmt.Printf("?? [InitRegistry] OLLAMA_HOST is empty, please set it in envfile\n")
	}

	dbPath := filepath.Join(home, "botmemory", "pcai.db")
	sqliteDB, err := database.NewSQLite(dbPath)
	if err != nil {
		fmt.Printf("?? [InitRegistry] ?⊥???鞈?摨? %v\n", err)
	}
	GlobalDB = sqliteDB // 撠靘??其蝙??
	// Note: We do NOT close the DB here because it needs to persist for the lifetime of the application.
	// defer sqliteDB.Close()

	// ????蝔恣?(Hybrid Manager)
	myBrain := heartbeat.NewPCAIBrain(sqliteDB, cfg.OllamaURL, cfg.Model, cfg.TelegramToken, cfg.TelegramAdminID, cfg.LineToken)
	schedMgr := scheduler.NewManager(myBrain, sqliteDB)
	if onAsyncEvent != nil {
		schedMgr.OnCompletion = onAsyncEvent // ?嗆?蝔遙???撓?箏?嚗敺拇?蝷箇泵
	}

	// 閮餃? Cron 憿??遙??(?望???, ?ㄐ摰儔 LLM ?臭誑閫貊???臬?雿?

	// 摰?瑼Ｘ銵?????(瘥???
	schedMgr.RegisterTaskType("calendar_watcher", func() {
		watcher := skills.NewCalendarWatcherSkill(cfg.TelegramToken, cfg.TelegramAdminID)
		watcher.Execute(7) // 瑼Ｘ?芯? 7 憭?
	})
	// ?身??? (憒? db 瘝???
	// 瘜冽?嚗ㄐ?芾酉??Type嚗祕??蝔 DB ?蝙?刻身摰?
	// 雿鈭泵??瘙?"銝餃??"嚗???閰脣?ㄐ蝣箔?摰?頝?
	// ?望 schedMgr.LoadJobs() ????DB嚗???DB 瘝?job嚗??? add 銝??
	// ?ㄐ蝪∪????init ?炎?交?血歇摮嚗?∪???
	// ??亙神甇餃 Code 鋆∟?摰 "System Task" (銝? DB)?
	// ?桀??嗆???Hybrid嚗ㄐ閮餃? TaskType嚗敺 CronSchedule ???

	schedMgr.RegisterTaskType("backup_knowledge", func() {
		msg, err := AutoBackupKnowledge()
		if err != nil {
			log.Printf("Backup failed: %v", err)
		} else {
			log.Println(msg)
		}
	})

	// [NEW] 閮餃???批???隞餃? (?蔭?銵?
	schedMgr.RegisterTaskType("personalization_extraction", func() {
		fmt.Println("?? [Personalization] ?????亥?隞交???嗅?憟?..")
		worker := history.NewPersonalizationWorker(filepath.Join(home, "botmemory"), sqliteDB, cfg.Model, func(model, prompt string) (string, error) {
			var resp strings.Builder
			chatFn := llms.GetDefaultChatStream()
			_, err := chatFn(model, []ollama.Message{
				{Role: "system", Content: "雿銝?批???撠振??},
				{Role: "user", Content: prompt},
			}, nil, ollama.Options{Temperature: 0.3}, func(c string) { resp.WriteString(c) })
			return resp.String(), err
		})
		if err := worker.RunOnce(); err != nil {
			log.Printf("?? [Personalization] ??憭望?: %v", err)
		} else {
			fmt.Println("??[Personalization] ??摰?嚗?)
		}
	})

	// ?身瘥?4 撠???銝甈∴????箇頂蝯曹遙??
	if err := schedMgr.EnsureSystemJob("background_personalization", "0 */4 * * *", "personalization_extraction", "摰????亥????冽?末"); err != nil {
		log.Printf("?對? [Scheduler] personalization job: %v", err)
	}

	// 閮餃?瘥蝪∪隞餃? (?臭誑??manage_calendar ??daily_calendar_report)
	schedMgr.RegisterTaskType("manage_calendar", func() {
		watcher := skills.NewCalendarWatcherSkill(cfg.TelegramToken, cfg.TelegramAdminID)
		if briefing, err := watcher.GenerateDailyBriefing(client, cfg.Model); err != nil {
			log.Printf("[Scheduler] Daily briefing failed: %v", err)
		} else {
			// ?湔?潮?
			fmt.Println("??[Scheduler] ?潮??亥?鈭?蝪∪...")
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
			// ?湔?潮?(GenerateDailyBriefing ?折?芸?瑼?銝??銝?摰? return string嚗??ㄐ?潮?
			// 雿?GenerateDailyBriefing ?府?芾?鞎祉??摮?嚗?漱蝯虫??ｇ?
			// 銝??箔?蝪∪嚗?隞砍?ㄐ隤輻 sendTelegram // wait, Watcher struct has private sendTelegram.
			// Let's modify GenerateDailyBriefing to return string, and we send it here using helper?
			// Or better, let Watcher handle sending if we expose it?
			// Actually, I added a private `sendTelegram` to Watcher.
			// I should probably make `GenerateDailyBriefing` send it too, or expose `SendTelegram`.
			// Let's assume for now I will use the return string to send via `dispatcher` or just implement a sender here.
			// Oops, `calendar_watcher_skill.go` already implemented `sendTelegram` but it is private.
			// Let's reuse the internal config to send.

			// Packer: I can create a new adapter/notifier here.
			// Or simply:
			fmt.Println("??[Scheduler] ?潮??亦陛??..")
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

	// 閮餃?銵???隞餃? (瘥?5 ???瑁?)
	schedMgr.RegisterTaskType("calendar_notifier", func() {
		watcher := skills.NewCalendarWatcherSkill(cfg.TelegramToken, cfg.TelegramAdminID)
		if err := watcher.CheckUpcoming(30 * time.Minute); err != nil {
			log.Printf("[Scheduler] Calendar notifier check failed: %v", err)
		}
	})

	// 撱箇? Skills

	// ?????嗥頂蝯?(OpenClaw ToolKit)
	kbDir := filepath.Join(home, "botmemory", "knowledge")
	_ = os.MkdirAll(kbDir, 0750)

	memCfg := memory.MemoryConfig{
		WorkspaceDir: kbDir,
		StateDir:     kbDir,
		AgentID:      "pcai",
		Search: memory.SearchConfig{
			Provider:  "ollama",
			Model:     "mxbai-embed-large",
			OllamaURL: os.Getenv("OLLAMA_HOST"),
			Hybrid: memory.HybridConfig{
				Enabled:             true,
				VectorWeight:        0.7,
				TextWeight:          0.3,
				CandidateMultiplier: 4,
			},
			Cache: memory.CacheConfig{
				Enabled:    true,
				MaxEntries: 50000,
			},
			Sync: memory.SyncConfig{
				Watch: true,
			},
		},
	}

	memToolKit, err := memory.NewToolKit(memCfg)
	if err != nil {
		fmt.Printf("?? [Memory] ToolKit ???仃?? %v\n", err)
	} else {
		GlobalMemoryToolKit = memToolKit
		history.GlobalMemoryToolKit = memToolKit
		fmt.Printf("??[Memory] ToolKit ??????(蝝Ｗ? %d ??chunks)\n", memToolKit.ChunkCount())
	}

	// 瑼?蝟餌絞蝞∠??剁?閮剖? "Sandbox" ?寧??
	workspacePath := os.Getenv("WORKSPACE_PATH")
	if workspacePath == "" {
		workspacePath = home
		log.Printf("?? [Init] WORKSPACE_PATH is empty, defaulting to home: %s", home)
	}
	fmt.Printf("??[Init] Set WORKSPACE_PATH env is: '%s'\n", workspacePath)
	// 霈?極?瑞?摮葡
	envTools := os.Getenv("PCAI_ENABLED_TOOLS")
	var enabledTools []string
	if envTools != "" {
		// 撠?"fs_read_file,fs_list_dir" ?圾??slice
		rawList := strings.Split(envTools, ",")
		for _, t := range rawList {
			// 皜?航摮?征??(靘? " fs_read_file" -> "fs_read_file")
			trimmed := strings.TrimSpace(t)
			if trimmed != "" {
				enabledTools = append(enabledTools, trimmed)
			}
		}
	}
	// ????獢恣?
	fsManager, err := NewFileSystemManager(workspacePath)
	if err != nil {
		log.Fatalf("?? ?⊥?????獢頂蝯? %v", err)
	}
	// ?寞??賢??株??亙極??

	// ???蒂閮餃?撌亙
	registry := core.NewRegistry()

	// ?箇?撌亙
	registry.Register(&ShellExecTool{Mgr: bgMgr, Manager: fsManager}) // ?喳?蝞∠?????Sandbox Manager
	registry.RegisterWithPriority(&WebFetchTool{}, 5)
	registry.RegisterWithPriority(&WebSearchTool{}, 5)
	registry.Register(&ListTasksTool{Mgr: bgMgr, SchedMgr: schedMgr}) // ?喳?蝞∠??刻???蝞∠???
	registry.Register(&ListSkillsTool{Registry: registry})            // ??????
	registry.Register(&VideoConverterTool{})
	registry.Register(&EmailDraftTool{})
	registry.Register(NewGoogleTool())
	registry.Register(&GitAutoCommitTool{}) // Git ?芸??漱撌亙

	botInteractSkill := skills.NewBotInteractSkill()
	registry.Register(botInteractSkill.CreateTool()) // [NEW] Bot-to-Bot ?????

	// Browser Tools
	registry.RegisterWithPriority(&browserskill.BrowserOpenTool{}, 5)
	registry.Register(&browserskill.BrowserSnapshotTool{})
	registry.Register(&browserskill.BrowserClickTool{})
	registry.Register(&browserskill.BrowserTypeTool{})
	registry.Register(&browserskill.BrowserScrollTool{})
	registry.Register(&browserskill.BrowserGetTool{})
	registry.RegisterWithPriority(&browserskill.BrowserGetTextTool{}, 5)

	// Python Sandbox Tool
	if pyTool, err := NewPythonSandboxTool(workspacePath, home); err != nil {
		fmt.Printf("?? [Tools] Python Sandbox not available: %v\n", err)
	} else {
		registry.Register(pyTool)
	}

	// 閮?賊?撌亙嚗蝙?冽 ToolKit API嚗?
	if memToolKit != nil {
		pendingStore := memory.NewPendingStore(30 * time.Minute)

		registry.Register(NewMemoryTool(memToolKit))                      // ??撌亙
		registry.Register(NewMemorySaveTool(memToolKit, pendingStore))    // ?脣?撌亙 (?怠?)
		registry.Register(NewMemoryConfirmTool(memToolKit, pendingStore)) // 蝣箄?撌亙
		registry.Register(NewMemoryGetTool(memToolKit))                   // 霈?極??
		registry.Register(NewMemoryForgetTool(memToolKit))                // ?箏?撌亙
	}

	// ??撌亙 (霈?LLM ?臭誑閮剖? Cron)
	registry.Register(&SchedulerTool{Mgr: schedMgr})

	// 隞餃?閬?撌亙
	registry.Register(NewPlannerTool())

	// [NEW] 蝟餌絞蝻箸?撌亙
	registry.Register(&ReportMissingTool{})

	// 閮餃?瑼?蝟餌絞撌亙
	registry.Register(&FsMkdirTool{Manager: fsManager})
	registry.Register(&FsWriteFileTool{Manager: fsManager})
	registry.Register(&FsListDirTool{Manager: fsManager})
	registry.Register(&FsRemoveTool{Manager: fsManager})
	registry.Register(&FsReadFileTool{
		Manager:     fsManager,
		MaxReadSize: 32 * 1024, // ?身 32KB
	})
	registry.Register(&FsAppendFileTool{Manager: fsManager})

	// --- ?舐匱蝥憓??賊???賢極??---
	// ?啣? Advisor Skill (擃??)
	advisorSkill := skills.NewAdvisorSkill(client, cfg.Model)
	registry.RegisterWithPriority(advisorSkill.CreateTool(), 10)

	// [NEW] 頛?????(skills.md)
	// ????Docker Client (?澈蝯行???Dynamic Skills)
	dockerCli, err := dclient.NewClientWithOpts(dclient.FromEnv, dclient.WithAPIVersionNegotiation())
	if err != nil {
		log.Printf("?? [Skills] ?⊥?????Docker Client: %v (Sidecar 璅∪?撠瘜蝙??", err)
		dockerCli = nil
	}

	skillsDir := filepath.Join(home, "skills")

	// ????SkillManager (鞎痊???? Registry 頛)
	skillRegistryPath := filepath.Join(home, "botmemory", "skillregistry.json")
	skillManager := NewSkillManager(skillsDir, skillRegistryPath, registry, dockerCli)

	// 1. 敺?Registry ?撌脣?鋆????(??????
	if err := skillManager.LoadAll(); err != nil {
		log.Printf("?? [Skills] LoadAll failed: %v", err)
	}

	// 2. ???桅?頛???啣???SKILL.md
	if err := skillManager.LoadLocalSkills(skillsDir); err != nil {
		log.Printf("?? [Skills] LoadLocalSkills failed: %v", err)
	}

	// Keep manage_email on a single stable path after local skills are loaded.
	registry.RegisterWithPriority(&ManageEmailSkillTool{}, 10)
	// ?啣? Skill ?單??嗅遣蝡極??(Meta-Tool)
	registry.Register(&CreateSkillTool{})

	// 閮餃? GitHub Skill Installer
	registry.Register(&SkillInstaller{
		Manager: skillManager,
		BaseDir: skillsDir,
	})

	// 閮餃? Skills Reload 撌亙 (New)
	registry.Register(&ReloadSkillsTool{Manager: skillManager})

	// 閮餃? Skill 撉冽?Ｙ???& 閬撽???
	registry.Register(&SkillScaffoldTool{SkillsDir: skillsDir})
	registry.Register(&SkillValidateTool{SkillsDir: skillsDir})

	// [NEW] ?芸???賜??極??
	registry.Register(NewSkillGeneratorTool(client, cfg.Model, skillsDir))

	// [FIX] 閮餃? manage_email 隞餃?憿? (閫?捱 Scheduler Warning)
	schedMgr.RegisterTaskType("manage_email", func() {
		// ?身?: ?仿?芾?靽∩辣
		args := `{"query":"is:unread", "limit":5}`
		res, err := registry.CallTool("manage_email", args)
		if err != nil {
			log.Printf("??[Scheduler] manage_email task failed: %v", err)
			return
		}
		// 憒??摰?(銝??舀銝)嚗? Telegram
		if res != "" && !strings.Contains(res, "?曆??啁泵??隞嗥??萎辣") {
			fmt.Println("? [Scheduler] Sending email digest to Telegram...")
			if cfg.TelegramToken != "" && cfg.TelegramAdminID != "" {
				resty.New().R().
					SetBody(map[string]string{
						"chat_id":    cfg.TelegramAdminID,
						"text":       res,
						"parse_mode": "Markdown",
					}).
					Post(fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", cfg.TelegramToken))
			}
		}
	})
	// [NEW] 閮餃??券?蝪∪隞餃? (email + calendar + weather ??LLM ??Telegram)
	schedMgr.RegisterTaskType("morning_briefing", func() {
		fmt.Println("?儭?[Scheduler] ???Ｙ??券?蝪∪...")
		today := time.Now().Format("2006-01-02")

		var emailResult, calendarResult, weatherResult string

		// 1. 霈?霈?萎辣
		if res, err := registry.CallTool("manage_email", `{"query":"is:unread","limit":"10"}`); err != nil {
			log.Printf("?? [MorningBriefing] Email 霈?仃?? %v", err)
			emailResult = "嚗隞嗉??仃??"
		} else {
			emailResult = res
		}

		// 2. 霈???亥?蝔?
		calArgs := fmt.Sprintf(`{"mode":"read","from":"%s","to":"%s"}`, today, today)
		if res, err := registry.CallTool("manage_calendar", calArgs); err != nil {
			log.Printf("?? [MorningBriefing] 銵????仃?? %v", err)
			calendarResult = "嚗?鈭?霈?仃??"
		} else {
			calendarResult = res
		}

		// 3. ?亥岷憭拇除
		if res, err := registry.CallTool("get_taiwan_weather", `{"location":"?箏?撣?}`); err != nil {
			log.Printf("?? [MorningBriefing] 憭拇除?亥岷憭望?: %v", err)
			weatherResult = "嚗予瘞?閰Ｗ仃??"
		} else {
			weatherResult = res
		}

		// 4. ??LLM 敶蝪∪
		prompt := fmt.Sprintf(`雿銝雿票敹??訾?蝞∪振??冽?拐?嚗??寞?隞乩?鞈??箔蝙?刻??隞賜陛瞏???陛?晞?
隢蝜?銝剜??arkdown ?澆???嚗?瘞?澈??璆准?

## ? ?芾??萎辣
%s

## ?? 隞銵?
%s

## ?儭?憭拇除璁?
%s

隢鼠???湔?嚗?
1. ?儭??拙???銝?亥店嚗?
2. ? ?萎辣??嚗?憭?3 蝑?暺?
3. ?? 隞銵?蝮質汗
4. ?儭?憭拇除??
5. ? 皞恍成撱箄降
`, emailResult, calendarResult, weatherResult)

		// 雿輻閮剖???LLM Provider ?Ｙ???
		var briefingResult strings.Builder
		chatFn := llms.GetDefaultChatStream()
		_, llmErr := chatFn(cfg.Model, []ollama.Message{
			{Role: "system", Content: "雿銝雿票敹??訾?蝞∪振??},
			{Role: "user", Content: prompt},
		}, nil, ollama.Options{Temperature: 0.5}, func(c string) { briefingResult.WriteString(c) })

		briefing := ""
		if llmErr != nil {
			log.Printf("?? [MorningBriefing] LLM 敶憭望?: %v", llmErr)
			// Fallback: ?湔?潸???鞈?
			briefing = fmt.Sprintf("?儭??拙?嚗誑銝隞璁汗嚗n\n? **?萎辣**\n%s\n\n?? **銵?**\n%s\n\n?儭?**憭拇除**\n%s",
				emailResult, calendarResult, weatherResult)
		} else {
			briefing = strings.TrimSpace(briefingResult.String())
		}

		// 5. ?潮 Telegram (??閰?Markdown嚗仃???函???)
		if cfg.TelegramToken != "" && cfg.TelegramAdminID != "" {
			fmt.Println("? [Scheduler] ?潮?陛?勗 Telegram...")
			tgURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", cfg.TelegramToken)
			tgResp, tgErr := resty.New().R().
				SetBody(map[string]string{
					"chat_id":    cfg.TelegramAdminID,
					"text":       briefing,
					"parse_mode": "Markdown",
				}).
				Post(tgURL)
			// 憒? Markdown 閫??憭望? (Telegram ? 400)嚗?函?????
			if tgErr != nil || tgResp.StatusCode() == 400 {
				log.Printf("?? [MorningBriefing] Markdown ?潮仃???寧蝝?摮???..")
				resty.New().R().
					SetBody(map[string]string{
						"chat_id": cfg.TelegramAdminID,
						"text":    briefing,
					}).
					Post(tgURL)
			}
		}
		fmt.Println("??[Scheduler] ?券?蝪∪摰?嚗?)

		// [?剜?閮] ?芸?摮?券?蝪∪?極?瑕???
		ttlDays := cfg.ShortTermTTLDays
		if ttlDays <= 0 {
			ttlDays = 7
		}
		ctxMem := context.Background()
		if emailResult != "" {
			_ = sqliteDB.AddShortTermMemory(ctxMem, "email", emailResult, ttlDays)
		}
		if calendarResult != "" {
			_ = sqliteDB.AddShortTermMemory(ctxMem, "calendar", calendarResult, ttlDays)
		}
		if weatherResult != "" {
			_ = sqliteDB.AddShortTermMemory(ctxMem, "weather", weatherResult, ttlDays)
		}
		if briefing != "" {
			_ = sqliteDB.AddShortTermMemory(ctxMem, "briefing", briefing, ttlDays)
		}
	})

	if err := schedMgr.EnsureSystemJob("daily_memory_cleanup", "0 3 * * *", "memory_cleanup", "瘥 03:00 皜????剜?閮"); err != nil {
		log.Printf("?對? [Scheduler] memory_cleanup job: %v", err)
	}

	// --- 閮餃? memory_cleanup 隞餃?憿? (瘥予? 3 暺???????? ---
	schedMgr.RegisterTaskType("memory_cleanup", func() {
		ctxClean := context.Background()
		deleted, err := sqliteDB.CleanExpiredMemory(ctxClean)
		if err != nil {
			log.Printf("?? [MemoryCleanup] 皜?憭望?: %v", err)
		} else {
			fmt.Printf("?完 [MemoryCleanup] 撌脣??%d 蝑?????跚n", deleted)
		}
	})
	if err := schedMgr.AddJob("daily_memory_cleanup", "0 3 * * *", "memory_cleanup", "瘥 03:00 皜????剜?閮"); err != nil {
		log.Printf("?對? [Scheduler] memory_cleanup job: %v", err)
	}

	// [NEW] 閮餃? memory_sleep_optimization 隞餃?憿? (瘥予? 3 暺銵?auto_summaries 蝣????園???
	schedMgr.RegisterTaskType("memory_sleep", func() {
		ctxSleep := context.Background()
		// ??銝??隤輯? history ?賢??default chat stream ??prompt 蝯?LLM (?ㄐ?梁 cfg.Model)
		err := history.OptimizeAutoSummaries(ctxSleep, func(prompt string) (string, error) {
			var resp strings.Builder
			chatFn := llms.GetDefaultChatStream()
			_, lErr := chatFn(cfg.Model, []ollama.Message{
				{Role: "user", Content: prompt},
			}, nil, ollama.Options{Temperature: 0.1}, func(c string) { resp.WriteString(c) })
			return resp.String(), lErr
		})

		if err != nil {
			log.Printf("?? [MemorySleep] ?∠??憭望?: %v", err)
		}
	})
	if err := schedMgr.EnsureSystemJob("memory_sleep_optimization", "0 3 * * *", "memory_sleep", "?蔥蝣????塚?皜?擃?"); err != nil {
		log.Printf("?對? [Scheduler] memory_sleep job: %v", err)
	}

	// ?冽???TaskType 閮餃?摰?敺?(? Dynamic Tools)嚗?頛鞈?摨思葉??蝔?
	if err := schedMgr.LoadJobs(); err != nil {
		fmt.Printf("?? [Scheduler] Failed to load persistent jobs: %v\n", err)
	}

	// --- ?啣?嚗elegram & WhatsApp ?游? ---
	var tgChannel *channel.TelegramChannel // 摰???典??其誑靘?cleanup 摮?
	var waChannel *channel.WhatsAppChannel
	var wsChannel *channel.WebSocketChannel // [NEW] WebSocket Channel

	// [FIX] 蝘餃??圈ㄐ嚗Ⅱ靽?registry 撌脩?閮餃?摰??極??
	if cfg.TelegramToken != "" || cfg.WhatsAppEnabled || cfg.WebsocketEnabled {
		// 1. 撱箇? Agent Adapter
		adapter := gateway.NewAgentAdapter(registry, cfg.Model, cfg.SystemPrompt, cfg.TelegramDebug, logger)

		// [?剜?閮] 閮剖??芸?摮?矽
		ttlDays := cfg.ShortTermTTLDays
		if ttlDays <= 0 {
			ttlDays = 7
		}
		adapter.SetShortTermMemoryCallback(func(source, content string) {
			// ?芣??批捆 (?踹? DB ?刻)
			if len(content) > 2000 {
				content = content[:2000] + "...竄撌脫?溘?
			}
			ctxMem := context.Background()
			if err := sqliteDB.AddShortTermMemory(ctxMem, source, content, ttlDays); err != nil {
				fmt.Printf("?? [ShortTermMemory] 摮憭望? (%s): %v\n", source, err)
			} else {
				fmt.Printf("?? [ShortTermMemory] 撌脣???[%s] (%d 摮?, TTL=%d憭?\n", source, len(content), ttlDays)
			}
		})

		// [MEMORY-FIRST] 閮剖?閮??撠?隤?
		if sqliteDB != nil || GlobalMemoryToolKit != nil {
			adapter.SetMemorySearchCallback(agent.BuildMemorySearchFunc(sqliteDB, GlobalMemoryToolKit))
		}

		// [TASK RECOVERY] 閮剖??芸??遙?炎?亙?隤?
		adapter.SetPendingPlanCallback(CheckPendingPlan)
		adapter.SetTaskLockCallbacks(AcquireTaskLock, ReleaseTaskLock, IsTaskLocked)

		// 2. 撱箇? Dispatcher
		// 瘜冽?嚗ispatcher ?桀?蝬? TelegramAdminID嚗 WhatsApp 靘?銝?蝞∠????航??游? Dispatcher
		// 雿蝪∪?嚗???Admin ID 瑼Ｘ (Dispatcher ?折?乩?撘瑕 Admin ??撌?
		dispatcher := gateway.NewDispatcher(adapter, cfg.TelegramAdminID)
		if onAsyncEvent != nil {
			dispatcher.OnCompletion = onAsyncEvent
		}

		// 3. 撱箇? Telegram Channel
		if cfg.TelegramToken != "" {
			var err error
			tgChannel, err = channel.NewTelegramChannel(cfg.TelegramToken, cfg.TelegramDebug)
			if err != nil {
				log.Printf("?? ?⊥??? Telegram Channel: %v", err)
			} else {
				// 4. ???? (??甇?
				go tgChannel.Listen(dispatcher.HandleMessage)
				// log.Println("??Telegram Channel 撌脣??蒂????Gateway") // Listen ?折?
			}
		}

		// 3.5 撱箇? WhatsApp Channel
		if cfg.WhatsAppEnabled {
			var err error
			waChannel, err = channel.NewWhatsAppChannel(cfg.WhatsAppStorePath, logger)
			if err != nil {
				log.Printf("?? ?⊥??? WhatsApp Channel: %v", err)
			} else {
				go waChannel.Listen(dispatcher.HandleMessage)
			}

			// [FIX] ?∟??臬?????質酉?極?瘀??踹? Agent 撟餉死 (Run ?扳?瑼Ｘ Channel ?臬??nil)
			registry.Register(&WhatsAppSendTool{Channel: waChannel})
		}

		// 3.6 撱箇? WebSocket Channel
		if cfg.WebsocketEnabled && cfg.WebsocketURL != "" {
			var err error
			wsChannel, err = channel.NewWebSocketChannel(cfg.WebsocketURL, cfg.WebsocketUserID)
			if err != nil {
				log.Printf("?? ?⊥??? WebSocket Channel: %v", err)
			} else {
				go wsChannel.Listen(dispatcher.HandleMessage)
			}
		}
	}

	// 瘜典撌亙?瑁??典憭扯
	myBrain.SetTools(registry)

	// 撱箇? Cleanup Function
	cleanup := func() {
		if tgChannel != nil {
			tgChannel.Stop()
		}
		if waChannel != nil {
			waChannel.Stop()
		}
		if wsChannel != nil {
			wsChannel.Stop()
		}
	}

	return registry, cleanup
}
