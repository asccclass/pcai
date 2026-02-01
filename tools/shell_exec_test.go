package tools

import "testing"

func TestSanitizeCommand(t *testing.T) {
	shellTool := &ShellExecTool{}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"帶引號指令", `"ls -la"`, "ls -la"},
		{"修正刪除", `delete test.txt`, "rm -f test.txt"},
		{"轉義引號", `\"echo hello\"`, "echo hello"},
		{"前後空白", `  ls -la  `, "ls -la"},
		{"單引號包裹", `'rm -rf /'`, "rm -rf /"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shellTool.sanitizeCommand(tt.input)
			if result != tt.expected {
				t.Errorf("失敗 [%s]: 輸入 [%s], 預期 [%s], 得到 [%s]", tt.name, tt.input, tt.expected, result)
			}
		})
	}
}
