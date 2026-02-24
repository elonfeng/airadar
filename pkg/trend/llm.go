package trend

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/elonfeng/airadar/pkg/source"
)

const batchPrompt = `You are an AI trend analyst. Your job is to evaluate a batch of items collected from various sources (Hacker News, GitHub, Reddit, ArXiv, RSS feeds, etc.) and identify which ones represent genuinely important, trending AI products, tools, research, or news.

For each item, assign:
1. "score" (0-10): How important/trending is this for the AI community?
   - 9-10: Breakthrough product launch, major model release, industry-shaking news
   - 7-8: Notable new tool, significant research paper, important industry update
   - 5-6: Interesting but not exceptional, niche AI tool, incremental update
   - 3-4: Tangentially AI-related, low novelty
   - 0-2: Not actually AI-related, spam, or irrelevant noise
2. "reason" (1 sentence): Why this score?
3. "topic" (short phrase): A clean, normalized topic label for grouping (e.g., "Claude 4 Release", "Stable Diffusion 4.0")

IMPORTANT: Be strict. Most items should score 5 or below. Only truly significant items deserve 7+. We want to surface signal, not noise.

Items to evaluate:
%s

Respond with a JSON array. Each element must have: "id" (the item ID), "score" (integer 0-10), "reason" (string), "topic" (string).
Example: [{"id":"hackernews:123","score":8,"reason":"Major new open-source LLM release","topic":"Llama 4 Release"}]

Return ONLY the JSON array, no other text.`

// LLMEvaluator uses an LLM to batch-evaluate items for AI relevance and importance.
type LLMEvaluator struct {
	client   *http.Client
	provider string // "openai" or "anthropic"
	model    string
	apiKey   string
	baseURL  string
	minScore float64
}

// LLMResult is the per-item evaluation from the LLM.
type LLMResult struct {
	ID     string `json:"id"`
	Score  int    `json:"score"`
	Reason string `json:"reason"`
	Topic  string `json:"topic"`
}

// NewLLMEvaluator creates a new LLM evaluator.
func NewLLMEvaluator(provider, model, apiKey, baseURL string, minScore float64) *LLMEvaluator {
	if model == "" {
		switch provider {
		case "anthropic":
			model = "claude-sonnet-4-20250514"
		default:
			model = "gpt-4o-mini"
		}
	}
	if minScore <= 0 {
		minScore = 6
	}
	return &LLMEvaluator{
		client:   &http.Client{Timeout: 60 * time.Second},
		provider: provider,
		model:    model,
		apiKey:   apiKey,
		baseURL:  baseURL,
		minScore: minScore,
	}
}

// EvaluateItems sends all items in one batch to the LLM and returns scored results.
// Items scoring below minScore are filtered out.
func (e *LLMEvaluator) EvaluateItems(ctx context.Context, items []source.Item) ([]LLMResult, error) {
	if len(items) == 0 {
		return nil, nil
	}

	// Build item list for the prompt.
	var lines []string
	for _, item := range items {
		line := fmt.Sprintf("- ID: %s | Source: %s | Score: %d | Title: %s",
			item.ID, item.Source, item.Score, item.Title)
		if item.Description != "" {
			desc := item.Description
			if len(desc) > 200 {
				desc = desc[:200] + "..."
			}
			line += " | Desc: " + desc
		}
		if item.URL != "" {
			line += " | URL: " + item.URL
		}
		lines = append(lines, line)
	}

	prompt := fmt.Sprintf(batchPrompt, strings.Join(lines, "\n"))

	var raw string
	var err error

	switch e.provider {
	case "anthropic":
		raw, err = e.callAnthropic(ctx, prompt)
	default:
		raw, err = e.callOpenAI(ctx, prompt)
	}
	if err != nil {
		return nil, err
	}

	// Parse JSON response.
	raw = strings.TrimSpace(raw)
	// Handle markdown code block wrapping.
	if strings.HasPrefix(raw, "```") {
		if idx := strings.Index(raw[3:], "\n"); idx >= 0 {
			raw = raw[3+idx+1:]
		}
		if strings.HasSuffix(raw, "```") {
			raw = raw[:len(raw)-3]
		}
		raw = strings.TrimSpace(raw)
	}

	var results []LLMResult
	if err := json.Unmarshal([]byte(raw), &results); err != nil {
		return nil, fmt.Errorf("parse llm response: %w\nraw: %s", err, truncateStr(raw, 500))
	}

	// Filter by min score.
	var filtered []LLMResult
	for _, r := range results {
		if float64(r.Score) >= e.minScore {
			filtered = append(filtered, r)
		}
	}

	return filtered, nil
}

func (e *LLMEvaluator) callOpenAI(ctx context.Context, prompt string) (string, error) {
	baseURL := e.baseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}

	payload := map[string]any{
		"model": e.model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"temperature": 0.1,
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create openai request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.apiKey)

	resp, err := e.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("call openai: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp map[string]any
		json.NewDecoder(resp.Body).Decode(&errResp)
		return "", fmt.Errorf("openai status %d: %v", resp.StatusCode, errResp)
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode openai response: %w", err)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("openai: no choices returned")
	}
	return result.Choices[0].Message.Content, nil
}

func (e *LLMEvaluator) callAnthropic(ctx context.Context, prompt string) (string, error) {
	baseURL := e.baseURL
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}

	payload := map[string]any{
		"model":      e.model,
		"max_tokens": 4096,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create anthropic request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", e.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := e.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("call anthropic: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp map[string]any
		json.NewDecoder(resp.Body).Decode(&errResp)
		return "", fmt.Errorf("anthropic status %d: %v", resp.StatusCode, errResp)
	}

	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode anthropic response: %w", err)
	}

	if len(result.Content) == 0 {
		return "", fmt.Errorf("anthropic: no content returned")
	}
	return result.Content[0].Text, nil
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
