// 檔案: pcai.go (或是 main.go)
package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/asccclass/pcai/tools"
	"github.com/joho/godotenv"
)

func main() {
	// 0. 載入環境變數
	currentDir, err := os.Getwd()
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	if err := godotenv.Load(currentDir + "/envfile"); err != nil {
		fmt.Println("Warning: envfile not found, using defaults or system env.")
	}
	// 設定 IP
	if host := os.Getenv("OllamaHost"); host != "" {
		os.Setenv("OLLAMA_HOST", host)
	}

	// 1. 初始化工具箱
	registry := tools.NewRegistry()
	registry.Register(&tools.ListFilesTool{})
	registry.Register(&tools.TimeTool{})
	registry.Register(&tools.VideoConverterTool{})

	// 2. 初始化對話 Session (包含記憶與工具)
	session, err := NewChatSession(registry)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("\n=== PCAI Agent (輸入 'exit' 或 'quit' 離開) ===")

	// 3. 進入互動模式
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("\nUser: ")
		if !scanner.Scan() {
			break
		}
		userInput := strings.TrimSpace(scanner.Text())

		if userInput == "exit" || userInput == "quit" {
			fmt.Println("Bye!")
			break
		}
		if userInput == "" {
			continue
		}

		response, err := session.Chat(userInput)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			continue
		}
		fmt.Printf("AI: %s\n", response)
	}
}
