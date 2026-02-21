package cmd

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/asccclass/pcai/internal/database"
	"github.com/asccclass/pcai/internal/memory"
	"github.com/asccclass/pcai/internal/webapi"
	SherryServer "github.com/asccclass/sherryserver"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(serveCmd)
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "啟動記憶管理 Web 伺服器",
	Run:   runServe,
}

func runServe(cmd *cobra.Command, args []string) {
	// 1. 載入環境變數
	if err := godotenv.Load("envfile"); err != nil {
		fmt.Printf("⚠️ 無法載入 envfile: %v\n", err)
	}

	// 2. 讀取設定
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	documentRoot := os.Getenv("DocumentRoot")
	if documentRoot == "" {
		documentRoot = "www/html"
	}
	templateRoot := os.Getenv("TemplateRoot")
	if templateRoot == "" {
		templateRoot = "www/template"
	}

	// 3. 初始化記憶系統 (OpenClaw ToolKit)
	home, _ := os.Getwd()
	kbDir := filepath.Join(home, "botmemory", "knowledge")
	_ = os.MkdirAll(kbDir, 0750)

	ollamaHost := os.Getenv("OLLAMA_HOST")
	if ollamaHost == "" {
		ollamaHost = "http://localhost:11434"
	}

	memCfg := memory.MemoryConfig{
		WorkspaceDir: kbDir,
		StateDir:     kbDir,
		AgentID:      "pcai",
		Search: memory.SearchConfig{
			Provider:  "ollama",
			Model:     "mxbai-embed-large",
			OllamaURL: ollamaHost,
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
		},
	}

	memToolKit, err := memory.NewToolKit(memCfg)
	if err != nil {
		fmt.Printf("❌ 無法初始化記憶系統: %v\n", err)
		return
	}
	defer memToolKit.Close()

	fmt.Printf("✅ [Memory] ToolKit 初始化完成 (索引 %d 個 chunks)\n", memToolKit.ChunkCount())

	// 3.5 初始化資料庫 (讓 WebAPI 能存取 Short-term Memory)
	dbPath := filepath.Join(home, "botmemory", "pcai.db")
	sqliteDB, err := database.NewSQLite(dbPath)
	if err != nil {
		fmt.Printf("⚠️ 無法連線資料庫 (短期記憶功能可能無法使用): %v\n", err)
	} else {
		// defer sqliteDB.Close() // 持續開啟給整個 server 生命週期使用
		fmt.Println("✅ [Database] SQLite 初始化完成")
	}

	// 4. 建立 SherryServer
	server, err := SherryServer.NewServer(":"+port, documentRoot, templateRoot)
	if err != nil {
		fmt.Printf("❌ 無法建立伺服器: %v\n", err)
		return
	}

	// 5. 建立路由
	router := http.NewServeMux()

	// 5a. API 路由
	memHandler := webapi.NewMemoryHandler(memToolKit, sqliteDB)
	memHandler.AddRoutes(router)

	// 5b. 靜態檔案服務
	staticServer := SherryServer.StaticFileServer{documentRoot, "index.html"}
	staticServer.AddRouter(router)

	// 6. 啟動伺服器
	server.Server.Handler = router
	fmt.Printf("🚀 記憶管理伺服器已啟動: http://localhost:%s\n", port)
	server.Start()
}
