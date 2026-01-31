// 定義共用參數
package github

// RepoConfig 用於定義專案的基本資訊
type RepoConfig struct {
	Owner       string `json:"owner"`
	RepoName    string `json:"repo_name"`
	Description string `json:"description"`
	Private     bool   `json:"private"`
	LocalPath   string `json:"local_path"`
}

// PushRequest 用於自動提交的參數
type PushRequest struct {
	LocalPath     string `json:"local_path"`
	CommitMessage string `json:"commit_message"`
	Branch        string `json:"branch"`
}
