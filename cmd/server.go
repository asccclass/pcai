package cmd

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"

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

	// 3. 初始化記憶管理器
	home, _ := os.Getwd()
	kbDir := filepath.Join(home, "botmemory", "knowledge")
	jsonPath := filepath.Join(kbDir, "memory_store.json")

	// 確保目錄存在
	_ = os.MkdirAll(kbDir, 0755)

	ollamaHost := os.Getenv("OLLAMA_HOST")
	if ollamaHost == "" {
		ollamaHost = "http://localhost:11434"
	}
	embedder := memory.NewOllamaEmbedder(ollamaHost, "mxbai-embed-large")
	memManager := memory.NewManager(jsonPath, embedder)

	fmt.Printf("✅ [Memory] 載入 %d 筆記憶\n", memManager.Count())

	// 4. 建立 SherryServer
	server, err := SherryServer.NewServer(":"+port, documentRoot, templateRoot)
	if err != nil {
		fmt.Printf("❌ 無法建立伺服器: %v\n", err)
		return
	}

	// 5. 建立路由
	router := http.NewServeMux()

	// 5a. API 路由
	memHandler := webapi.NewMemoryHandler(memManager)
	memHandler.AddRoutes(router)

	// 5b. 靜態檔案服務
	staticServer := SherryServer.StaticFileServer{documentRoot, "index.html"}
	staticServer.AddRouter(router)

	// 6. 啟動伺服器
	server.Server.Handler = router
	fmt.Printf("🚀 記憶管理伺服器已啟動: http://localhost:%s\n", port)
	server.Start()
}
