package scheduler

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/elonfeng/airadar/internal/store"
	"github.com/elonfeng/airadar/pkg/alert"
	"github.com/elonfeng/airadar/pkg/source"
	"github.com/elonfeng/airadar/pkg/trend"
)

// Scheduler runs periodic collection and trend detection.
type Scheduler struct {
	store      store.Store
	sources    []source.Source
	engine     *trend.Engine
	alertMgr   *alert.Manager
	collectInt time.Duration
	trendInt   time.Duration
	minScore   float64
}

// New creates a new scheduler.
func New(
	s store.Store,
	sources []source.Source,
	engine *trend.Engine,
	alertMgr *alert.Manager,
	collectInt, trendInt time.Duration,
	minScore float64,
) *Scheduler {
	if collectInt == 0 {
		collectInt = 15 * time.Minute
	}
	if trendInt == 0 {
		trendInt = 30 * time.Minute
	}
	if minScore == 0 {
		minScore = 30
	}
	return &Scheduler{
		store:      s,
		sources:    sources,
		engine:     engine,
		alertMgr:   alertMgr,
		collectInt: collectInt,
		trendInt:   trendInt,
		minScore:   minScore,
	}
}

// Run starts the scheduler loop. Blocks until ctx is cancelled.
func (s *Scheduler) Run(ctx context.Context) error {
	collectTicker := time.NewTicker(s.collectInt)
	trendTicker := time.NewTicker(s.trendInt)
	defer collectTicker.Stop()
	defer trendTicker.Stop()

	// Run immediately on start.
	fmt.Fprintln(os.Stderr, "scheduler: initial collection...")
	s.collectAll(ctx)
	fmt.Fprintln(os.Stderr, "scheduler: initial trend detection...")
	s.detectAndAlert(ctx)

	fmt.Fprintf(os.Stderr, "scheduler: running (collect every %s, trends every %s)\n",
		s.collectInt, s.trendInt)

	for {
		select {
		case <-ctx.Done():
			fmt.Fprintln(os.Stderr, "scheduler: stopped")
			return ctx.Err()
		case <-collectTicker.C:
			fmt.Fprintln(os.Stderr, "scheduler: collecting...")
			s.collectAll(ctx)
		case <-trendTicker.C:
			fmt.Fprintln(os.Stderr, "scheduler: detecting trends...")
			s.detectAndAlert(ctx)
		}
	}
}

func (s *Scheduler) collectAll(ctx context.Context) {
	totalItems := 0
	for _, src := range s.sources {
		items, err := src.Collect(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  %s error: %v\n", src.Name(), err)
			continue
		}

		if err := s.store.UpsertItems(ctx, items); err != nil {
			fmt.Fprintf(os.Stderr, "  %s store error: %v\n", src.Name(), err)
			continue
		}

		// Record score snapshots.
		for i := range items {
			_ = s.store.AddSnapshot(ctx, items[i].ID, items[i].Score, items[i].Comments)
		}

		fmt.Fprintf(os.Stderr, "  %s: %d items\n", src.Name(), len(items))
		totalItems += len(items)
	}
	fmt.Fprintf(os.Stderr, "  total: %d items\n", totalItems)
}

func (s *Scheduler) detectAndAlert(ctx context.Context) {
	trends, err := s.engine.Detect(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  trend detection error: %v\n", err)
		return
	}

	if !s.alertMgr.HasNotifiers() {
		return
	}

	// Alert for high-scoring unalerted trends.
	for _, t := range trends {
		if t.Score < s.minScore || t.Alerted {
			continue
		}

		// Build notification.
		var items []source.Item
		for _, itemID := range t.ItemIDs {
			item, err := s.store.GetItem(ctx, itemID)
			if err == nil && item != nil {
				items = append(items, *item)
			}
		}

		notification := &alert.Notification{
			Title:   t.Topic,
			Body:    fmt.Sprintf("Trending across %d sources with score %.1f", t.SourceCount, t.Score),
			Score:   t.Score,
			Sources: t.ItemIDs,
			Items:   items,
		}

		if err := s.alertMgr.Broadcast(ctx, notification); err != nil {
			fmt.Fprintf(os.Stderr, "  alert error for %q: %v\n", t.Topic, err)
			continue
		}

		_ = s.store.MarkAlerted(ctx, t.ID)
		fmt.Fprintf(os.Stderr, "  alerted: %s (score: %.1f)\n", t.Topic, t.Score)
	}
}
