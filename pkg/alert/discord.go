package alert

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Discord sends notifications via Discord webhook.
type Discord struct {
	client     *http.Client
	webhookURL string
}

// NewDiscord creates a new Discord notifier.
func NewDiscord(webhookURL string) *Discord {
	return &Discord{
		client:     &http.Client{Timeout: 10 * time.Second},
		webhookURL: webhookURL,
	}
}

func (d *Discord) Name() string { return "discord" }

func (d *Discord) Send(ctx context.Context, n *Notification) error {
	// Build links.
	var links []string
	limit := 5
	if len(n.Items) < limit {
		limit = len(n.Items)
	}
	for _, item := range n.Items[:limit] {
		links = append(links, fmt.Sprintf("• [%s](%s) [%s]", item.Title, item.URL, item.Source))
	}

	embed := map[string]any{
		"title":       fmt.Sprintf("🔥 %s", n.Title),
		"description": fmt.Sprintf("**Score:** %.1f | **Sources:** %d\n\n%s\n\n%s", n.Score, len(n.Sources), n.Body, strings.Join(links, "\n")),
		"color":       0xFF6600,
		"timestamp":   time.Now().UTC().Format(time.RFC3339),
	}

	payload := map[string]any{
		"embeds": []map[string]any{embed},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal discord payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create discord request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("send discord webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("discord webhook status %d", resp.StatusCode)
	}

	return nil
}
