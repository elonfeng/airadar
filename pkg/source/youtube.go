package source

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// YouTube collects trending AI videos from YouTube.
type YouTube struct {
	client   *http.Client
	apiKey   string
	queries  []string
	channels []string
}

// NewYouTube creates a new YouTube collector.
func NewYouTube(apiKey string, queries, channels []string) *YouTube {
	if len(queries) == 0 {
		queries = []string{"AI news", "LLM", "artificial intelligence"}
	}
	return &YouTube{
		client:   &http.Client{Timeout: 30 * time.Second},
		apiKey:   apiKey,
		queries:  queries,
		channels: channels,
	}
}

func (y *YouTube) Name() SourceType { return SourceYouTube }

func (y *YouTube) Collect(ctx context.Context) ([]Item, error) {
	if y.apiKey == "" {
		return nil, fmt.Errorf("youtube: API key required (set YOUTUBE_API_KEY)")
	}

	var allItems []Item

	// Search for each query.
	for _, query := range y.queries {
		items, err := y.search(ctx, query)
		if err != nil {
			fmt.Printf("  youtube query %q error: %v\n", query, err)
			continue
		}
		allItems = append(allItems, items...)
	}

	// Fetch statistics for all found videos.
	if len(allItems) > 0 {
		y.enrichWithStats(ctx, allItems)
	}

	return allItems, nil
}

func (y *YouTube) search(ctx context.Context, query string) ([]Item, error) {
	publishedAfter := time.Now().Add(-24 * time.Hour).Format(time.RFC3339)

	params := url.Values{}
	params.Set("part", "snippet")
	params.Set("q", query)
	params.Set("type", "video")
	params.Set("order", "viewCount")
	params.Set("publishedAfter", publishedAfter)
	params.Set("maxResults", "20")
	params.Set("key", y.apiKey)

	reqURL := "https://www.googleapis.com/youtube/v3/search?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create youtube search request: %w", err)
	}

	resp, err := y.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch youtube search: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("youtube search status %d", resp.StatusCode)
	}

	var result ytSearchResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode youtube search: %w", err)
	}

	var items []Item
	for _, item := range result.Items {
		videoID := item.ID.VideoID
		if videoID == "" {
			continue
		}

		published := item.Snippet.PublishedAt
		if published.IsZero() {
			published = time.Now().UTC()
		}

		items = append(items, Item{
			ID:          fmt.Sprintf("youtube:%s", videoID),
			Source:      SourceYouTube,
			ExternalID:  videoID,
			Title:       item.Snippet.Title,
			URL:         fmt.Sprintf("https://www.youtube.com/watch?v=%s", videoID),
			Description: truncate(item.Snippet.Description, 500),
			Author:      item.Snippet.ChannelTitle,
			PublishedAt: published,
			CollectedAt: time.Now().UTC(),
			Extra: map[string]any{
				"channel_id": item.Snippet.ChannelID,
				"query":      query,
			},
		})
	}

	return items, nil
}

func (y *YouTube) enrichWithStats(ctx context.Context, items []Item) {
	// Collect all video IDs.
	var ids []string
	idMap := make(map[string]int)
	for i, item := range items {
		ids = append(ids, item.ExternalID)
		idMap[item.ExternalID] = i
	}

	// Batch fetch statistics (max 50 per request).
	for start := 0; start < len(ids); start += 50 {
		end := start + 50
		if end > len(ids) {
			end = len(ids)
		}

		batch := ids[start:end]
		params := url.Values{}
		params.Set("part", "statistics")
		params.Set("id", strings.Join(batch, ","))
		params.Set("key", y.apiKey)

		reqURL := "https://www.googleapis.com/youtube/v3/videos?" + params.Encode()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
		if err != nil {
			continue
		}

		resp, err := y.client.Do(req)
		if err != nil {
			continue
		}

		var result ytVideoResult
		json.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()

		for _, video := range result.Items {
			if idx, ok := idMap[video.ID]; ok {
				items[idx].Score = video.Statistics.ViewCount
				items[idx].Comments = video.Statistics.CommentCount
			}
		}
	}
}

type ytSearchResult struct {
	Items []struct {
		ID struct {
			VideoID string `json:"videoId"`
		} `json:"id"`
		Snippet ytSnippet `json:"snippet"`
	} `json:"items"`
}

type ytSnippet struct {
	Title        string    `json:"title"`
	Description  string    `json:"description"`
	ChannelTitle string    `json:"channelTitle"`
	ChannelID    string    `json:"channelId"`
	PublishedAt  time.Time `json:"publishedAt"`
}

type ytVideoResult struct {
	Items []struct {
		ID         string `json:"id"`
		Statistics struct {
			ViewCount    int `json:"viewCount,string"`
			LikeCount    int `json:"likeCount,string"`
			CommentCount int `json:"commentCount,string"`
		} `json:"statistics"`
	} `json:"items"`
}
