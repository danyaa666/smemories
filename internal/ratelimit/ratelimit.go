// Package ratelimit holds the sliding-window rate limiters. The production implementation lives in
// Redis (redis.go) so every API task and every restart sees the same counts (D-23); Memory is the
// in-process implementation for unit tests only and is never wired in cmd/smemories-api.
package ratelimit

import (
	"context"
	"sync"
	"time"
)

// Limiter allows at most limit hits per key in any window.
type Limiter interface {
	// Take records a hit for key unless the key is already at its limit. When it is, it returns
	// false and how long until the oldest hit leaves the window (the Retry-After). Recording up
	// front (and Refund-ing on success) keeps a burst of parallel requests from all passing the
	// check before any of them is counted. err is set only when the limiter itself failed.
	Take(ctx context.Context, key string) (ok bool, retryAfter time.Duration, err error)
	// Refund removes the most recent hit for key, undoing a Take (for example after a successful
	// login: only failures should count).
	Refund(ctx context.Context, key string) error
}

// Memory is a per-process Limiter for unit tests. Do not use it in the API: its counts are not
// shared between tasks and a restart resets them.
type Memory struct {
	limit  int
	window time.Duration
	now    func() time.Time // injectable for tests

	mu        sync.Mutex
	hits      map[string][]time.Time // ascending; at most limit entries per key
	lastSweep time.Time
}

// NewMemory returns a Memory limiter for limit hits per window. now may be nil (time.Now).
func NewMemory(limit int, window time.Duration, now func() time.Time) *Memory {
	if now == nil {
		now = time.Now
	}
	return &Memory{limit: limit, window: window, now: now, hits: map[string][]time.Time{}}
}

// Take implements Limiter.
func (l *Memory) Take(_ context.Context, key string) (bool, time.Duration, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.sweep(now)
	hs := l.fresh(key, now)
	if len(hs) >= l.limit {
		return false, hs[0].Add(l.window).Sub(now), nil
	}
	l.hits[key] = append(hs, now)
	return true, 0, nil
}

// Refund implements Limiter.
func (l *Memory) Refund(_ context.Context, key string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	hs := l.fresh(key, l.now())
	if len(hs) <= 1 {
		delete(l.hits, key)
		return nil
	}
	l.hits[key] = hs[:len(hs)-1]
	return nil
}

// fresh drops the expired hits of key. Callers hold l.mu.
func (l *Memory) fresh(key string, now time.Time) []time.Time {
	hs := l.hits[key]
	i := 0
	for i < len(hs) && !hs[i].After(now.Add(-l.window)) {
		i++
	}
	return hs[i:]
}

// sweep deletes keys whose hits have all expired, at most once per window, so the map
// does not grow forever. Callers hold l.mu.
func (l *Memory) sweep(now time.Time) {
	if now.Sub(l.lastSweep) < l.window {
		return
	}
	l.lastSweep = now
	for k := range l.hits {
		if len(l.fresh(k, now)) == 0 {
			delete(l.hits, k)
		}
	}
}
