package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/asccclass/pcai/internal/config"
	"github.com/asccclass/pcai/llms/ollama"
	"github.com/asccclass/pcai/tools"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var (
	headerStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true).Underline(true)
	labelStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Width(18)
	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	warnStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true)
	failStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
)

// 我們需要一個外部傳入的 BackgroundManager 實例
var GlobalBgMgr *tools.BackgroundManager

var healthCmd = &cobra.Command{
	Use:   "health",
	Short: "檢查 PCAI 運作環境與背景任務狀態",
	Run: func(cmd *cobra.Command, args []string) {
		cfg := config.LoadConfig()
		fmt.Println(headerStyle.Render("\n🔍 PCAI 系統健康檢查報告"))
		fmt.Println()

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
		home, _ := os.Getwd()
		kPath := filepath.Join(home, "botmemory", "knowledge", "knowledge.md")
		fmt.Print(labelStyle.Render("長期記憶 (RAG):"))
		if info, err := os.Stat(kPath); err == nil {
			sizeKB := float64(info.Size()) / 1024
			fmt.Printf("%s (大小: %.2f KB)\n", successStyle.Render("● 正常"), sizeKB)

			// --- 新增標籤統計顯示 ---
			fmt.Print(labelStyle.Render(" └ 標籤統計:"))
			fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("246")).Render(tools.GetKnowledgeStats()))

			// --- 執行自動備份 ---
			backupMsg, err := tools.AutoBackupKnowledge()
			fmt.Print(labelStyle.Render(" └ 自動備份:"))
			if err != nil {
				fmt.Println(failStyle.Render("○ 失敗: " + err.Error()))
			} else {
				fmt.Println(successStyle.Render("● " + backupMsg))
			}
		} else {
			fmt.Println(warnStyle.Render("○ " + kPath + " 尚未建立 (累積對話後將自動生成)"))
		}

		// 4. 背景任務統計 (BackgroundManager 整合)
		fmt.Print(labelStyle.Render("背景任務狀態:"))
		if GlobalBgMgr == nil {
			// 如果是獨立執行 pcai health 而非在 chat 中呼叫
			fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("242")).Render("服務未啟動 (僅在 chat 模式下追蹤)"))
		} else {
			summary := GlobalBgMgr.GetTaskSummary()
			if strings.Contains(summary, "執行中") && !strings.HasPrefix(summary, "0") {
				fmt.Println(warnStyle.Render("● " + summary))
			} else {
				fmt.Println(successStyle.Render("○ " + summary))
			}
		}

		// 5. 系統架構資訊
		fmt.Print(labelStyle.Render("系統環境:"))
		fmt.Printf("%s/%s (Optimized)\n", os.Getenv("GOOS"), os.Getenv("GOARCH"))

		fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("242")).Render("\n提示: 輸入 'pcai chat' 進入對話模式後，可使用 'list_tasks' 取得詳細清單。"))
		fmt.Println()

	},
}

func runHealthCheck(bgMgr *tools.BackgroundManager) {
	// ... 原本的 CPU/記憶體檢查代碼 ...

	taskSummary := bgMgr.GetTaskSummary()

	// 根據是否有任務在跑，給予不同的顏色
	statusColor := "10" // 綠色
	if strings.Contains(taskSummary, "執行中") && !strings.HasPrefix(taskSummary, "0") {
		statusColor = "11" // 黃色 (代表正在忙碌)
	}

	// 輸出格式化結果
	fmt.Printf("%-15s %s\n", "背景任務狀態:",
		lipgloss.NewStyle().Foreground(lipgloss.Color(statusColor)).Render(taskSummary))
}

func getPing(url string) string {
	start := time.Now()
	if ollama.CheckService(url) {
		duration := time.Since(start)
		return fmt.Sprintf("%v", duration.Round(time.Millisecond))
	}
	return "連線超時"
}

func init() {
	rootCmd.AddCommand(healthCmd)
}
