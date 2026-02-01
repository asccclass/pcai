package tools

import (
	"os"
	"testing"
)

func TestCopyFile(t *testing.T) {
	src := "test_source.txt"
	dst := "test_dest.txt"
	content := "jii哥的測試資料"

	// 準備測試檔案
	_ = os.WriteFile(src, []byte(content), 0644)
	defer os.Remove(src)
	defer os.Remove(dst)

	// 執行複製
	err := copyFile(src, dst)
	if err != nil {
		t.Fatalf("複製失敗: %v", err)
	}

	// 驗證內容
	got, _ := os.ReadFile(dst)
	if string(got) != content {
		t.Errorf("預期內容 %s, 得到 %s", content, string(got))
	}
}
