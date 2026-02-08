package skillloader

// ==========================================
// 1. 內部中間結構 (Intermediate Representation)
// ==========================================
type SkillDoc struct {
	Name        string
	Description string
	Parameters  []ParamDoc
}

type ParamDoc struct {
	Name        string
	Type        string
	Required    bool
	Description string
}

// ==========================================
// 2. OpenAI / Gemini Function Schema 結構
// ==========================================
type OpenAITool struct {
	Type     string               `json:"type"`
	Function OpenAIFuncDefinition `json:"function"`
}

type OpenAIFuncDefinition struct {
	Name        string               `json:"name"`
	Description string               `json:"description"`
	Parameters  JSONSchemaDefinition `json:"parameters"`
}

type JSONSchemaDefinition struct {
	Type       string                  `json:"type"`
	Properties map[string]JSONProperty `json:"properties"`
	Required   []string                `json:"required,omitempty"`
}

type JSONProperty struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}
