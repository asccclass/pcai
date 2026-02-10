package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

// 取得 Token (如果本地沒有 token.json，則會引導使用者進行網頁授權)
func getClient(config *oauth2.Config) *http.Client {
	tokFile := "token.json"
	tok, err := tokenFromFile(tokFile)
	if err != nil {
		tok = getTokenFromWeb(config)
		saveToken(tokFile, tok)
	}
	return config.Client(context.Background(), tok)
}

// 請求網頁授權
func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("請點擊連結授權並複製代碼: \n%v\n", authURL)

	// 改用 bufio 讀取一整行，避免 Malformed 錯誤
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("貼上授權碼並按 Enter: ")
	authCode, _ := reader.ReadString('\n')
	authCode = strings.TrimSpace(authCode) // 移除頭尾換行與空白

	tok, err := config.Exchange(context.TODO(), authCode)
	if err != nil {
		log.Fatalf("無法換取 Token: %v", err)
	}
	return tok
}
func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

func saveToken(path string, token *oauth2.Token) {
	fmt.Printf("儲存認證檔案至: %s\n", path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("無法儲存 Token: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}

func main() {
	ctx := context.Background()
	b, err := os.ReadFile("credentials.json")
	if err != nil {
		log.Fatalf("無法讀取憑證檔案: %v", err)
	}

	// 設定權限範圍 (這裡使用 Sheets 唯讀權限)
	config, err := google.ConfigFromJSON(b, sheets.SpreadsheetsReadonlyScope)
	if err != nil {
		log.Fatalf("解析憑證失敗: %v", err)
	}
	client := getClient(config)

	// 初始化 Sheets 服務
	srv, err := sheets.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		log.Fatalf("無法初始化 Sheets 服務: %v", err)
	}

	// 填入你的試算表 ID (從網址列取得)
	spreadsheetId := "你的_SPREADSHEET_ID"
	readRange := "Sheet1!A1:B5"
	resp, err := srv.Spreadsheets.Values.Get(spreadsheetId, readRange).Do()
	if err != nil {
		log.Fatalf("無法讀取資料: %v", err)
	}

	if len(resp.Values) == 0 {
		fmt.Println("未找到資料。")
	} else {
		fmt.Println("資料內容:")
		for _, row := range resp.Values {
			fmt.Printf("%v, %v\n", row[0], row[1])
		}
	}
}
