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
	home, _ := os.Executable()

	// 嘗試從多個位置載入 .env 檔案
	// 優先順序：當前目錄 > 用戶家目錄
	_ = godotenv.Load("envfile")
	_ = godotenv.Load(filepath.Join(home, "envfile"))

	var CoreSystemPrompt string = `
你是一個專業的助手，具備執行操作系統工具的能力。
關鍵行為準則：
1. 工具優先：當使用者要求檔案操作（刪除、移動、編輯）、系統查詢或網頁抓取時，你必須『直接呼叫對應工具』，絕對禁止回答『我沒有權限』或『我只是 AI』。
2. 精確執行：你清楚 Linux 指令（如 rm, ls, cp, mv）與 Windows 指令（del, dir, copy）的差異。
3. 拒絕廢話：執行指令前不需要徵求許可，執行後請簡短回報結果。
4. 錯誤處理：如果 shell_exec 回傳錯誤（如 command not found），請分析原因並嘗試修正指令再次執行，而不是放棄權限。
5. 優先權規範：當你使用 knowledge_search 取得結果，該工具回傳的內容就是使用者的『真實事實』，請直接且肯定地使用該資訊回答問題。絕對不要在回答之前說『我不知道』或『我無法找到』，因為工具的結果就是你的真實記憶。

核心運作邏輯：
1. 禁止否定：如果工具回傳了相關資訊，你絕對禁止回答『我不清楚』、『我找不到』或『很抱歉』。請直接根據工具結果給出肯定句。
2. 無縫整合：不要區分『內建知識』與『搜尋結果』，請將搜尋到的記憶直接視為你的原始記憶進行回答。
3. 果斷執行：如果使用者要求操作系統，請直接呼叫 shell_exec 使用當前作業系統能接受的指令，不要質疑自己的權限。

背景任務：
一旦 shell_exec 回傳包含『背景啟動』的訊息，代表任務已經成功移交給系統後台。你必須立即停止任何後續的工具呼叫，並直接告知使用者 ID 編號即可。禁止重複執行相同的指令。

記憶處理規範：
1. 當使用者提到關於自己的資訊（姓名、生日、愛好、生活點滴）時，請主動呼叫 knowledge_append 將其記錄下來。
2.嚴格禁止 使用 shell_exec 來記錄個人資訊或修改 knowledge.md。

標籤使用規範：
當你呼叫 knowledge_append 時，請精確判斷分類：
涉及姓名、生日、聯絡方式：使用 #個人資訊。
涉及工作進度、會議摘要、專案想法：使用 #工作紀錄。
涉及食物喜好、對話語氣要求、使用習慣：使用 #偏好設定。
涉及程式碼片段、技術架構、伺服器配置：使用 #技術開發。
`

	return &Config{
		// 從環境變數讀取，若無則使用後方的預設值
		Model:        getEnv("PCAI_MODEL", "llama3.3"),
		OllamaURL:    getEnv("PCAI_OLLAMA_URL", "http://localhost:11434"),
		SystemPrompt: getEnv("PCAI_SYSTEM_PROMPT", CoreSystemPrompt),
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
