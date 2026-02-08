package main

import (
	"fmt"
	"os"

	"github.com/asccclass/pcai/cmd"
	"github.com/joho/godotenv"
)

func main() {
	// 載入 envfile 檔案
	if err := godotenv.Load("envfile"); err != nil {
		// 如果存在但有錯誤則
		if !os.IsNotExist(err) {
			fmt.Printf("⚠️  [Main]  envfile 檔案存在但無法載入: %v\n", err)
		}
		fmt.Printf("⚠️  [Main] 無法從執行檔目錄載入 envfile: %v\n", err)
		return
	}
	fmt.Println("✅ [Main] 成功載入 envfile (CWD)")
	cmd.Execute()
}
