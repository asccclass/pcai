package config

import (
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
)

// Config 儲存全域配置參數
type Config struct {
	Model        string
	OllamaURL    string
	SystemPrompt string
	FontPath     string
	OutputDir    string
	HistoryPath  string
}

// LoadConfig 負責初始化配置，支援 .env 檔案與環境變數
func LoadConfig() *Config {
	home, _ := os.UserHomeDir()

	// 嘗試從多個位置載入 .env 檔案
	// 優先順序：當前目錄 > 用戶家目錄
	_ = godotenv.Load("envfile")
	_ = godotenv.Load(filepath.Join(home, "envfile"))

	return &Config{
		// 從環境變數讀取，若無則使用後方的預設值
		Model:        getEnv("PCAI_MODEL", "llama3.3"),
		OllamaURL:    getEnv("PCAI_OLLAMA_URL", "http://localhost:11434"),
		SystemPrompt: getEnv("PCAI_SYSTEM_PROMPT", "你是一個專業的助手"),
		FontPath:     getEnv("PCAI_FONT_PATH", filepath.Join(home, "assets", "fonts", "msjh.ttf")),
		OutputDir:    getEnv("PCAI_PDF_OUTPUT_DIR", "./exports"),
		HistoryPath:  getEnv("PCAI_HISTORY_PATH", filepath.Join(home, "internal", "history")),
	}
}

// getEnv 是輔助函式，用來處理環境變數與預設值的邏輯
func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
