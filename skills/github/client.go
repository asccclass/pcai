// 負責在 GitHub 雲端建立 Repository。這裡使用 google/go-github
package github

import (
	"context"

	"github.com/google/go-github/v60/github"
	"golang.org/x/oauth2"
)

type GitHubClient struct {
	client *github.Client
	ctx    context.Context
}

func NewGitHubClient(token string) *GitHubClient {
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(ctx, ts)
	return &GitHubClient{
		client: github.NewClient(tc),
		ctx:    ctx,
	}
}

// CreateRemoteRepo 建立遠端倉庫並回傳 Clone URL
func (g *GitHubClient) CreateRemoteRepo(cfg RepoConfig) (string, error) {
	repo := &github.Repository{
		Name:        github.String(cfg.RepoName),
		Description: github.String(cfg.Description),
		Private:     github.Bool(cfg.Private),
	}
	res, _, err := g.client.Repositories.Create(g.ctx, "", repo)
	if err != nil {
		return "", err
	}
	return res.GetSSHURL(), nil // 針對您的環境，優先使用 SSH URL
}
