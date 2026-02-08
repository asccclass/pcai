// 建立註冊中心 (tools/registry.go)，我們需要一個地方來存放所有「待命」的工具
package tools

// ToolFactory 定義一個函數類型，用於產生工具實例
// 這裡傳入 manager 是因為你的檔案工具都依賴它
// 如果未來有不需要 manager 的工具，可以在實作時忽略這個參數
type ToolFactory func(manager *FileSystemManager) Tool

// internalRegistry 存放所有已註冊的工具工廠
var internalRegistry []ToolFactory

// Register 讓各個工具檔案可以用來註冊自己 (通常在 init() 呼叫)
func Register(factory ToolFactory) {
	internalRegistry = append(internalRegistry, factory)
}

// 假設你有一個 config 字串列表，例如 ["fs_mkdir", "fs_read_file"]
// 如果 whitelist 為空，代表啟用全部
func LoadAllTools(manager *FileSystemManager, whitelist []string) []Tool {
	var tools []Tool

	// 轉成 map 加速查詢
	allowed := make(map[string]bool)
	for _, name := range whitelist {
		allowed[name] = true
	}
	enableAll := len(whitelist) == 0

	for _, factory := range internalRegistry {
		// 這裡執行依賴注入 (Dependency Injection)
		t := factory(manager)
		if enableAll || allowed[t.Name()] {
			tools = append(tools, t)
		}
	}
	return tools
}
