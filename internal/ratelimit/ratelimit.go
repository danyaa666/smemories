// Package ratelimit is a small in-memory sliding-window limiter.
//
// ponytail: state is per process. With more than one API task the effective limit is
// limit x tasks; move the counters to a shared store (Redis, DynamoDB) when that matters.
package ratelimit

import (
	"sync"
	"time"
)

// Limiter allows at most limit hits per key in any window.
type Limiter struct {
	limit  int
	window time.Duration
	now    func() time.Time // injectable for tests

	mu        sync.Mutex
	hits      map[string][]time.Time // ascending; at most limit entries per key
	lastSweep time.Time
}

// New returns a Limiter for limit hits per window. now may be nil (time.Now).
func New(limit int, window time.Duration, now func() time.Time) *Limiter {
	if now == nil {
		now = time.Now
	}
	return &Limiter{limit: limit, window: window, now: now, hits: map[string][]time.Time{}}
}

// Take records a hit for key unless the key is already at its limit. When it is, it
// returns false and how long until the oldest hit leaves the window (the Retry-After).
// Recording up front (and Refund-ing on success) keeps a burst of parallel requests
// from all passing the check before any of them is counted.
func (l *Limiter) Take(key string) (ok bool, retryAfter time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.sweep(now)
	hs := l.fresh(key, now)
	if len(hs) >= l.limit {
		return false, hs[0].Add(l.window).Sub(now)
	}
	l.hits[key] = append(hs, now)
	return true, 0
}

// Refund removes the most recent hit for key, undoing a Take (for example after a
// successful login: only failures should count).
func (l *Limiter) Refund(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	hs := l.fresh(key, l.now())
	if len(hs) <= 1 {
		delete(l.hits, key)
		return
	}
	l.hits[key] = hs[:len(hs)-1]
}

// fresh drops the expired hits of key. Callers hold l.mu.
func (l *Limiter) fresh(key string, now time.Time) []time.Time {
	hs := l.hits[key]
	i := 0
	for i < len(hs) && !hs[i].After(now.Add(-l.window)) {
		i++
	}
	return hs[i:]
}

// sweep deletes keys whose hits have all expired, at most once per window, so the map
// does not grow forever. Callers hold l.mu.
func (l *Limiter) sweep(now time.Time) {
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
