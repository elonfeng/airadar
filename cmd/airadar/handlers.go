package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/elonfeng/airadar/internal/config"
	"github.com/elonfeng/airadar/internal/scheduler"
	"github.com/elonfeng/airadar/internal/store"
	"github.com/elonfeng/airadar/pkg/alert"
	"github.com/elonfeng/airadar/pkg/server"
	"github.com/elonfeng/airadar/pkg/source"
	"github.com/elonfeng/airadar/pkg/trend"
)

func loadConfig() (*config.Config, error) {
	path := cfgFile
	if path == "" {
		if _, err := os.Stat("config.yaml"); err == nil {
			path = "config.yaml"
		}
	}
	return config.Load(path)
}

func buildEngine(cfg *config.Config, db store.Store) *trend.Engine {
	var llm *trend.LLMEvaluator
	if cfg.Trend.LLM.Enabled && cfg.Trend.LLM.APIKey != "" {
		llm = trend.NewLLMEvaluator(
			cfg.Trend.LLM.Provider,
			cfg.Trend.LLM.Model,
			cfg.Trend.LLM.APIKey,
			cfg.Trend.LLM.BaseURL,
			cfg.Trend.LLM.MinScore,
		)
		fmt.Fprintf(os.Stderr, "llm evaluator: %s/%s (min_score: %.0f)\n",
			cfg.Trend.LLM.Provider, cfg.Trend.LLM.Model, cfg.Trend.LLM.MinScore)
	}
	return trend.NewEngine(db, cfg.Trend.VelocityWeight, cfg.Trend.CrossSourceWeight, cfg.Trend.AbsoluteWeight, llm)
}

func buildSources(cfg *config.Config, filter *source.Filter) []source.Source {
	var sources []source.Source

	if cfg.Sources.HackerNews.Enabled {
		sources = append(sources, source.NewHackerNews(cfg.Sources.HackerNews.Limit, filter))
	}
	if cfg.Sources.GitHub.Enabled {
		sources = append(sources, source.NewGitHub(cfg.Sources.GitHub.Token))
	}
	if cfg.Sources.Reddit.Enabled {
		sources = append(sources, source.NewReddit(
			cfg.Sources.Reddit.ClientID,
			cfg.Sources.Reddit.ClientSecret,
			cfg.Sources.Reddit.Subreddits,
		))
	}
	if cfg.Sources.ArXiv.Enabled {
		sources = append(sources, source.NewArXiv(cfg.Sources.ArXiv.Categories, cfg.Sources.ArXiv.MaxResults))
	}
	if cfg.Sources.Twitter.Enabled {
		sources = append(sources, source.NewTwitter(cfg.Sources.Twitter.NitterURL, cfg.Sources.Twitter.Accounts))
	}
	if cfg.Sources.YouTube.Enabled {
		sources = append(sources, source.NewYouTube(cfg.Sources.YouTube.APIKey, cfg.Sources.YouTube.Queries, cfg.Sources.YouTube.Channels))
	}
	if cfg.Sources.RSS.Enabled {
		feeds := make([]source.RSSFeed, len(cfg.Sources.RSS.Feeds))
		for i, f := range cfg.Sources.RSS.Feeds {
			feeds[i] = source.RSSFeed{Name: f.Name, URL: f.URL}
		}
		sources = append(sources, source.NewRSS(feeds, filter))
	}

	return sources
}

func buildAlertManager(cfg *config.Config) *alert.Manager {
	var notifiers []alert.Notifier

	if cfg.Alerts.Slack.Enabled && cfg.Alerts.Slack.WebhookURL != "" {
		notifiers = append(notifiers, alert.NewSlack(cfg.Alerts.Slack.WebhookURL))
	}
	if cfg.Alerts.Discord.Enabled && cfg.Alerts.Discord.WebhookURL != "" {
		notifiers = append(notifiers, alert.NewDiscord(cfg.Alerts.Discord.WebhookURL))
	}
	if cfg.Alerts.Webhook.Enabled && cfg.Alerts.Webhook.URL != "" {
		notifiers = append(notifiers, alert.NewWebhook(cfg.Alerts.Webhook.URL, cfg.Alerts.Webhook.Secret))
	}

	return alert.NewManager(notifiers)
}

func runCollect(filterSources []string) error {
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	db, err := store.New(cfg.Database.Path)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer db.Close()

	filter := source.NewFilter(cfg.Filter.ExtraKeywords, cfg.Filter.ExcludeKeywords)
	allSources := buildSources(cfg, filter)

	// Filter to requested sources only.
	var sources []source.Source
	if len(filterSources) > 0 {
		wanted := make(map[string]bool)
		for _, s := range filterSources {
			wanted[strings.ToLower(strings.TrimSpace(s))] = true
		}
		for _, s := range allSources {
			name := string(s.Name())
			short := shortName(s.Name())
			if wanted[name] || wanted[short] {
				sources = append(sources, s)
			}
		}
		if len(sources) == 0 {
			return fmt.Errorf("no matching sources for: %s", strings.Join(filterSources, ", "))
		}
	} else {
		sources = allSources
	}

	ctx := context.Background()
	totalItems := 0

	for _, src := range sources {
		fmt.Fprintf(os.Stderr, "collecting from %s...\n", src.Name())
		items, err := src.Collect(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  error: %v\n", err)
			continue
		}

		if err := db.UpsertItems(ctx, items); err != nil {
			fmt.Fprintf(os.Stderr, "  store error: %v\n", err)
			continue
		}

		// Record score snapshots for velocity tracking.
		for i := range items {
			_ = db.AddSnapshot(ctx, items[i].ID, items[i].Score, items[i].Comments)
		}

		fmt.Fprintf(os.Stderr, "  collected %d items\n", len(items))
		totalItems += len(items)
	}

	fmt.Fprintf(os.Stderr, "\ntotal: %d items from %d sources\n", totalItems, len(sources))
	return nil
}

func runTrends(jsonOutput bool, minScore float64, limit int) error {
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	db, err := store.New(cfg.Database.Path)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer db.Close()

	// Run trend detection first.
	engine := buildEngine(cfg, db)
	if _, err := engine.Detect(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "trend detection error: %v\n", err)
	}

	if minScore < 0 {
		minScore = cfg.Trend.MinScore
	}

	trends, err := db.ListTrends(context.Background(), store.TrendListOpts{
		MinScore: minScore,
		Limit:    limit,
	})
	if err != nil {
		return fmt.Errorf("list trends: %w", err)
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(trends)
	}

	if len(trends) == 0 {
		fmt.Println("no trends found (try collecting data first: airadar collect)")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "SCORE\tSOURCES\tTOPIC\tLAST UPDATED")
	for _, t := range trends {
		fmt.Fprintf(w, "%.1f\t%d\t%s\t%s\n",
			t.Score, t.SourceCount, t.Topic,
			t.LastUpdated.Format(time.RFC3339))
	}
	return w.Flush()
}

func runServe(port int) error {
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if port == 0 {
		port = cfg.Server.Port
	}

	db, err := store.New(cfg.Database.Path)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer db.Close()

	engine := buildEngine(cfg, db)
	filter := source.NewFilter(cfg.Filter.ExtraKeywords, cfg.Filter.ExcludeKeywords)
	sources := buildSources(cfg, filter)

	srv := server.New(db, engine, sources, port)
	return srv.ListenAndServe()
}

func runDaemon(port int) error {
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if port == 0 {
		port = cfg.Server.Port
	}

	db, err := store.New(cfg.Database.Path)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer db.Close()

	engine := buildEngine(cfg, db)
	filter := source.NewFilter(cfg.Filter.ExtraKeywords, cfg.Filter.ExcludeKeywords)
	sources := buildSources(cfg, filter)
	alertMgr := buildAlertManager(cfg)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	sched := scheduler.New(db, sources, engine, alertMgr,
		cfg.Schedule.ParseCollectInterval(),
		cfg.Schedule.ParseTrendInterval(),
		cfg.Trend.MinScore,
	)

	// Start scheduler in background.
	go func() {
		if err := sched.Run(ctx); err != nil && ctx.Err() == nil {
			fmt.Fprintf(os.Stderr, "scheduler error: %v\n", err)
		}
	}()

	// Start HTTP server.
	srv := server.New(db, engine, sources, port)
	go func() {
		<-ctx.Done()
		fmt.Fprintln(os.Stderr, "\nshutting down...")
	}()

	return srv.ListenAndServe()
}

func shortName(st source.SourceType) string {
	switch st {
	case source.SourceHackerNews:
		return "hn"
	case source.SourceGitHub:
		return "github"
	case source.SourceReddit:
		return "reddit"
	case source.SourceArXiv:
		return "arxiv"
	case source.SourceTwitter:
		return "twitter"
	case source.SourceYouTube:
		return "youtube"
	case source.SourceRSS:
		return "rss"
	}
	return string(st)
}
