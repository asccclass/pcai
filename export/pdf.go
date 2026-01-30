package export

import (
	"fmt"
	"os"
	"runtime"

	"github.com/jung-kurt/gofpdf"
)

func getSystemFont() string {
	// 優先順序 1: 根據作業系統自動選擇系統字體
	switch runtime.GOOS {
	case "windows":
		return "C:\\Windows\\Fonts\\msjh.ttc" // 微軟正黑體
	case "darwin": // macOS
		return "/Library/Fonts/Arial Unicode.ttf"
	case "linux":
		// Linux 路徑較分散，通常建議隨附字體檔在 assets
		return "/usr/share/fonts/truetype/droid/DroidSansFallbackFull.ttf"
	default: // 優先順序 2: 專案目錄下的自定義字體
		localFont := "assets/fonts/TaipeiSansTCBeta-Regular.ttf"
		if _, err := os.Stat(localFont); err == nil {
			return localFont
		}
		return ""
	}
}

func SaveAsPDF(filename, content string) error {
	pdf := gofpdf.New("P", "mm", "A4", "")
	// 1. 指定字型路徑 (請確保路徑下有 ttf 檔案)
	// 這裡假設你將字型放在執行路徑下的 fonts 目錄
	fontPath := getSystemFont()

	// 檢查字型檔案是否存在
	if _, err := os.Stat(fontPath); err != nil {
		return fmt.Errorf("找不到適合的中文字體，建議手動將字體放至 assets/fonts/msjh.ttf")
	}

	// 2. 註冊 UTF-8 字型
	// 參數: 字型名稱, 樣式, 檔案路徑
	pdf.AddUTF8Font("MainFont", "", fontPath)
	pdf.SetFont("MainFont", "", 12)
	pdf.AddPage()
	// 3. 設定自動換行並寫入內容
	// 0 代表延伸到頁面邊緣，10 代表行高
	pdf.MultiCell(0, 10, content, "", "L", false)
	return pdf.OutputFileAndClose(filename)
}
