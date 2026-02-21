package agent

// ExportGetToolHint 導出 getToolHint 供外部測試使用。
// 僅供系統測試使用，不應在正式程式碼中呼叫。
func ExportGetToolHint(input string) string {
	return getToolHint(input, "pending_test_123456789")
}
