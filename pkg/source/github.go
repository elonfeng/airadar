package source

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// GitHub collects trending AI repositories from GitHub.
type GitHub struct {
	client *http.Client
	token  string
}

// NewGitHub creates a new GitHub collector.
func NewGitHub(token string) *GitHub {
	return &GitHub{
		client: &http.Client{Timeout: 30 * time.Second},
		token:  token,
	}
}

func (g *GitHub) Name() SourceType { return SourceGitHub }

func (g *GitHub) Collect(ctx context.Context) ([]Item, error) {
	// Search for AI-related repos created in the last 7 days, sorted by stars.
	since := time.Now().AddDate(0, 0, -7).Format("2006-01-02")
	query := fmt.Sprintf("created:>%s (topic:ai OR topic:llm OR topic:machine-learning OR topic:deep-learning OR topic:gpt OR topic:transformer OR topic:chatgpt)", since)

	params := url.Values{}
	params.Set("q", query)
	params.Set("sort", "stars")
	params.Set("order", "desc")
	params.Set("per_page", "50")

	reqURL := "https://api.github.com/search/repositories?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create github request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github+json")
	if g.token != "" {
		req.Header.Set("Authorization", "Bearer "+g.token)
	}

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch github trending: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github API status %d", resp.StatusCode)
	}

	var result ghSearchResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode github response: %w", err)
	}

	var items []Item
	for _, repo := range result.Items {
		tags := repo.Topics
		if repo.Language != "" {
			tags = append(tags, repo.Language)
		}

		items = append(items, Item{
			ID:         fmt.Sprintf("github:%s", repo.FullName),
			Source:     SourceGitHub,
			ExternalID: repo.FullName,
			Title:      repo.FullName,
			URL:        repo.HTMLURL,
			Description: repo.Description,
			Author:     repo.Owner.Login,
			Score:      repo.Stars,
			Comments:   repo.Forks,
			Tags:       tags,
			PublishedAt: repo.CreatedAt,
			CollectedAt: time.Now().UTC(),
			Extra: map[string]any{
				"language":    repo.Language,
				"open_issues": repo.OpenIssues,
				"watchers":    repo.Watchers,
			},
		})
	}

	return items, nil
}

type ghSearchResult struct {
	TotalCount int      `json:"total_count"`
	Items      []ghRepo `json:"items"`
}

type ghRepo struct {
	FullName    string    `json:"full_name"`
	HTMLURL     string    `json:"html_url"`
	Description string    `json:"description"`
	Stars       int       `json:"stargazers_count"`
	Forks       int       `json:"forks_count"`
	Watchers    int       `json:"watchers_count"`
	OpenIssues  int       `json:"open_issues_count"`
	Language    string    `json:"language"`
	Topics      []string  `json:"topics"`
	CreatedAt   time.Time `json:"created_at"`
	Owner       ghOwner   `json:"owner"`
}

type ghOwner struct {
	Login string `json:"login"`
}
