package alert

import (
	"context"
	"errors"
	"fmt"

	"github.com/elonfeng/airadar/pkg/source"
)

// Notification is the data sent to alert destinations.
type Notification struct {
	Title   string         `json:"title"`
	Body    string         `json:"body"`
	URL     string         `json:"url"`
	Score   float64        `json:"score"`
	Sources []string       `json:"sources"`
	Items   []source.Item  `json:"items"`
}

// Notifier delivers alerts to a specific destination.
type Notifier interface {
	Name() string
	Send(ctx context.Context, n *Notification) error
}

// Manager broadcasts notifications to all registered notifiers.
type Manager struct {
	notifiers []Notifier
}

// NewManager creates a new alert manager.
func NewManager(notifiers []Notifier) *Manager {
	return &Manager{notifiers: notifiers}
}

// HasNotifiers returns true if at least one notifier is configured.
func (m *Manager) HasNotifiers() bool {
	return len(m.notifiers) > 0
}

// Broadcast sends a notification to all registered notifiers.
func (m *Manager) Broadcast(ctx context.Context, n *Notification) error {
	var errs []error
	for _, notifier := range m.notifiers {
		if err := notifier.Send(ctx, n); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", notifier.Name(), err))
		}
	}
	return errors.Join(errs...)
}
