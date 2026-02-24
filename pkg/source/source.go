package source

import (
	"context"
	"time"
)

// SourceType identifies which platform an item came from.
type SourceType string

const (
	SourceHackerNews SourceType = "hackernews"
	SourceGitHub     SourceType = "github"
	SourceReddit     SourceType = "reddit"
	SourceArXiv      SourceType = "arxiv"
	SourceTwitter    SourceType = "twitter"
	SourceYouTube    SourceType = "youtube"
	SourceRSS        SourceType = "rss"
)

// Item is the standardized data model for all sources.
type Item struct {
	ID          string         `json:"id" db:"id"`
	Source      SourceType     `json:"source" db:"source"`
	ExternalID  string         `json:"external_id" db:"external_id"`
	Title       string         `json:"title" db:"title"`
	URL         string         `json:"url" db:"url"`
	Description string         `json:"description" db:"description"`
	Author      string         `json:"author" db:"author"`
	Score       int            `json:"score" db:"score"`
	Comments    int            `json:"comments" db:"comments"`
	Tags        []string       `json:"tags" db:"-"`
	PublishedAt time.Time      `json:"published_at" db:"published_at"`
	CollectedAt time.Time      `json:"collected_at" db:"collected_at"`
	Extra       map[string]any `json:"extra,omitempty" db:"-"`
	TagsJSON    string         `json:"-" db:"tags"`
	ExtraJSON   string         `json:"-" db:"extra"`
}

// Source is the interface every collector must implement.
type Source interface {
	Name() SourceType
	Collect(ctx context.Context) ([]Item, error)
}

// AllSourceTypes returns all known source types.
func AllSourceTypes() []SourceType {
	return []SourceType{
		SourceHackerNews,
		SourceGitHub,
		SourceReddit,
		SourceArXiv,
		SourceTwitter,
		SourceYouTube,
		SourceRSS,
	}
}
