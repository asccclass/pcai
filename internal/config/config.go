package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// Config 儲存全域配置參數
type Config struct {
	Model           string
	OllamaURL       string
	SystemPrompt    string
	FontPath        string
	OutputDir       string
	HistoryPath     string
	TelegramToken   string
	TelegramAdminID string
	TelegramDebug   bool
}

func getEnvBool(key string, fallback bool) bool {
	if value, ok := os.LookupEnv(key); ok {
		return value == "true" || value == "1"
	}
	return fallback
}

// getEnv 是輔助函式，用來處理環境變數與預設值的邏輯
func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

// LoadConfig 負責初始化配置，支援 .env 檔案與環境變數
func LoadConfig() *Config {
	home, _ := os.Executable()

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

	return &Config{
		// 從環境變數讀取，若無則使用後方的預設值
		Model:        getEnv("PCAI_MODEL", "llama3.3"),
		OllamaURL:    getEnv("OLLAMA_HOST", "http://localhost:11434"),
		SystemPrompt: getEnv("PCAI_SYSTEM_PROMPT", CoreSystemPrompt),
		FontPath:     getEnv("PCAI_FONT_PATH", filepath.Join(home, "assets", "fonts", "msjh.ttf")),
		OutputDir:    getEnv("PCAI_PDF_OUTPUT_DIR", "./exports"),

		HistoryPath:     getEnv("PCAI_HISTORY_PATH", filepath.Join(home, "internal", "history")),
		TelegramToken:   getEnv("TELEGRAM_TOKEN", ""),
		TelegramAdminID: getEnv("TELEGRAM_ADMIN_ID", ""),
		TelegramDebug:   getEnvBool("TELEGRAM_DEBUG", false),
	}
}
