package source

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Reddit collects AI-related posts from Reddit subreddits.
type Reddit struct {
	client       *http.Client
	clientID     string
	clientSecret string
	subreddits   []string
	mu           sync.Mutex
	token        string
	tokenExpiry  time.Time
}

// NewReddit creates a new Reddit collector.
func NewReddit(clientID, clientSecret string, subreddits []string) *Reddit {
	if len(subreddits) == 0 {
		subreddits = []string{
			"MachineLearning", "artificial", "LocalLLM",
			"singularity", "ChatGPT", "StableDiffusion",
		}
	}
	return &Reddit{
		client:       &http.Client{Timeout: 30 * time.Second},
		clientID:     clientID,
		clientSecret: clientSecret,
		subreddits:   subreddits,
	}
}

func (r *Reddit) Name() SourceType { return SourceReddit }

func (r *Reddit) Collect(ctx context.Context) ([]Item, error) {
	if err := r.authenticate(ctx); err != nil {
		return nil, fmt.Errorf("reddit auth: %w", err)
	}

	var allItems []Item
	for _, sub := range r.subreddits {
		items, err := r.fetchSubreddit(ctx, sub)
		if err != nil {
			fmt.Printf("  reddit r/%s error: %v\n", sub, err)
			continue
		}
		allItems = append(allItems, items...)
	}

	return allItems, nil
}

func (r *Reddit) authenticate(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.token != "" && time.Now().Before(r.tokenExpiry) {
		return nil
	}

	data := url.Values{"grant_type": {"client_credentials"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://www.reddit.com/api/v1/access_token",
		strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}

	req.SetBasicAuth(r.clientID, r.clientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "airadar/1.0")

	resp, err := r.client.Do(req)
	if err != nil {
		return fmt.Errorf("reddit token request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("reddit auth status %d", resp.StatusCode)
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return fmt.Errorf("decode reddit token: %w", err)
	}

	r.token = tokenResp.AccessToken
	r.tokenExpiry = time.Now().Add(time.Duration(tokenResp.ExpiresIn-60) * time.Second)
	return nil
}

func (r *Reddit) fetchSubreddit(ctx context.Context, subreddit string) ([]Item, error) {
	reqURL := fmt.Sprintf("https://oauth.reddit.com/r/%s/hot.json?limit=50", subreddit)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+r.token)
	req.Header.Set("User-Agent", "airadar/1.0")

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch r/%s: %w", subreddit, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("reddit r/%s status %d", subreddit, resp.StatusCode)
	}

	var listing redditListing
	if err := json.NewDecoder(resp.Body).Decode(&listing); err != nil {
		return nil, fmt.Errorf("decode r/%s: %w", subreddit, err)
	}

	var items []Item
	for _, child := range listing.Data.Children {
		post := child.Data
		if post.Stickied {
			continue
		}

		postURL := post.URL
		if postURL == "" || strings.HasPrefix(postURL, "/r/") {
			postURL = "https://reddit.com" + post.Permalink
		}

		items = append(items, Item{
			ID:          fmt.Sprintf("reddit:%s", post.ID),
			Source:      SourceReddit,
			ExternalID:  post.ID,
			Title:       post.Title,
			URL:         postURL,
			Description: truncate(post.Selftext, 500),
			Author:      post.Author,
			Score:       post.Score,
			Comments:    post.NumComments,
			Tags:        []string{subreddit},
			PublishedAt: time.Unix(int64(post.CreatedUTC), 0).UTC(),
			CollectedAt: time.Now().UTC(),
			Extra: map[string]any{
				"subreddit": subreddit,
				"upvote_ratio": post.UpvoteRatio,
			},
		})
	}

	return items, nil
}

type redditListing struct {
	Data struct {
		Children []struct {
			Data redditPost `json:"data"`
		} `json:"children"`
	} `json:"data"`
}

type redditPost struct {
	ID          string  `json:"id"`
	Title       string  `json:"title"`
	URL         string  `json:"url"`
	Permalink   string  `json:"permalink"`
	Selftext    string  `json:"selftext"`
	Author      string  `json:"author"`
	Score       int     `json:"score"`
	NumComments int     `json:"num_comments"`
	CreatedUTC  float64 `json:"created_utc"`
	Stickied    bool    `json:"stickied"`
	UpvoteRatio float64 `json:"upvote_ratio"`
}
