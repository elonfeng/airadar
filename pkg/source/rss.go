package source

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/mmcdole/gofeed"
)

// RSSFeed is a named RSS/Atom feed URL.
type RSSFeed struct {
	Name string
	URL  string
}

// RSS collects AI news from RSS/Atom feeds.
type RSS struct {
	client *http.Client
	parser *gofeed.Parser
	feeds  []RSSFeed
	filter *Filter
}

// NewRSS creates a new RSS collector.
func NewRSS(feeds []RSSFeed, filter *Filter) *RSS {
	return &RSS{
		client: &http.Client{Timeout: 30 * time.Second},
		parser: gofeed.NewParser(),
		feeds:  feeds,
		filter: filter,
	}
}

func (r *RSS) Name() SourceType { return SourceRSS }

func (r *RSS) Collect(ctx context.Context) ([]Item, error) {
	var allItems []Item

	for _, feed := range r.feeds {
		items, err := r.collectFeed(ctx, feed)
		if err != nil {
			fmt.Printf("  rss feed %s error: %v\n", feed.Name, err)
			continue
		}
		allItems = append(allItems, items...)
	}

	return allItems, nil
}

func (r *RSS) collectFeed(ctx context.Context, feed RSSFeed) ([]Item, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feed.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("create rss request %s: %w", feed.Name, err)
	}
	req.Header.Set("User-Agent", "airadar/1.0")

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch rss %s: %w", feed.Name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("rss %s status %d", feed.Name, resp.StatusCode)
	}

	parsed, err := r.parser.Parse(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("parse rss %s: %w", feed.Name, err)
	}

	var items []Item
	cutoff := time.Now().Add(-24 * time.Hour) // Only last 24h

	for _, entry := range parsed.Items {
		published := time.Now().UTC()
		if entry.PublishedParsed != nil {
			published = entry.PublishedParsed.UTC()
		} else if entry.UpdatedParsed != nil {
			published = entry.UpdatedParsed.UTC()
		}

		// Skip old items.
		if published.Before(cutoff) {
			continue
		}

		// Some RSS feeds are AI-specific (TechCrunch AI), others need filtering.
		text := entry.Title + " " + entry.Description
		if r.filter != nil && !r.filter.MatchesAI(text) {
			continue
		}

		link := entry.Link
		if link == "" && len(entry.Links) > 0 {
			link = entry.Links[0]
		}

		author := ""
		if entry.Author != nil {
			author = entry.Author.Name
		}

		items = append(items, Item{
			ID:          fmt.Sprintf("rss:%s:%s", feed.Name, entry.GUID),
			Source:      SourceRSS,
			ExternalID:  entry.GUID,
			Title:       entry.Title,
			URL:         link,
			Description: truncate(entry.Description, 500),
			Author:      author,
			Score:       0,
			PublishedAt: published,
			CollectedAt: time.Now().UTC(),
			Tags:        entry.Categories,
			Extra: map[string]any{
				"feed_name": feed.Name,
			},
		})
	}

	return items, nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
