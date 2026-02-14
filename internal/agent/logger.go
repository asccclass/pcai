package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// LogEvent 定義日誌事件類型
type LogEvent string

const (
	EventUserInput  LogEvent = "user_input"
	EventToolCall   LogEvent = "tool_call"
	EventToolResult LogEvent = "tool_result"
	EventAIResponse LogEvent = "ai_response"
	EventError      LogEvent = "error"
)

// LogEntry 定義單條日誌結構 (JSONL)
type LogEntry struct {
	Timestamp string   `json:"timestamp"`
	Event     LogEvent `json:"event"`
	Content   string   `json:"content,omitempty"`
	// Tool 相關欄位
	ToolName string `json:"tool_name,omitempty"`
	ToolArgs string `json:"tool_args,omitempty"`
	Success  *bool  `json:"success,omitempty"` // 指標以便區分 nil
	Error    string `json:"error,omitempty"`
}

// SystemLogger 負責寫入系統日誌
type SystemLogger struct {
	mu       sync.Mutex
	filePath string
	file     *os.File
}

// NewSystemLogger 初始化日誌器
// logDir: 日誌目錄，例如 "botmemory"
func NewSystemLogger(logDir string) (*SystemLogger, error) {
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	filePath := filepath.Join(logDir, "system.log")
	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open system log file: %w", err)
	}

	return &SystemLogger{
		filePath: filePath,
		file:     f,
	}, nil
}

// Close 關閉檔案
func (l *SystemLogger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

func (l *SystemLogger) writeEntry(entry LogEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file == nil {
		return
	}

	data, err := json.Marshal(entry)
	if err != nil {
		fmt.Printf("⚠️ [Logger] Failed to marshal log entry: %v\n", err)
		return
	}

	_, err = l.file.Write(append(data, '\n'))
	if err != nil {
		fmt.Printf("⚠️ [Logger] Failed to write to log file: %v\n", err)
	}
}

// LogUserInput 記錄使用者輸入
func (l *SystemLogger) LogUserInput(input string) {
	l.writeEntry(LogEntry{
		Timestamp: time.Now().Format(time.RFC3339),
		Event:     EventUserInput,
		Content:   input,
	})
}

// LogToolCall 記錄工具呼叫
func (l *SystemLogger) LogToolCall(name, args string) {
	l.writeEntry(LogEntry{
		Timestamp: time.Now().Format(time.RFC3339),
		Event:     EventToolCall,
		ToolName:  name,
		ToolArgs:  args,
	})
}

// LogToolResult 記錄工具執行結果
func (l *SystemLogger) LogToolResult(name string, result string, err error) {
	success := err == nil
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}

	l.writeEntry(LogEntry{
		Timestamp: time.Now().Format(time.RFC3339),
		Event:     EventToolResult,
		ToolName:  name,
		Content:   result, // 結果內容
		Success:   &success,
		Error:     errMsg,
	})
}

// LogAIResponse 記錄 AI 回應
func (l *SystemLogger) LogAIResponse(response string) {
	l.writeEntry(LogEntry{
		Timestamp: time.Now().Format(time.RFC3339),
		Event:     EventAIResponse,
		Content:   response,
	})
}

// LogError 記錄一般錯誤
func (l *SystemLogger) LogError(prefix string, err error) {
	l.writeEntry(LogEntry{
		Timestamp: time.Now().Format(time.RFC3339),
		Event:     EventError,
		Content:   prefix,
		Error:     err.Error(),
	})
}

// LogHallucination 記錄幻覺 (嘗試呼叫不存在的工具)
func (l *SystemLogger) LogHallucination(instruction, toolName string) {
	// 這裡我們直接寫入 notools.log，保持與 tools.ReportMissingTool 一致的行為
	// 雖然有點重複代碼，但避免了 circular dependency

	entry := map[string]string{
		"timestamp":   time.Now().Format(time.RFC3339),
		"type":        "Hallucination",
		"instruction": instruction,
		"missing":     toolName,
	}

	data, _ := json.Marshal(entry)

	// 假設 botmemory 目錄已存在 (logger 初始化時會建立)
	// logDir is parent of l.filePath
	logDir := filepath.Dir(l.filePath)
	logPath := filepath.Join(logDir, "notools.log")

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("⚠️ [Logger] Failed to open notools.log: %v\n", err)
		return
	}
	defer f.Close()

	if _, err := f.WriteString(string(data) + "\n"); err != nil {
		fmt.Printf("⚠️ [Logger] Failed to write to notools.log: %v\n", err)
	}
}
