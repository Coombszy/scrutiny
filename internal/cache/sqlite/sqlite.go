// Package sqlite is the Cache adapter backed by a sqlite file, normally
// $XDG_STATE_HOME/scrutiny/cache.db.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"

	"github.com/Coombszy/scrutiny/internal/cache"
	"github.com/Coombszy/scrutiny/internal/domain"
)

const schema = `
CREATE TABLE IF NOT EXISTS entries (
	key        TEXT PRIMARY KEY,
	value      BLOB NOT NULL,
	etag       TEXT NOT NULL,
	fetched_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS visits (
	host       TEXT NOT NULL,
	owner      TEXT NOT NULL,
	repo       TEXT NOT NULL,
	number     INTEGER NOT NULL,
	title      TEXT NOT NULL,
	visited_at INTEGER NOT NULL,
	PRIMARY KEY (host, owner, repo, number)
);
`

// Cache is a sqlite-backed cache.Cache.
type Cache struct {
	db *sql.DB
}

var _ cache.Cache = (*Cache)(nil)

// Open opens (creating if needed) the cache database at path.
func Open(path string) (*Cache, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	return &Cache{db: db}, nil
}

func (c *Cache) Close() error { return c.db.Close() }

func (c *Cache) Get(ctx context.Context, key string) (cache.Entry, bool, error) {
	var (
		e  cache.Entry
		at int64
	)
	err := c.db.QueryRowContext(ctx, `SELECT value, etag, fetched_at FROM entries WHERE key = ?`, key).Scan(&e.Value, &e.ETag, &at)
	if errors.Is(err, sql.ErrNoRows) {
		return cache.Entry{}, false, nil
	}
	if err != nil {
		return cache.Entry{}, false, err
	}
	e.FetchedAt = time.Unix(0, at).UTC()
	return e, true, nil
}

func (c *Cache) Put(ctx context.Context, key string, e cache.Entry) error {
	_, err := c.db.ExecContext(ctx,
		`INSERT INTO entries (key, value, etag, fetched_at) VALUES (?, ?, ?, ?)
		 ON CONFLICT (key) DO UPDATE SET value = excluded.value, etag = excluded.etag, fetched_at = excluded.fetched_at`,
		key, e.Value, e.ETag, e.FetchedAt.UnixNano())
	return err
}

func (c *Cache) Delete(ctx context.Context, key string) error {
	_, err := c.db.ExecContext(ctx, `DELETE FROM entries WHERE key = ?`, key)
	return err
}

// DeletePrefix compares with substr rather than LIKE, so keys holding % or
// _ match literally.
func (c *Cache) DeletePrefix(ctx context.Context, prefix string) error {
	_, err := c.db.ExecContext(ctx, `DELETE FROM entries WHERE substr(key, 1, length(?1)) = ?1`, prefix)
	return err
}

func (c *Cache) RecordVisit(ctx context.Context, v cache.Visit) error {
	_, err := c.db.ExecContext(ctx,
		`INSERT INTO visits (host, owner, repo, number, title, visited_at) VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT (host, owner, repo, number) DO UPDATE SET title = excluded.title, visited_at = excluded.visited_at`,
		v.PR.Host, v.PR.Owner, v.PR.Name, v.PR.Number, v.Title, v.VisitedAt.UnixNano())
	return err
}

func (c *Cache) RecentVisits(ctx context.Context, limit int) ([]cache.Visit, error) {
	rows, err := c.db.QueryContext(ctx,
		`SELECT host, owner, repo, number, title, visited_at FROM visits ORDER BY visited_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var visits []cache.Visit
	for rows.Next() {
		var (
			v  cache.Visit
			at int64
		)
		if err := rows.Scan(&v.PR.Host, &v.PR.Owner, &v.PR.Name, &v.PR.Number, &v.Title, &at); err != nil {
			return nil, err
		}
		v.VisitedAt = time.Unix(0, at).UTC()
		visits = append(visits, v)
	}
	return visits, rows.Err()
}

func (c *Cache) DeleteVisit(ctx context.Context, pr domain.PRRef) error {
	_, err := c.db.ExecContext(ctx,
		`DELETE FROM visits WHERE host = ? AND owner = ? AND repo = ? AND number = ?`,
		pr.Host, pr.Owner, pr.Name, pr.Number)
	return err
}
