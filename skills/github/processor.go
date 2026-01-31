// 這是 PCAI 呼叫 LLM 後，將結果寫入檔案的銜接層。
package github

import (
	"os"
	"path/filepath"
)

// ApplyAutoComments 將 LLM 產生的帶註解內容更新至本地檔案
func ApplyAutoComments(localPath string, fileName string, newContent string) error {
	targetFile := filepath.Join(localPath, fileName)

	// 備份原始檔案 (安全性考量)
	backupFile := targetFile + ".bak"
	_ = os.Rename(targetFile, backupFile)

	err := os.WriteFile(targetFile, []byte(newContent), 0644)
	if err != nil {
		// 失敗則還原
		os.Rename(backupFile, targetFile)
		return err
	}

	os.Remove(backupFile)
	return nil
}
