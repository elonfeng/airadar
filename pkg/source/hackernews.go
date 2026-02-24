package source

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

const hnBaseURL = "https://hacker-news.firebaseio.com/v0"

// HackerNews collects AI-related stories from Hacker News.
type HackerNews struct {
	client *http.Client
	limit  int
	filter *Filter
}

// NewHackerNews creates a new HN collector.
func NewHackerNews(limit int, filter *Filter) *HackerNews {
	if limit <= 0 {
		limit = 100
	}
	return &HackerNews{
		client: &http.Client{Timeout: 30 * time.Second},
		limit:  limit,
		filter: filter,
	}
}

func (h *HackerNews) Name() SourceType { return SourceHackerNews }

func (h *HackerNews) Collect(ctx context.Context) ([]Item, error) {
	ids, err := h.fetchTopStories(ctx)
	if err != nil {
		return nil, err
	}

	if len(ids) > h.limit {
		ids = ids[:h.limit]
	}

	var (
		mu    sync.Mutex
		items []Item
		wg    sync.WaitGroup
		sem   = make(chan struct{}, 10) // concurrency limit
	)

	for _, id := range ids {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			story, err := h.fetchItem(ctx, id)
			if err != nil || story == nil {
				return
			}

			// Filter for AI-related content.
			text := story.Title + " " + story.URL
			if h.filter != nil && !h.filter.MatchesAI(text) {
				return
			}

			item := Item{
				ID:          fmt.Sprintf("hackernews:%d", story.ID),
				Source:      SourceHackerNews,
				ExternalID:  fmt.Sprintf("%d", story.ID),
				Title:       story.Title,
				URL:         story.URL,
				Author:      story.By,
				Score:       story.Score,
				Comments:    story.Descendants,
				PublishedAt: time.Unix(story.Time, 0).UTC(),
				CollectedAt: time.Now().UTC(),
			}
			if item.URL == "" {
				item.URL = fmt.Sprintf("https://news.ycombinator.com/item?id=%d", story.ID)
			}

			mu.Lock()
			items = append(items, item)
			mu.Unlock()
		}(id)
	}

	wg.Wait()
	return items, nil
}

type hnStory struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	Score       int    `json:"score"`
	By          string `json:"by"`
	Time        int64  `json:"time"`
	Descendants int    `json:"descendants"`
	Type        string `json:"type"`
}

func (h *HackerNews) fetchTopStories(ctx context.Context) ([]int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, hnBaseURL+"/topstories.json", nil)
	if err != nil {
		return nil, fmt.Errorf("create hn request: %w", err)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch hn top stories: %w", err)
	}
	defer resp.Body.Close()

	var ids []int
	if err := json.NewDecoder(resp.Body).Decode(&ids); err != nil {
		return nil, fmt.Errorf("decode hn top stories: %w", err)
	}
	return ids, nil
}

func (h *HackerNews) fetchItem(ctx context.Context, id int) (*hnStory, error) {
	url := fmt.Sprintf("%s/item/%d.json", hnBaseURL, id)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create hn item request: %w", err)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch hn item %d: %w", id, err)
	}
	defer resp.Body.Close()

	var story hnStory
	if err := json.NewDecoder(resp.Body).Decode(&story); err != nil {
		return nil, fmt.Errorf("decode hn item %d: %w", id, err)
	}

	if story.Type != "story" {
		return nil, nil
	}
	return &story, nil
}
