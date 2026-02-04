package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// CodeParts 定義 LLM 生成的代碼片段
type CodeParts struct {
	Fields  string `json:"fields"`
	Methods string `json:"methods"`
}

// SkillConfig 定義產生範本所需的變數
type SkillConfig struct {
	PackageName string // 例如: "reminders"
	StructName  string // 例如: "ReminderSkill"
	Fields      string // for smart generation
	Methods     string // for smart generation
}

// LLMCodeGenerator 定義了 PCAI 呼叫 LLM 生成代碼的接口
type LLMCodeGenerator interface {
	GenerateCode(ctx context.Context, prompt string) (string, error)
}

func GenerateSmartSkill(ctx context.Context, llm LLMCodeGenerator, skillName string, userRequirement string) error {
	// 1. 請求 LLM 生成特定的 Data Struct 和 邏輯函數
	prompt := fmt.Sprintf(`
	你是一個 Go 專家。請為名為 "%s" 的 Skill 寫出核心邏輯。
	需求描述: %s
	請只提供兩個部分：
	1. DataFields: 定義在 Struct 裡的欄位 (例如: Amount int)
	2. BusinessLogic: 一個具體的方法 (例如: func (s *Skill) AddWater(ml int))
	回傳 JSON 格式：{"fields": "...", "methods": "..."}
	`, skillName, userRequirement)

	jsonStr, err := llm.GenerateCode(ctx, prompt)
	if err != nil {
		return err
	}

	// 清理可能的 markdown 標記 (```json ... ```)
	cleaned := strings.TrimSpace(jsonStr)
	cleaned = strings.TrimPrefix(cleaned, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```") // sometimes just ```
	cleaned = strings.TrimSuffix(cleaned, "```")

	var parts CodeParts
	if err := json.Unmarshal([]byte(cleaned), &parts); err != nil {
		// Fallback: 如果解析失敗，把所有內容當作方法註解
		parts.Methods = "// Parse Error: " + err.Error() + "\n/*\n" + jsonStr + "\n*/"
	}

	return saveToFiles(skillName, parts)
}

func saveToFiles(skillName string, parts CodeParts) error {
	pkgName := strings.ToLower(skillName)
	structName := cases.Title(language.English).String(skillName) + "Skill"
	basePath := filepath.Join("skills", pkgName)

	if err := os.MkdirAll(basePath, 0755); err != nil {
		return err
	}

	config := SkillConfig{
		PackageName: pkgName,
		StructName:  structName,
		Fields:      parts.Fields,
		Methods:     parts.Methods,
	}

	// 更新後的範本
	templates := map[string]string{
		"init.go":        initTemplate,
		"persistence.go": persistenceTemplate,
	}

	for fileName, content := range templates {
		filePath := filepath.Join(basePath, fileName)
		tmpl, _ := template.New(fileName).Parse(content)

		f, err := os.Create(filePath)
		if err != nil {
			return err
		}
		defer f.Close()
		tmpl.Execute(f, config)
	}

	fmt.Printf("✅ 已成功為 %s (Smart) 產生腳手架於 %s\n", skillName, basePath)
	return nil
}

// GenerateSkillScaffold 產生 Skill 的基礎檔案結構
func GenerateSkillScaffold(skillName string) error {
	pkgName := strings.ToLower(skillName)
	structName := strings.Title(skillName) + "Skill"
	basePath := filepath.Join("skills", pkgName)

	// 1. 建立目錄
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return err
	}

	config := SkillConfig{PackageName: pkgName, StructName: structName}

	// 2. 定義範本
	templates := map[string]string{
		"init.go":        initTemplate,
		"persistence.go": persistenceTemplate,
	}

	// 3. 渲染並寫入檔案
	for fileName, content := range templates {
		filePath := filepath.Join(basePath, fileName)
		tmpl, _ := template.New(fileName).Parse(content)

		f, err := os.Create(filePath)
		if err != nil {
			return err
		}
		defer f.Close()
		tmpl.Execute(f, config)
	}

	fmt.Printf("✅ 已成功為 %s 產生腳手架於 %s\n", skillName, basePath)
	return nil
}

// --- 範本定義 ---

const initTemplate = `package {{.PackageName}}

import "fmt"

type {{.StructName}} struct {
	Data []interface{}
	{{.Fields}}
}

// New{{.StructName}} 初始化技能並載入持久化數據
func New{{.StructName}}() *{{.StructName}} {
	s := &{{.StructName}}{}
	s.LoadFromDB()
	fmt.Println("{{.StructName}} 已啟動並完成數據加載")
	return s
}

func (s *{{.StructName}}) Register(agent interface{}) {
	// 在此處向 PCAI 註冊 Tool
}

// --- Business Logic ---
{{.Methods}}
`

const persistenceTemplate = `package {{.PackageName}}

import (
	"encoding/json"
	"os"
)

const dbPath = "data/{{.PackageName}}.json"

// Save 儲存狀態至本地
func (s *{{.StructName}}) Save() error {
	data, _ := json.Marshal(s.Data)
	return os.WriteFile(dbPath, data, 0644)
}

// LoadFromDB 從本地載入狀態
func (s *{{.StructName}}) LoadFromDB() error {
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return nil
	}
	fileData, err := os.ReadFile(dbPath)
	if err != nil {
		return err
	}
	return json.Unmarshal(fileData, &s.Data)
}
`
