package alert

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Slack sends notifications via Slack incoming webhook.
type Slack struct {
	client     *http.Client
	webhookURL string
}

// NewSlack creates a new Slack notifier.
func NewSlack(webhookURL string) *Slack {
	return &Slack{
		client:     &http.Client{Timeout: 10 * time.Second},
		webhookURL: webhookURL,
	}
}

func (s *Slack) Name() string { return "slack" }

func (s *Slack) Send(ctx context.Context, n *Notification) error {
	// Build Slack Block Kit message.
	blocks := []map[string]any{
		{
			"type": "header",
			"text": map[string]any{
				"type": "plain_text",
				"text": fmt.Sprintf("🔥 %s", n.Title),
			},
		},
		{
			"type": "section",
			"text": map[string]any{
				"type": "mrkdwn",
				"text": fmt.Sprintf("*Score:* %.1f | *Sources:* %d\n%s", n.Score, len(n.Sources), n.Body),
			},
		},
	}

	// Add top items as context.
	if len(n.Items) > 0 {
		limit := 5
		if len(n.Items) < limit {
			limit = len(n.Items)
		}
		var elements []map[string]any
		for _, item := range n.Items[:limit] {
			elements = append(elements, map[string]any{
				"type": "mrkdwn",
				"text": fmt.Sprintf("<%s|%s> [%s]", item.URL, item.Title, item.Source),
			})
		}
		blocks = append(blocks, map[string]any{
			"type":     "context",
			"elements": elements,
		})
	}

	payload := map[string]any{"blocks": blocks}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal slack payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create slack request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("send slack webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("slack webhook status %d", resp.StatusCode)
	}

	return nil
}
