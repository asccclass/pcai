package cmd

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/asccclass/pcai/internal/agent"
	"github.com/asccclass/pcai/internal/config"
	"github.com/asccclass/pcai/internal/database"
	"github.com/asccclass/pcai/internal/memory"
	"github.com/asccclass/pcai/internal/webapi"
	"github.com/asccclass/pcai/tools"
	SherryServer "github.com/asccclass/sherryserver"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(serveCmd)
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the web server",
	Run:   runServe,
}

func runServe(cmd *cobra.Command, args []string) {
	if err := godotenv.Load("envfile"); err != nil {
		fmt.Printf("Warning: could not load envfile: %v\n", err)
	}

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
		fmt.Printf("Failed to initialize memory toolkit: %v\n", err)
		return
	}
	defer memToolKit.Close()

	fmt.Printf("[Memory] toolkit initialized (%d chunks)\n", memToolKit.ChunkCount())

	dbPath := filepath.Join(home, "botmemory", "pcai.db")
	sqliteDB, err := database.NewSQLite(dbPath)
	if err != nil {
		fmt.Printf("Warning: could not initialize SQLite: %v\n", err)
	} else {
		fmt.Println("[Database] SQLite initialized")
	}

	cfg := config.LoadConfig()
	if cfg == nil {
		fmt.Printf("Warning: could not load config for tools, using fallback env values\n")
		cfg = &config.Config{
			OllamaURL:    ollamaHost,
			Model:        os.Getenv("MODEL"),
			SystemPrompt: os.Getenv("SYSTEM_PROMPT"),
		}
	}
	if cfg.OllamaURL == "" {
		cfg.OllamaURL = ollamaHost
	}
	if cfg.Model == "" {
		cfg.Model = os.Getenv("MODEL")
	}
	if cfg.Model == "" {
		cfg.Model = "llama3"
	}
	if cfg.SystemPrompt == "" {
		cfg.SystemPrompt = os.Getenv("SYSTEM_PROMPT")
	}

	registry, cleanup := tools.InitRegistry(nil, cfg, nil, nil)
	defer cleanup()

	server, err := SherryServer.NewServer(":"+port, documentRoot, templateRoot)
	if err != nil {
		fmt.Printf("Failed to create server: %v\n", err)
		return
	}

	router := http.NewServeMux()

	memHandler := webapi.NewMemoryHandler(memToolKit, sqliteDB)
	memHandler.AddRoutes(router)

	sysLogger, _ := agent.NewSystemLogger("botmemory")
	chatModel := cfg.Model
	if chatModel == "" {
		chatModel = "llama3"
	}
	systemPrompt := cfg.SystemPrompt

	chatHandler := webapi.NewChatHandler(chatModel, systemPrompt, registry, sysLogger)
	chatHandler.AddRoutes(router)

	staticServer := SherryServer.StaticFileServer{documentRoot, "index.html"}
	staticServer.AddRouter(router)

	server.Server.Handler = router
	fmt.Printf("Web server started at http://localhost:%s\n", port)
	server.Start()
}

