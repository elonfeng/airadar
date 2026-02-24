package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/elonfeng/airadar/pkg/source"
	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

// Snapshot records a point-in-time score for an item.
type Snapshot struct {
	ID        int64     `db:"id"`
	ItemID    string    `db:"item_id"`
	Score     int       `db:"score"`
	Comments  int       `db:"comments"`
	CheckedAt time.Time `db:"checked_at"`
}

// Trend represents a detected trending topic.
type Trend struct {
	ID          int64     `db:"id" json:"id"`
	Topic       string    `db:"topic" json:"topic"`
	Score       float64   `db:"score" json:"score"`
	SourceCount int       `db:"source_count" json:"source_count"`
	ItemIDsJSON string    `db:"item_ids" json:"-"`
	ItemIDs     []string  `json:"item_ids" db:"-"`
	FirstSeen   time.Time `db:"first_seen" json:"first_seen"`
	LastUpdated time.Time `db:"last_updated" json:"last_updated"`
	Alerted     bool      `db:"alerted" json:"alerted"`
}

// ListOpts controls item listing.
type ListOpts struct {
	Source source.SourceType
	Since  time.Time
	Limit  int
}

// TrendListOpts controls trend listing.
type TrendListOpts struct {
	MinScore  float64
	Limit     int
	Unalerted bool
}

// Store is the persistence interface.
type Store interface {
	UpsertItem(ctx context.Context, item *source.Item) error
	UpsertItems(ctx context.Context, items []source.Item) error
	GetItem(ctx context.Context, id string) (*source.Item, error)
	ListItems(ctx context.Context, opts ListOpts) ([]source.Item, error)
	CountItemsBySource(ctx context.Context) (map[source.SourceType]int, error)

	AddSnapshot(ctx context.Context, itemID string, score, comments int) error
	GetSnapshots(ctx context.Context, itemID string, since time.Time) ([]Snapshot, error)

	ClearTrends(ctx context.Context) error
	UpsertTrend(ctx context.Context, t *Trend) error
	ListTrends(ctx context.Context, opts TrendListOpts) ([]Trend, error)
	MarkAlerted(ctx context.Context, trendID int64) error

	Close() error
}

// SQLiteStore implements Store using SQLite.
type SQLiteStore struct {
	db *sqlx.DB
}

// New opens a SQLite database and runs migrations.
func New(path string) (*SQLiteStore, error) {
	db, err := sqlx.Open("sqlite", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", path, err)
	}

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return &SQLiteStore{db: db}, nil
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

func (s *SQLiteStore) UpsertItem(ctx context.Context, item *source.Item) error {
	tagsJSON, _ := json.Marshal(item.Tags)
	extraJSON, _ := json.Marshal(item.Extra)

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO items (id, source, external_id, title, url, description, author, score, comments, tags, published_at, collected_at, extra)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			score = excluded.score,
			comments = excluded.comments,
			collected_at = excluded.collected_at,
			tags = excluded.tags,
			extra = excluded.extra
	`, item.ID, item.Source, item.ExternalID, item.Title, item.URL,
		item.Description, item.Author, item.Score, item.Comments,
		string(tagsJSON), item.PublishedAt, item.CollectedAt, string(extraJSON))
	if err != nil {
		return fmt.Errorf("upsert item %s: %w", item.ID, err)
	}
	return nil
}

func (s *SQLiteStore) UpsertItems(ctx context.Context, items []source.Item) error {
	for i := range items {
		if err := s.UpsertItem(ctx, &items[i]); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLiteStore) GetItem(ctx context.Context, id string) (*source.Item, error) {
	var item source.Item
	err := s.db.GetContext(ctx, &item, "SELECT * FROM items WHERE id = ?", id)
	if err != nil {
		return nil, fmt.Errorf("get item %s: %w", id, err)
	}
	json.Unmarshal([]byte(item.TagsJSON), &item.Tags)
	json.Unmarshal([]byte(item.ExtraJSON), &item.Extra)
	return &item, nil
}

func (s *SQLiteStore) ListItems(ctx context.Context, opts ListOpts) ([]source.Item, error) {
	query := "SELECT * FROM items WHERE 1=1"
	var args []any

	if opts.Source != "" {
		query += " AND source = ?"
		args = append(args, opts.Source)
	}
	if !opts.Since.IsZero() {
		query += " AND collected_at >= ?"
		args = append(args, opts.Since)
	}

	query += " ORDER BY collected_at DESC"

	limit := opts.Limit
	if limit <= 0 {
		limit = 100
	}
	query += " LIMIT ?"
	args = append(args, limit)

	var items []source.Item
	if err := s.db.SelectContext(ctx, &items, query, args...); err != nil {
		return nil, fmt.Errorf("list items: %w", err)
	}

	for i := range items {
		json.Unmarshal([]byte(items[i].TagsJSON), &items[i].Tags)
		json.Unmarshal([]byte(items[i].ExtraJSON), &items[i].Extra)
	}
	return items, nil
}

func (s *SQLiteStore) CountItemsBySource(ctx context.Context) (map[source.SourceType]int, error) {
	rows, err := s.db.QueryxContext(ctx, "SELECT source, COUNT(*) as cnt FROM items GROUP BY source")
	if err != nil {
		return nil, fmt.Errorf("count items by source: %w", err)
	}
	defer rows.Close()

	counts := make(map[source.SourceType]int)
	for rows.Next() {
		var src string
		var cnt int
		if err := rows.Scan(&src, &cnt); err != nil {
			return nil, err
		}
		counts[source.SourceType(src)] = cnt
	}
	return counts, nil
}

func (s *SQLiteStore) AddSnapshot(ctx context.Context, itemID string, score, comments int) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO score_snapshots (item_id, score, comments, checked_at)
		VALUES (?, ?, ?, ?)
	`, itemID, score, comments, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("add snapshot %s: %w", itemID, err)
	}
	return nil
}

func (s *SQLiteStore) ClearTrends(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM trends")
	return err
}

func (s *SQLiteStore) GetSnapshots(ctx context.Context, itemID string, since time.Time) ([]Snapshot, error) {
	var snaps []Snapshot
	err := s.db.SelectContext(ctx, &snaps,
		"SELECT * FROM score_snapshots WHERE item_id = ? AND checked_at >= ? ORDER BY checked_at",
		itemID, since)
	if err != nil {
		return nil, fmt.Errorf("get snapshots %s: %w", itemID, err)
	}
	return snaps, nil
}

func (s *SQLiteStore) UpsertTrend(ctx context.Context, t *Trend) error {
	itemIDsJSON, _ := json.Marshal(t.ItemIDs)
	if t.ID > 0 {
		_, err := s.db.ExecContext(ctx, `
			UPDATE trends SET topic = ?, score = ?, source_count = ?, item_ids = ?, last_updated = ?, alerted = ?
			WHERE id = ?
		`, t.Topic, t.Score, t.SourceCount, string(itemIDsJSON), t.LastUpdated, t.Alerted, t.ID)
		if err != nil {
			return fmt.Errorf("update trend %d: %w", t.ID, err)
		}
		return nil
	}

	res, err := s.db.ExecContext(ctx, `
		INSERT INTO trends (topic, score, source_count, item_ids, first_seen, last_updated, alerted)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, t.Topic, t.Score, t.SourceCount, string(itemIDsJSON), t.FirstSeen, t.LastUpdated, t.Alerted)
	if err != nil {
		return fmt.Errorf("insert trend: %w", err)
	}
	t.ID, _ = res.LastInsertId()
	return nil
}

func (s *SQLiteStore) ListTrends(ctx context.Context, opts TrendListOpts) ([]Trend, error) {
	query := "SELECT * FROM trends WHERE 1=1"
	var args []any

	if opts.MinScore > 0 {
		query += " AND score >= ?"
		args = append(args, opts.MinScore)
	}
	if opts.Unalerted {
		query += " AND alerted = 0"
	}

	query += " ORDER BY score DESC"

	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}
	query += " LIMIT ?"
	args = append(args, limit)

	var trends []Trend
	if err := s.db.SelectContext(ctx, &trends, query, args...); err != nil {
		return nil, fmt.Errorf("list trends: %w", err)
	}

	for i := range trends {
		json.Unmarshal([]byte(trends[i].ItemIDsJSON), &trends[i].ItemIDs)
	}
	return trends, nil
}

func (s *SQLiteStore) MarkAlerted(ctx context.Context, trendID int64) error {
	_, err := s.db.ExecContext(ctx, "UPDATE trends SET alerted = 1 WHERE id = ?", trendID)
	if err != nil {
		return fmt.Errorf("mark alerted %d: %w", trendID, err)
	}
	return nil
}
