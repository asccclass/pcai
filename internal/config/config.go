package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/asccclass/pcai/internal/skillloader"
	"github.com/joho/godotenv"
)

// Config 儲存全域配置參數
type Config struct {
	Model             string
	OllamaURL         string
	SystemPrompt      string
	FontPath          string
	OutputDir         string
	HistoryPath       string
	TelegramToken     string
	TelegramAdminID   string
	TelegramDebug     bool
	GOGPath           string
	ShortTermTTLDays  int // 短期記憶保留天數 (預設 7 天)
	MemoryEnabled     bool
	WhatsAppEnabled   bool
	WhatsAppStorePath string
	LineToken         string // [NEW] LINE Notify Token
}

func getEnvBool(key string, fallback bool) bool {
	if value, ok := os.LookupEnv(key); ok {
		return value == "true" || value == "1"
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if value, ok := os.LookupEnv(key); ok {
		if n, err := strconv.Atoi(value); err == nil {
			return n
		}
	}
	return fallback
}

// ensureProtocol 確保 URL 有 http:// 或 https:// 前綴
func ensureProtocol(url string) string {
	if url == "" {
		return ""
	}
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return "http://" + url
	}
	return url
}

// getEnv 是輔助函式，用來處理環境變數與預設值的邏輯
func getEnv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

// LoadConfig 負責初始化配置，支援 .env 檔案與環境變數
func LoadConfig() *Config {
	// 載入 envfile 檔案
	if err := godotenv.Overload("envfile"); err != nil {
		// 如果存在但有錯誤則
		if !os.IsNotExist(err) {
			fmt.Printf("⚠️  [Main]  envfile 檔案存在但無法載入: %v\n", err)
		}
		fmt.Printf("⚠️  [Main] 無法從執行檔目錄載入 envfile: %v\n", err)
		return nil
	}
	fmt.Println("✅ [Main] 成功載入 envfile (CWD)")

	home, _ := os.Getwd()

	// [NEW] 讀取外部 System Prompt
	soulPath := "botcharacter/SOUL.md"
	content, err := os.ReadFile(soulPath)
	var CoreSystemPrompt string
	if err != nil {
		// 嘗試從執行檔目錄讀取
		exeSoulPath := filepath.Join(filepath.Dir(home), "botcharacter", "SOUL.md")
		content, err = os.ReadFile(exeSoulPath)
		if err != nil {
			fmt.Printf("⚠️  [Config] 無法讀取 System Prompt (SOUL.md): %v\n", err)
			CoreSystemPrompt = "你是一個專業的助手。" // Fallback
		} else {
			CoreSystemPrompt = string(content)
			fmt.Printf("✅ [Config] 成功載入 System Prompt (%s)\n", exeSoulPath)
		}
	} else {
		CoreSystemPrompt = string(content)
		fmt.Printf("✅ [Config] 成功載入 System Prompt (%s)\n", soulPath)
	}

	// [SKILLS SNAPSHOT] 產生技能快照並 prepend 到 System Prompt
	if snapshot, err := skillloader.GenerateAndSaveSnapshot("skills"); err == nil && snapshot != "" {
		CoreSystemPrompt = snapshot + "\n\n" + CoreSystemPrompt
	}

	return &Config{
		// 從環境變數讀取，若無則使用後方的預設值
		Model:        getEnv("PCAI_MODEL", getEnv("PCAI_MODEL", "llama3.3")),
		OllamaURL:    ensureProtocol(getEnv("OLLAMA_HOST", "http://localhost:11434")),
		SystemPrompt: getEnv("PCAI_SYSTEM_PROMPT", CoreSystemPrompt),
		FontPath:     getEnv("PCAI_FONT_PATH", filepath.Join(home, "assets", "fonts", "msjh.ttf")),
		OutputDir:    getEnv("PCAI_PDF_OUTPUT_DIR", "./exports"),

		HistoryPath:      getEnv("PCAI_HISTORY_PATH", filepath.Join(home, "internal", "history")),
		TelegramToken:    getEnv("TELEGRAM_TOKEN", ""),
		TelegramAdminID:  getEnv("TELEGRAM_ADMIN_ID", ""),
		TelegramDebug:    getEnvBool("TELEGRAM_DEBUG", false),
		GOGPath:          getEnv("GOG_PATH", filepath.Join(home, "bin", "gog.exe")),
		ShortTermTTLDays: getEnvInt("SHORT_TERM_TTL_DAYS", 7),
		MemoryEnabled:    getEnvBool("PCAI_MEMORY_ENABLED", true),

		WhatsAppEnabled:   getEnvBool("WHATSAPP_ENABLED", false),
		WhatsAppStorePath: getEnv("WHATSAPP_STORE_PATH", filepath.Join(home, "botmemory", "whatsapp-store.db")),
		LineToken:         getEnv("LINE_TOKEN", ""),
	}
}
