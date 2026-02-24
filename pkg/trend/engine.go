package trend

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/elonfeng/airadar/internal/store"
	"github.com/elonfeng/airadar/pkg/source"
)

// Engine detects trending topics from collected items.
type Engine struct {
	store             store.Store
	velocityWeight    float64
	crossSourceWeight float64
	absoluteWeight    float64
	llm               *LLMEvaluator // optional, nil = disabled
}

// NewEngine creates a new trend detection engine.
func NewEngine(s store.Store, velocityW, crossSourceW, absoluteW float64, llm *LLMEvaluator) *Engine {
	if velocityW+crossSourceW+absoluteW == 0 {
		velocityW = 0.3
		crossSourceW = 0.5
		absoluteW = 0.2
	}
	return &Engine{
		store:             s,
		velocityWeight:    velocityW,
		crossSourceWeight: crossSourceW,
		absoluteWeight:    absoluteW,
		llm:               llm,
	}
}

// TopicCluster groups related items from potentially different sources.
type TopicCluster struct {
	Topic       string
	Items       []source.Item
	Sources     map[source.SourceType]bool
	TotalScore  int
	MaxVelocity float64
}

// Detect runs trend detection on recent items and returns new/updated trends.
func (e *Engine) Detect(ctx context.Context) ([]store.Trend, error) {
	// Load items from last 24 hours.
	items, err := e.store.ListItems(ctx, store.ListOpts{
		Since: time.Now().Add(-24 * time.Hour),
		Limit: 1000,
	})
	if err != nil {
		return nil, fmt.Errorf("list recent items: %w", err)
	}

	if len(items) == 0 {
		return nil, nil
	}

	// Clear old trends and regenerate.
	if err := e.store.ClearTrends(ctx); err != nil {
		return nil, fmt.Errorf("clear trends: %w", err)
	}

	// LLM batch evaluation: send all items to LLM in one call,
	// filter out low-value items, and use LLM topics for better clustering.
	if e.llm != nil {
		items, err = e.llmFilter(ctx, items)
		if err != nil {
			fmt.Printf("  llm evaluation error (falling back to algorithm): %v\n", err)
			// Continue with all items if LLM fails.
		}
		if len(items) == 0 {
			return nil, nil
		}
	}

	// Cluster items into topics.
	clusters := e.clusterItems(items)

	// Score each cluster.
	var trends []store.Trend
	now := time.Now().UTC()

	for _, cluster := range clusters {
		score := e.scoreCluster(ctx, cluster)

		trend := store.Trend{
			Topic:       cluster.Topic,
			Score:       score,
			SourceCount: len(cluster.Sources),
			FirstSeen:   now,
			LastUpdated: now,
		}

		for _, item := range cluster.Items {
			trend.ItemIDs = append(trend.ItemIDs, item.ID)
		}

		if err := e.store.UpsertTrend(ctx, &trend); err != nil {
			fmt.Printf("  trend upsert error: %v\n", err)
			continue
		}
		trends = append(trends, trend)
	}

	// Sort by score descending.
	sort.Slice(trends, func(i, j int) bool {
		return trends[i].Score > trends[j].Score
	})

	return trends, nil
}

// llmFilter sends all items to the LLM in one batch call and keeps only high-value ones.
// Also replaces item titles with LLM-generated topic labels for better clustering.
func (e *Engine) llmFilter(ctx context.Context, items []source.Item) ([]source.Item, error) {
	results, err := e.llm.EvaluateItems(ctx, items)
	if err != nil {
		return items, err // return original items on error
	}

	if len(results) == 0 {
		return nil, nil
	}

	// Build lookup: item ID -> LLM result.
	resultMap := make(map[string]LLMResult)
	for _, r := range results {
		resultMap[r.ID] = r
	}

	// Keep only items that passed LLM filter, use LLM topic as title.
	var filtered []source.Item
	for i := range items {
		if r, ok := resultMap[items[i].ID]; ok {
			if r.Topic != "" {
				items[i].Title = r.Topic // use LLM's clean topic label
			}
			filtered = append(filtered, items[i])
		}
	}

	fmt.Printf("  llm: %d/%d items passed evaluation\n", len(filtered), len(items))
	return filtered, nil
}

// clusterItems groups items with similar titles into topic clusters.
func (e *Engine) clusterItems(items []source.Item) []TopicCluster {
	type node struct {
		parent int
	}

	n := len(items)
	uf := make([]node, n)
	for i := range uf {
		uf[i].parent = i
	}

	var find func(int) int
	find = func(x int) int {
		if uf[x].parent != x {
			uf[x].parent = find(uf[x].parent)
		}
		return uf[x].parent
	}
	union := func(x, y int) {
		px, py := find(x), find(y)
		if px != py {
			uf[px].parent = py
		}
	}

	// Tokenize all titles.
	tokens := make([][]string, n)
	for i, item := range items {
		tokens[i] = significantTokens(item.Title)
	}

	// Compare all pairs (O(n²) but n is bounded by 1000).
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			if jaccardSimilarity(tokens[i], tokens[j]) >= 0.3 {
				union(i, j)
			}
		}
	}

	// Group by root.
	groups := make(map[int][]int)
	for i := 0; i < n; i++ {
		root := find(i)
		groups[root] = append(groups[root], i)
	}

	var clusters []TopicCluster
	for _, indices := range groups {
		sources := make(map[source.SourceType]bool)
		var clusterItems []source.Item
		totalScore := 0

		for _, idx := range indices {
			item := items[idx]
			sources[item.Source] = true
			clusterItems = append(clusterItems, item)
			totalScore += item.Score
		}

		// Pick the item with highest score as the topic name.
		best := clusterItems[0]
		for _, item := range clusterItems {
			if item.Score > best.Score {
				best = item
			}
		}

		clusters = append(clusters, TopicCluster{
			Topic:      best.Title,
			Items:      clusterItems,
			Sources:    sources,
			TotalScore: totalScore,
		})
	}

	return clusters
}

// scoreCluster computes a weighted trend score for a topic cluster.
func (e *Engine) scoreCluster(ctx context.Context, cluster TopicCluster) float64 {
	// 1. Cross-source score (0-100): more sources = higher score.
	crossScore := float64(len(cluster.Sources)) * 20
	if crossScore > 100 {
		crossScore = 100
	}

	// 2. Velocity score (0-100): based on score growth rate.
	velocityScore := 0.0
	for _, item := range cluster.Items {
		v := e.itemVelocity(ctx, item)
		if v > velocityScore {
			velocityScore = v
		}
	}
	if velocityScore > 100 {
		velocityScore = 100
	}

	// 3. Absolute score (0-100): normalized by item count and source type.
	absoluteScore := 0.0
	if cluster.TotalScore > 0 {
		// Simple heuristic: log scale for normalization.
		avg := float64(cluster.TotalScore) / float64(len(cluster.Items))
		if avg > 1000 {
			absoluteScore = 100
		} else if avg > 100 {
			absoluteScore = 60 + (avg-100)/900*40
		} else if avg > 10 {
			absoluteScore = 20 + (avg-10)/90*40
		} else {
			absoluteScore = avg / 10 * 20
		}
	}

	return crossScore*e.crossSourceWeight +
		velocityScore*e.velocityWeight +
		absoluteScore*e.absoluteWeight
}

// itemVelocity calculates how fast an item's score is growing.
func (e *Engine) itemVelocity(ctx context.Context, item source.Item) float64 {
	snaps, err := e.store.GetSnapshots(ctx, item.ID, time.Now().Add(-6*time.Hour))
	if err != nil || len(snaps) < 2 {
		return 0
	}

	first := snaps[0]
	last := snaps[len(snaps)-1]
	hours := last.CheckedAt.Sub(first.CheckedAt).Hours()
	if hours < 0.1 {
		return 0
	}

	scoreDelta := float64(last.Score - first.Score)
	velocity := scoreDelta / hours // points per hour

	// Normalize: 100 points/hour is exceptional.
	return velocity
}

// significantTokens extracts meaningful words from a title.
func significantTokens(title string) []string {
	stopwords := map[string]bool{
		"a": true, "an": true, "the": true, "and": true, "or": true,
		"but": true, "in": true, "on": true, "at": true, "to": true,
		"for": true, "of": true, "with": true, "by": true, "from": true,
		"is": true, "are": true, "was": true, "were": true, "be": true,
		"been": true, "being": true, "have": true, "has": true, "had": true,
		"do": true, "does": true, "did": true, "will": true, "would": true,
		"could": true, "should": true, "may": true, "might": true,
		"this": true, "that": true, "these": true, "those": true,
		"it": true, "its": true, "i": true, "we": true, "you": true,
		"he": true, "she": true, "they": true, "my": true, "your": true,
		"how": true, "what": true, "when": true, "where": true, "why": true,
		"not": true, "no": true, "new": true, "just": true, "about": true,
		"up": true, "out": true, "if": true, "so": true, "can": true,
		"all": true, "more": true, "also": true, "than": true, "very": true,
	}

	words := strings.FieldsFunc(strings.ToLower(title), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})

	var tokens []string
	for _, w := range words {
		if len(w) >= 2 && !stopwords[w] {
			tokens = append(tokens, w)
		}
	}
	return tokens
}

// jaccardSimilarity returns the Jaccard index of two token sets.
func jaccardSimilarity(a, b []string) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}

	setA := make(map[string]bool)
	for _, t := range a {
		setA[t] = true
	}

	setB := make(map[string]bool)
	for _, t := range b {
		setB[t] = true
	}

	intersection := 0
	for t := range setA {
		if setB[t] {
			intersection++
		}
	}

	unionSize := len(setA) + len(setB) - intersection
	if unionSize == 0 {
		return 0
	}
	return float64(intersection) / float64(unionSize)
}
