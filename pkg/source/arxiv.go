package source

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// ArXiv collects recent AI papers from ArXiv.
type ArXiv struct {
	client     *http.Client
	categories []string
	maxResults int
}

// NewArXiv creates a new ArXiv collector.
func NewArXiv(categories []string, maxResults int) *ArXiv {
	if len(categories) == 0 {
		categories = []string{"cs.AI", "cs.CL", "cs.CV", "cs.LG"}
	}
	if maxResults <= 0 {
		maxResults = 50
	}
	return &ArXiv{
		client:     &http.Client{Timeout: 30 * time.Second},
		categories: categories,
		maxResults: maxResults,
	}
}

func (a *ArXiv) Name() SourceType { return SourceArXiv }

func (a *ArXiv) Collect(ctx context.Context) ([]Item, error) {
	// Build search query: cat:cs.AI OR cat:cs.CL OR ...
	var parts []string
	for _, cat := range a.categories {
		parts = append(parts, "cat:"+cat)
	}
	query := strings.Join(parts, "+OR+")

	// ArXiv API expects unencoded +OR+ in the search query, so build URL manually.
	reqURL := fmt.Sprintf("https://export.arxiv.org/api/query?search_query=%s&sortBy=submittedDate&sortOrder=descending&max_results=%d",
		query, a.maxResults)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create arxiv request: %w", err)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch arxiv: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("arxiv status %d", resp.StatusCode)
	}

	var feed arxivFeed
	if err := xml.NewDecoder(resp.Body).Decode(&feed); err != nil {
		return nil, fmt.Errorf("decode arxiv: %w", err)
	}

	var items []Item
	for _, entry := range feed.Entries {
		// Extract paper ID from URL (e.g., "http://arxiv.org/abs/2402.12345v1" -> "2402.12345")
		paperID := extractArXivID(entry.ID)

		var tags []string
		for _, cat := range entry.Categories {
			tags = append(tags, cat.Term)
		}

		var authors []string
		for _, a := range entry.Authors {
			authors = append(authors, a.Name)
		}
		author := strings.Join(authors, ", ")

		published := entry.Published
		if published.IsZero() {
			published = time.Now().UTC()
		}

		items = append(items, Item{
			ID:          fmt.Sprintf("arxiv:%s", paperID),
			Source:      SourceArXiv,
			ExternalID:  paperID,
			Title:       strings.TrimSpace(entry.Title),
			URL:         entry.ID,
			Description: truncate(strings.TrimSpace(entry.Summary), 500),
			Author:      author,
			Score:       0, // ArXiv has no upvote system
			Tags:        tags,
			PublishedAt: published,
			CollectedAt: time.Now().UTC(),
			Extra: map[string]any{
				"categories": tags,
			},
		})
	}

	return items, nil
}

func extractArXivID(uri string) string {
	// "http://arxiv.org/abs/2402.12345v1" -> "2402.12345"
	parts := strings.Split(uri, "/abs/")
	if len(parts) == 2 {
		id := parts[1]
		// Remove version suffix.
		if idx := strings.LastIndex(id, "v"); idx > 0 {
			id = id[:idx]
		}
		return id
	}
	return uri
}

// ArXiv Atom feed structures.
type arxivFeed struct {
	XMLName xml.Name     `xml:"feed"`
	Entries []arxivEntry `xml:"entry"`
}

type arxivEntry struct {
	ID         string          `xml:"id"`
	Title      string          `xml:"title"`
	Summary    string          `xml:"summary"`
	Published  time.Time       `xml:"published"`
	Updated    time.Time       `xml:"updated"`
	Authors    []arxivAuthor   `xml:"author"`
	Categories []arxivCategory `xml:"category"`
}

type arxivAuthor struct {
	Name string `xml:"name"`
}

type arxivCategory struct {
	Term string `xml:"term,attr"`
}
