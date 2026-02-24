package trend

// This file contains additional scoring utilities used by the engine.
// The main scoring logic is in engine.go's scoreCluster method.

// NormalizeScore normalizes a raw score to 0-100 range based on source type.
func NormalizeScore(score int, sourceType string) float64 {
	// Different sources have different score scales:
	// - HN: 1-5000+ (500 is high)
	// - Reddit: 1-100k+ (1000 is high for AI subs)
	// - GitHub: stars 0-100k+ (100 new stars/week is high)
	// - YouTube: views 0-millions (10k is decent for AI)
	// - ArXiv/RSS/Twitter: no native scores

	thresholds := map[string]float64{
		"hackernews": 500,
		"reddit":     1000,
		"github":     100,
		"youtube":    10000,
	}

	threshold, ok := thresholds[sourceType]
	if !ok || threshold == 0 {
		return 0
	}

	ratio := float64(score) / threshold
	if ratio > 1 {
		return 100
	}
	return ratio * 100
}
