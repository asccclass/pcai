package tools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/ollama/ollama/api"
)

// Tool 定義 PCAI 工具的介面 (假設你的系統有類似的介面)
type Tool interface {
	Name() string
	Description() string
	Run(argsJSON string) (string, error)
}

// FileSystemManager 管理檔案操作的安全性
type FileSystemManager struct {
	RootPath string // 限制 AI 只能在這個目錄下活動 (Sandbox)
}

// NewFileSystemManager 初始化並確保根目錄存在
func NewFileSystemManager(rootPath string) (*FileSystemManager, error) {
	if rootPath == "" {
		return nil, fmt.Errorf("工作根目錄不能為空")
	}
	absPath, err := filepath.Abs(rootPath)
	if err != nil {
		return nil, err
	}
	// 確保根目錄存在
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		if err := os.MkdirAll(absPath, 0755); err != nil {
			return nil, fmt.Errorf("無法建立工作根目錄: %v", err)
		}
	}
	return &FileSystemManager{RootPath: absPath}, nil
}

// validatePath 檢查路徑是否越權 (Path Traversal Protection)
func (m *FileSystemManager) validatePath(userPath string) (string, error) {
	// 將路徑結合併清理 (處理 ../ 等符號)
	fullPath := filepath.Join(m.RootPath, userPath)
	cleanPath := filepath.Clean(fullPath)

	// 檢查清理後的路徑是否仍以 RootPath 開頭
	if !strings.HasPrefix(cleanPath, m.RootPath) {
		return "", fmt.Errorf("安全性錯誤: 禁止存取工作目錄以外的路徑 (%s)", userPath)
	}
	return cleanPath, nil
}

// ==================== 1. FsMkdir ====================

type FsMkdirTool struct {
	Manager *FileSystemManager
}

func (t *FsMkdirTool) Name() string { return "fs_mkdir" }
func (t *FsMkdirTool) Description() string {
	return `建立目錄。輸入 JSON: {"path": "skills/new_skill"}`
}

func (t *FsMkdirTool) Definition() api.Tool {
	return api.Tool{
		Type: "function",
		Function: api.ToolFunction{
			Name:        "fs_mkdir",
			Description: "在工作目錄下建立新目錄",
			Parameters: func() api.ToolFunctionParameters {
				var props api.ToolPropertiesMap
				// ToolPropertiesMap has unexported fields, so we initialize it via JSON
				js := `{
					"path": {
						Type:        "string",
						Description: "要建立的目錄路徑 (相對路徑)",
					},
				}`
				_ = json.Unmarshal([]byte(js), &props)

				return api.ToolFunctionParameters{
					Type:       "object",
					Properties: &props,
					Required:   []string{"path"},
				}
			}(),
		},
	}
}

func (t *FsMkdirTool) Run(argsJSON string) (string, error) {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("參數格式錯誤: %v", err)
	}

	safePath, err := t.Manager.validatePath(args.Path)
	if err != nil {
		return "", err
	}

	// 0755 代表擁有者可讀寫執行，其他人可讀執行
	if err := os.MkdirAll(safePath, 0755); err != nil {
		return "", fmt.Errorf("建立目錄失敗: %v", err)
	}

	return fmt.Sprintf("成功建立目錄: %s", args.Path), nil
}

// ==================== 2. FsWriteFile ====================

type FsWriteFileTool struct {
	Manager *FileSystemManager
}

func (t *FsWriteFileTool) Name() string { return "fs_write_file" }
func (t *FsWriteFileTool) Description() string {
	return `寫入檔案 (若存在則覆寫)。輸入 JSON: {"path": "test.txt", "content": "hello"}`
}

func (t *FsWriteFileTool) Definition() api.Tool {
	return api.Tool{
		Type: "function",
		Function: api.ToolFunction{
			Name:        "fs_write_file",
			Description: "寫入檔案內容 (若存在則覆寫)",
			Parameters: func() api.ToolFunctionParameters {
				var props api.ToolPropertiesMap
				js := `{
					"path": {
						Type:        "string",
						Description: "要建立的目錄路徑 (相對路徑)",
					},
					"content": {
						Type:        "string",
						Description: "要建立的目錄路徑 (相對路徑)",
					},
				}`
				_ = json.Unmarshal([]byte(js), &props)

				return api.ToolFunctionParameters{
					Type:       "object",
					Properties: &props,
					Required:   []string{"path"},
				}
			}(),
		},
	}
}

func (t *FsWriteFileTool) Run(argsJSON string) (string, error) {
	var args struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("參數格式錯誤: %v", err)
	}

	safePath, err := t.Manager.validatePath(args.Path)
	if err != nil {
		return "", err
	}

	// 寫入檔案 (0644 代表一般檔案權限)
	if err := os.WriteFile(safePath, []byte(args.Content), 0644); err != nil {
		return "", fmt.Errorf("寫入檔案失敗: %v", err)
	}

	return fmt.Sprintf("成功寫入檔案: %s (%d bytes)", args.Path, len(args.Content)), nil
}

// ==================== 3. FsListDir ====================

type FsListDirTool struct {
	Manager *FileSystemManager
}

func (t *FsListDirTool) Name() string { return "fs_list_dir" }
func (t *FsListDirTool) Description() string {
	return `列出目錄內容。輸入 JSON: {"path": "skills/"}`
}

func (t *FsListDirTool) Definition() api.Tool {
	return api.Tool{
		Type: "function",
		Function: api.ToolFunction{
			Name:        "fs_list_dir",
			Description: "列出指定目錄下的檔案與子目錄",
			Parameters: func() api.ToolFunctionParameters {
				var props api.ToolPropertiesMap
				js := `{
					"path": {
						Type:        "string",
						Description: "目錄路徑 (相對路徑)",
					}
				}`
				_ = json.Unmarshal([]byte(js), &props)

				return api.ToolFunctionParameters{
					Type:       "object",
					Properties: &props,
					Required:   []string{"path"},
				}
			}(),
		},
	}
}

func (t *FsListDirTool) Run(argsJSON string) (string, error) {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		// 容錯：若沒傳 path，預設列出根目錄
		args.Path = ""
	}

	safePath, err := t.Manager.validatePath(args.Path)
	if err != nil {
		return "", err
	}

	entries, err := os.ReadDir(safePath)
	if err != nil {
		return "", fmt.Errorf("讀取目錄失敗: %v", err)
	}

	// 整理回傳結果，讓 AI 知道哪些是資料夾，哪些是檔案
	var result []string
	for _, entry := range entries {
		typeStr := "FILE"
		if entry.IsDir() {
			typeStr = "DIR " // 加空格為了對齊好看，或方便 parse
		}
		result = append(result, fmt.Sprintf("[%s] %s", typeStr, entry.Name()))
	}

	if len(result) == 0 {
		return "目錄是空的", nil
	}

	return strings.Join(result, "\n"), nil
}

// === 4. FsRemove: 刪除檔案或目錄 (NEW) ===

type FsRemoveTool struct {
	Manager *FileSystemManager
}

func (t *FsRemoveTool) Name() string { return "fs_remove" }
func (t *FsRemoveTool) Description() string {
	return `刪除檔案或整個目錄 (小心使用)。JSON範例: {"path": "temp_folder"}`
}

func (t *FsRemoveTool) Definition() api.Tool {
	return api.Tool{
		Type: "function",
		Function: api.ToolFunction{
			Name:        "fs_remove",
			Description: "刪除檔案或目錄",
			Parameters: func() api.ToolFunctionParameters {
				var props api.ToolPropertiesMap
				js := `{
					"path": {
						Type:        "string",
						Description: "目錄路徑 (相對路徑)",
					}
				}`
				_ = json.Unmarshal([]byte(js), &props)

				return api.ToolFunctionParameters{
					Type:       "object",
					Properties: &props,
					Required:   []string{"path"},
				}
			}(),
		},
	}
}

func (t *FsRemoveTool) Run(argsJSON string) (string, error) {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("JSON 格式錯誤: %v", err)
	}

	safePath, err := t.Manager.validatePath(args.Path)
	if err != nil {
		return "", err
	}

	// 【關鍵安全檢查】防止刪除根目錄 (沙箱本身)
	// 如果 safePath 等於 RootPath，代表使用者傳入了 "." 或 ""，試圖刪除整個工作區
	if safePath == t.Manager.RootPath {
		return "", fmt.Errorf("安全警告: 禁止刪除工作區根目錄！請指定子目錄或檔案。")
	}

	// 使用 RemoveAll，它等同於 rm -rf，可以刪除檔案或非空目錄
	if err := os.RemoveAll(safePath); err != nil {
		return "", fmt.Errorf("刪除失敗: %v", err)
	}

	return fmt.Sprintf("成功刪除: %s", args.Path), nil
}

// === 5. FsReadFile: 讀取檔案內容 (NEW) ===

// defaultMaxReadSize 定義內部預設值 (32KB)
const defaultMaxReadSize int64 = 32 * 1024

type FsReadFileTool struct {
	Manager     *FileSystemManager
	MaxReadSize int64 // 可配置的最大讀取 byte 數，若為 0 則使用預設值
}

func (t *FsReadFileTool) Name() string { return "fs_read_file" }
func (t *FsReadFileTool) Description() string {
	// 為了讓 Description 準確顯示當前設定的大小
	limit := t.MaxReadSize
	if limit <= 0 {
		limit = defaultMaxReadSize
	}
	return fmt.Sprintf(`讀取檔案內容。若檔案過大，只會讀取前 %d KB。會自動過濾二進位檔案。`, limit/1024)
}

func (t *FsReadFileTool) Definition() api.Tool {
	return api.Tool{
		Type: "function",
		Function: api.ToolFunction{
			Name:        "fs_read_file",
			Description: "讀取檔案內容",
			Parameters: func() api.ToolFunctionParameters {
				var props api.ToolPropertiesMap
				js := `{
					"path": {
						Type:        "string",
						Description: "目錄路徑 (相對路徑)",
					}
				}`
				_ = json.Unmarshal([]byte(js), &props)

				return api.ToolFunctionParameters{
					Type:       "object",
					Properties: &props,
					Required:   []string{"path"},
				}
			}(),
		},
	}
}

func (t *FsReadFileTool) Run(argsJSON string) (string, error) {
	// 1. 決定讀取上限 (處理未初始化或是負數的情況)
	limit := t.MaxReadSize
	if limit <= 0 {
		limit = defaultMaxReadSize
	}

	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("JSON 格式錯誤: %v", err)
	}

	safePath, err := t.Manager.validatePath(args.Path)
	if err != nil {
		return "", err
	}

	f, err := os.Open(safePath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("檔案不存在: %s", args.Path)
		}
		return "", err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("錯誤: '%s' 是一個目錄，請改用 fs_list_dir", args.Path)
	}

	// --- 偵測二進位檔 (Header Check) ---
	headerBuf := make([]byte, 512)
	n, err := f.Read(headerBuf)
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("讀取檔頭失敗: %v", err)
	}
	if n == 0 {
		return "(檔案是空的)", nil
	}

	// 檢查 NUL byte
	if bytes.IndexByte(headerBuf[:n], 0) != -1 {
		return "", fmt.Errorf("略過讀取: 偵測到二進位檔案 (Binary File)")
	}

	// 檢查 MIME
	contentType := http.DetectContentType(headerBuf[:n])
	isText := strings.HasPrefix(contentType, "text/") ||
		strings.Contains(contentType, "json") ||
		strings.Contains(contentType, "xml") ||
		strings.Contains(contentType, "javascript")

	if !isText && contentType == "application/octet-stream" {
		if !utf8.Valid(headerBuf[:n]) {
			return "", fmt.Errorf("略過讀取: 偵測到非文字內容 (%s)", contentType)
		}
	}

	// --- 執行讀取 ---
	if _, err := f.Seek(0, 0); err != nil {
		return "", fmt.Errorf("重置檔案指標失敗: %v", err)
	}

	// 使用封裝好的 limit
	// 多讀 1 byte 用於判斷截斷
	reader := io.LimitReader(f, limit+1)

	content, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("讀取內容失敗: %v", err)
	}

	result := string(content)
	isTruncated := false

	// 判斷是否超過
	if int64(len(content)) > limit {
		result = string(content[:limit])
		isTruncated = true
	}

	if isTruncated {
		result += fmt.Sprintf("\n\n[系統提示: 檔案過大，僅顯示前 %d KB，總大小: %d bytes]", limit/1024, info.Size())
	}

	return result, nil
}

// ==================== 6. FsAppendFile: 附加內容到檔案 (NEW) ====================

type FsAppendFileTool struct {
	Manager *FileSystemManager
}

func (t *FsAppendFileTool) Name() string { return "fs_append_file" }
func (t *FsAppendFileTool) Description() string {
	return `將內容附加到檔案末尾 (Append)。適合寫入日誌或記憶。JSON範例: {"path": "logs/chat.log", "content": "\n新的記錄..."}`
}

func (t *FsAppendFileTool) Definition() api.Tool {
	return api.Tool{
		Type: "function",
		Function: api.ToolFunction{
			Name:        "fs_append_file",
			Description: "附加檔案內容",
			Parameters: func() api.ToolFunctionParameters {
				var props api.ToolPropertiesMap
				js := `{
					"path": {
						Type:        "string",
						Description: "目錄路徑 (相對路徑)",
					}
				}`
				_ = json.Unmarshal([]byte(js), &props)

				return api.ToolFunctionParameters{
					Type:       "object",
					Properties: &props,
					Required:   []string{"path"},
				}
			}(),
		},
	}
}

func (t *FsAppendFileTool) Run(argsJSON string) (string, error) {
	var args struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("JSON 格式錯誤: %v", err)
	}

	safePath, err := t.Manager.validatePath(args.Path)
	if err != nil {
		return "", err
	}

	// 確保父目錄存在 (防呆機制)
	parentDir := filepath.Dir(safePath)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return "", fmt.Errorf("無法建立父目錄: %v", err)
	}

	// 開啟檔案:
	// O_APPEND: 寫入時自動指到檔案末尾
	// O_CREATE: 如果檔案不存在則建立
	// O_WRONLY: 只寫模式
	f, err := os.OpenFile(safePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return "", fmt.Errorf("無法開啟檔案: %v", err)
	}
	defer f.Close()

	if _, err := f.WriteString(args.Content); err != nil {
		return "", fmt.Errorf("寫入失敗: %v", err)
	}

	return fmt.Sprintf("成功附加內容到: %s (長度: %d)", args.Path, len(args.Content)), nil
}

// 自動註冊機制
func init() {
	// 註冊 FsMkdir
	Register(func(m *FileSystemManager) Tool {
		return &FsMkdirTool{Manager: m}
	})

	// 註冊 FsWriteFile
	Register(func(m *FileSystemManager) Tool {
		return &FsWriteFileTool{Manager: m}
	})

	// 註冊 FsListDir
	Register(func(m *FileSystemManager) Tool {
		return &FsListDirTool{Manager: m}
	})

	// 註冊 FsRemove
	Register(func(m *FileSystemManager) Tool {
		return &FsRemoveTool{Manager: m}
	})

	// 註冊 FsReadFile
	Register(func(m *FileSystemManager) Tool {
		return &FsReadFileTool{
			Manager:     m,
			MaxReadSize: 64 * 1024, // 在這裡設定預設參數
		}
	})

	// 註冊 FsAppendFile
	Register(func(m *FileSystemManager) Tool {
		return &FsAppendFileTool{Manager: m}
	})

	// [FIX] 註冊別名 (Alias) 以處理 LLM 幻覺
	// 有些模型會習慣性把 append_file 叫成 append_to_file
	Register(func(m *FileSystemManager) Tool {
		t := &FsAppendFileTool{Manager: m}
		// 這裡做一個簡單的 Wrapper 來改名
		return &ToolAlias{
			Original: t,
			NewName:  "fs_append_to_file",
		}
	})
}

// ToolAlias 是一個簡單的包裝器，用來更改工具名稱
type ToolAlias struct {
	Original Tool
	NewName  string
}

func (t *ToolAlias) Name() string                    { return t.NewName }
func (t *ToolAlias) Description() string             { return t.Original.Description() }
func (t *ToolAlias) Run(args string) (string, error) { return t.Original.Run(args) }

// 透過 Type Assertion 處理 Definition
func (t *ToolAlias) Definition() api.Tool {
	// 如果原始工具支援 Definition，我們攔截並修改 Name
	if def, ok := t.Original.(interface{ Definition() api.Tool }); ok {
		d := def.Definition()
		d.Function.Name = t.NewName
		return d
	}
	return api.Tool{} // Fallback
}
