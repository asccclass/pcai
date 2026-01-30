package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/asccclass/pcai/internal/config"
	"github.com/asccclass/pcai/llms/ollama"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var (
	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	failStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	labelStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
)

var healthCmd = &cobra.Command{
	Use:   "health",
	Short: "檢查 PCAI 運行環境狀態",
	Run: func(cmd *cobra.Command, args []string) {
		cfg := config.LoadConfig()
		fmt.Println(lipgloss.NewStyle().Bold(true).Render("\n🔍 PCAI 系統健康檢查\n"))

		// 1. 檢查 Ollama 服務
		fmt.Print(labelStyle.Render("1. Ollama 服務狀態: "))
		if ollama.CheckService(cfg.OllamaURL) {
			fmt.Println(successStyle.Render("● 在線 (OK)"))
		} else {
			fmt.Println(failStyle.Render("○ 離線 (ERROR) - 請確認 Ollama 是否已啟動"))
		}

		// 2. 檢查模型是否已下載
		fmt.Print(labelStyle.Render(fmt.Sprintf("2. 模型狀態 [%s]: ", cfg.Model)))
		pulled, err := ollama.IsModelPulled(cfg.OllamaURL, cfg.Model)
		if err == nil && pulled {
			fmt.Println(successStyle.Render("● 已下載 (OK)"))
		} else {
			fmt.Println(failStyle.Render("○ 未找到 - 請執行 'ollama pull " + cfg.Model + "'"))
		}

		// 3. 檢查知識庫 (knowledge.md)
		// 取得目前執行檔案的絕對路徑
		home, err := os.Executable()
		if err != nil {
			panic(err)
		}
		// 取得執行檔案的所在目錄
		kPath := filepath.Join(home, "botmemory", "knowledge", "knowledge.md")
		fmt.Print(labelStyle.Render("3. 知識庫檔案狀態: "))

		info, err := os.Stat(kPath)
		if os.IsNotExist(err) {
			fmt.Println(failStyle.Render("○ 尚未建立 (提醒：對話超過 1 小時後會自動生成)"))
		} else {
			sizeKB := float64(info.Size()) / 1024
			fmt.Printf("%s (大小: %.2f KB, 位置: %s)\n", successStyle.Render("● 正常"), sizeKB, kPath)
		}

		fmt.Println()
	},
}

func init() {
	rootCmd.AddCommand(healthCmd)
}
