package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the root configuration.
type Config struct {
	Database DatabaseConfig `yaml:"database"`
	Schedule ScheduleConfig `yaml:"schedule"`
	Sources  SourcesConfig  `yaml:"sources"`
	Trend    TrendConfig    `yaml:"trend"`
	Alerts   AlertsConfig   `yaml:"alerts"`
	Server   ServerConfig   `yaml:"server"`
	Filter   FilterConfig   `yaml:"filter"`
}

// DatabaseConfig configures SQLite storage.
type DatabaseConfig struct {
	Path string `yaml:"path"`
}

// ScheduleConfig configures collection and trend detection intervals.
type ScheduleConfig struct {
	CollectInterval string `yaml:"collect_interval"`
	TrendInterval   string `yaml:"trend_interval"`
}

// ParseCollectInterval returns the collect interval as time.Duration.
func (s ScheduleConfig) ParseCollectInterval() time.Duration {
	d, err := time.ParseDuration(s.CollectInterval)
	if err != nil {
		return 15 * time.Minute
	}
	return d
}

// ParseTrendInterval returns the trend interval as time.Duration.
func (s ScheduleConfig) ParseTrendInterval() time.Duration {
	d, err := time.ParseDuration(s.TrendInterval)
	if err != nil {
		return 30 * time.Minute
	}
	return d
}

// SourcesConfig holds configuration for all data sources.
type SourcesConfig struct {
	HackerNews HackerNewsConfig `yaml:"hackernews"`
	GitHub     GitHubConfig     `yaml:"github"`
	Reddit     RedditConfig     `yaml:"reddit"`
	ArXiv      ArXivConfig      `yaml:"arxiv"`
	Twitter    TwitterConfig    `yaml:"twitter"`
	YouTube    YouTubeConfig    `yaml:"youtube"`
	RSS        RSSConfig        `yaml:"rss"`
}

// HackerNewsConfig for Hacker News collector.
type HackerNewsConfig struct {
	Enabled bool `yaml:"enabled"`
	Limit   int  `yaml:"limit"`
}

// GitHubConfig for GitHub trending collector.
type GitHubConfig struct {
	Enabled bool   `yaml:"enabled"`
	Token   string `yaml:"token"`
}

// RedditConfig for Reddit collector.
type RedditConfig struct {
	Enabled      bool     `yaml:"enabled"`
	ClientID     string   `yaml:"client_id"`
	ClientSecret string   `yaml:"client_secret"`
	Subreddits   []string `yaml:"subreddits"`
}

// ArXivConfig for ArXiv collector.
type ArXivConfig struct {
	Enabled    bool     `yaml:"enabled"`
	Categories []string `yaml:"categories"`
	MaxResults int      `yaml:"max_results"`
}

// TwitterConfig for Twitter/X collector.
type TwitterConfig struct {
	Enabled   bool     `yaml:"enabled"`
	NitterURL string   `yaml:"nitter_url"`
	Accounts  []string `yaml:"accounts"`
}

// YouTubeConfig for YouTube collector.
type YouTubeConfig struct {
	Enabled  bool     `yaml:"enabled"`
	APIKey   string   `yaml:"api_key"`
	Queries  []string `yaml:"queries"`
	Channels []string `yaml:"channels"`
}

// RSSConfig for RSS feed collector.
type RSSConfig struct {
	Enabled bool       `yaml:"enabled"`
	Feeds   []FeedItem `yaml:"feeds"`
}

// FeedItem is a single RSS feed entry.
type FeedItem struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
}

// TrendConfig configures trend detection.
type TrendConfig struct {
	MinScore          float64   `yaml:"min_score"`
	VelocityWeight    float64   `yaml:"velocity_weight"`
	CrossSourceWeight float64   `yaml:"cross_source_weight"`
	AbsoluteWeight    float64   `yaml:"absolute_weight"`
	LLM               LLMConfig `yaml:"llm"`
}

// LLMConfig configures the optional LLM batch evaluator.
type LLMConfig struct {
	Enabled  bool    `yaml:"enabled"`
	Provider string  `yaml:"provider"` // "openai" or "anthropic"
	Model    string  `yaml:"model"`
	APIKey   string  `yaml:"api_key"`
	BaseURL  string  `yaml:"base_url"`  // custom endpoint (optional)
	MinScore float64 `yaml:"min_score"` // LLM relevance threshold 0-10 (default: 6)
}

// AlertsConfig configures alert destinations.
type AlertsConfig struct {
	Slack   SlackConfig   `yaml:"slack"`
	Discord DiscordConfig `yaml:"discord"`
	Webhook WebhookConfig `yaml:"webhook"`
}

// SlackConfig for Slack webhook alerts.
type SlackConfig struct {
	Enabled    bool   `yaml:"enabled"`
	WebhookURL string `yaml:"webhook_url"`
}

// DiscordConfig for Discord webhook alerts.
type DiscordConfig struct {
	Enabled    bool   `yaml:"enabled"`
	WebhookURL string `yaml:"webhook_url"`
}

// WebhookConfig for generic webhook alerts.
type WebhookConfig struct {
	Enabled bool   `yaml:"enabled"`
	URL     string `yaml:"url"`
	Secret  string `yaml:"secret"`
}

// ServerConfig configures the HTTP server.
type ServerConfig struct {
	Port int `yaml:"port"`
}

// FilterConfig configures content filtering.
type FilterConfig struct {
	ExtraKeywords   []string `yaml:"extra_keywords"`
	ExcludeKeywords []string `yaml:"exclude_keywords"`
}

// Default returns a Config with sensible defaults.
func Default() *Config {
	return &Config{
		Database: DatabaseConfig{Path: "./airadar.db"},
		Schedule: ScheduleConfig{
			CollectInterval: "15m",
			TrendInterval:   "30m",
		},
		Sources: SourcesConfig{
			HackerNews: HackerNewsConfig{Enabled: true, Limit: 100},
			GitHub:     GitHubConfig{Enabled: true},
			Reddit:     RedditConfig{
				Enabled: false,
				Subreddits: []string{
					"MachineLearning", "artificial", "LocalLLM",
					"singularity", "ChatGPT", "StableDiffusion",
				},
			},
			ArXiv: ArXivConfig{
				Enabled:    true,
				Categories: []string{"cs.AI", "cs.CL", "cs.CV", "cs.LG"},
				MaxResults: 50,
			},
			Twitter: TwitterConfig{
				Enabled:   false,
				NitterURL: "https://nitter.net",
			},
			YouTube: YouTubeConfig{
				Enabled: false,
				Queries: []string{"AI news", "LLM", "artificial intelligence"},
			},
			RSS: RSSConfig{
				Enabled: true,
				Feeds: []FeedItem{
					{Name: "TechCrunch AI", URL: "https://techcrunch.com/category/artificial-intelligence/feed/"},
					{Name: "The Verge AI", URL: "https://www.theverge.com/rss/ai-artificial-intelligence/index.xml"},
					{Name: "Ars Technica", URL: "https://feeds.arstechnica.com/arstechnica/technology-lab"},
					{Name: "VentureBeat AI", URL: "https://venturebeat.com/category/ai/feed/"},
				},
			},
		},
		Trend: TrendConfig{
			MinScore:          30,
			VelocityWeight:    0.3,
			CrossSourceWeight: 0.5,
			AbsoluteWeight:    0.2,
			LLM: LLMConfig{
				Provider: "openai",
				Model:    "gpt-4o-mini",
				MinScore: 6,
			},
		},
		Alerts: AlertsConfig{},
		Server: ServerConfig{Port: 8080},
	}
}

// Load reads configuration from a YAML file and applies env var overrides.
func Load(path string) (*Config, error) {
	cfg := Default()

	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read config %s: %w", path, err)
		}
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parse config %s: %w", path, err)
		}
	}

	applyEnvOverrides(cfg)
	return cfg, nil
}

// applyEnvOverrides overrides config values with environment variables.
func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("AIRADAR_DB_PATH"); v != "" {
		cfg.Database.Path = v
	}
	if v := os.Getenv("GITHUB_TOKEN"); v != "" {
		cfg.Sources.GitHub.Token = v
	}
	if v := os.Getenv("REDDIT_CLIENT_ID"); v != "" {
		cfg.Sources.Reddit.ClientID = v
	}
	if v := os.Getenv("REDDIT_CLIENT_SECRET"); v != "" {
		cfg.Sources.Reddit.ClientSecret = v
	}
	if v := os.Getenv("YOUTUBE_API_KEY"); v != "" {
		cfg.Sources.YouTube.APIKey = v
	}
	if v := os.Getenv("SLACK_WEBHOOK_URL"); v != "" {
		cfg.Alerts.Slack.WebhookURL = v
		cfg.Alerts.Slack.Enabled = true
	}
	if v := os.Getenv("DISCORD_WEBHOOK_URL"); v != "" {
		cfg.Alerts.Discord.WebhookURL = v
		cfg.Alerts.Discord.Enabled = true
	}
	if v := os.Getenv("OPENAI_API_KEY"); v != "" {
		cfg.Trend.LLM.APIKey = v
		cfg.Trend.LLM.Enabled = true
		cfg.Trend.LLM.Provider = "openai"
	}
	if v := os.Getenv("ANTHROPIC_API_KEY"); v != "" {
		cfg.Trend.LLM.APIKey = v
		cfg.Trend.LLM.Enabled = true
		cfg.Trend.LLM.Provider = "anthropic"
	}
}
