package source

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/mmcdole/gofeed"
)

// Twitter collects AI tweets via Nitter RSS feeds.
type Twitter struct {
	client    *http.Client
	parser    *gofeed.Parser
	nitterURL string
	accounts  []string
}

// NewTwitter creates a new Twitter/X collector using Nitter RSS.
func NewTwitter(nitterURL string, accounts []string) *Twitter {
	if nitterURL == "" {
		nitterURL = "https://nitter.net"
	}
	return &Twitter{
		client:    &http.Client{Timeout: 30 * time.Second},
		parser:    gofeed.NewParser(),
		nitterURL: strings.TrimRight(nitterURL, "/"),
		accounts:  accounts,
	}
}

func (t *Twitter) Name() SourceType { return SourceTwitter }

func (t *Twitter) Collect(ctx context.Context) ([]Item, error) {
	var allItems []Item

	for _, account := range t.accounts {
		items, err := t.collectAccount(ctx, account)
		if err != nil {
			fmt.Printf("  twitter @%s error: %v\n", account, err)
			continue
		}
		allItems = append(allItems, items...)
	}

	return allItems, nil
}

func (t *Twitter) collectAccount(ctx context.Context, account string) ([]Item, error) {
	feedURL := fmt.Sprintf("%s/%s/rss", t.nitterURL, account)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create twitter request @%s: %w", account, err)
	}
	req.Header.Set("User-Agent", "airadar/1.0")

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch twitter @%s: %w", account, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("twitter @%s status %d", account, resp.StatusCode)
	}

	feed, err := t.parser.Parse(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("parse twitter @%s: %w", account, err)
	}

	var items []Item
	cutoff := time.Now().Add(-24 * time.Hour)

	for _, entry := range feed.Items {
		published := time.Now().UTC()
		if entry.PublishedParsed != nil {
			published = entry.PublishedParsed.UTC()
		}

		if published.Before(cutoff) {
			continue
		}

		link := entry.Link
		// Convert nitter link back to twitter.
		link = strings.Replace(link, t.nitterURL, "https://x.com", 1)

		items = append(items, Item{
			ID:          fmt.Sprintf("twitter:%s:%s", account, entry.GUID),
			Source:      SourceTwitter,
			ExternalID:  entry.GUID,
			Title:       truncate(entry.Title, 280),
			URL:         link,
			Description: truncate(entry.Description, 500),
			Author:      account,
			Score:       0,
			PublishedAt: published,
			CollectedAt: time.Now().UTC(),
			Extra: map[string]any{
				"account": account,
			},
		})
	}

	return items, nil
}
