// 使用 go-git 實作。使用 SSH 密鑰驗證
package github

import (
	"os"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
)

// getAuth 取得本地 SSH 金鑰驗證
func getAuth() (*ssh.PublicKeys, error) {
	home, _ := os.UserHomeDir()
	sshPath := home + "/.ssh/id_ed25519" // 您之前的 ed25519 格式
	return ssh.NewPublicKeysFromFile("git", sshPath, "")
}

// PushChanges 執行 Add, Commit, Push
func PushChanges(req PushRequest) error {
	r, err := git.PlainOpen(req.LocalPath)
	if err != nil {
		return err
	}

	w, err := r.Worktree()
	if err != nil {
		return err
	}

	// 1. git add .
	_, err = w.Add(".")

	// 2. git commit
	_, err = w.Commit(req.CommitMessage, &git.CommitOptions{
		Author: &object.Signature{
			Name:  "PCAI Agent",
			Email: "pcai@asus-gx10.local",
			When:  time.Now(),
		},
	})

	// 3. git push
	auth, err := getAuth()
	if err != nil {
		return err
	}
	return r.Push(&git.PushOptions{Auth: auth})
}
