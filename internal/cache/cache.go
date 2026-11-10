// Package cache is the Cache port: a disposable local store for upstream
// responses and derived data. Deleting it never loses data.
package cache

import (
	"context"
	"time"

	"github.com/Coombszy/scrutiny/internal/domain"
)

// Entry is a cached value with the validator it was served with.
type Entry struct {
	Value     []byte
	ETag      string
	FetchedAt time.Time
}

// Visit records a PR opened in Scrutiny, for the Dashboard.
type Visit struct {
	PR        domain.PRRef
	Title     string
	VisitedAt time.Time
}

// Cache stores entries by key, and the PRs recently opened.
type Cache interface {
	// Get returns the entry for key; ok is false when there is none.
	Get(ctx context.Context, key string) (e Entry, ok bool, err error)
	Put(ctx context.Context, key string, e Entry) error

	// RecordVisit notes that pr was opened, replacing any earlier visit.
	RecordVisit(ctx context.Context, v Visit) error
	// RecentVisits returns up to limit visits, most recent first.
	RecentVisits(ctx context.Context, limit int) ([]Visit, error)
}
