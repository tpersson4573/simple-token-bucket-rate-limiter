// Package simpletokenbucketratelimiter provides a token bucket rate limiter.
//
// A token bucket has a capacity (burst) and a refill rate (tokens per second).
// Each allowed operation consumes one token. Tokens are replenished continuously
// based on elapsed time. This lets a caller allow a steady rate while still
// permitting short bursts up to the bucket capacity.
package simpletokenbucketratelimiter

import (
	"errors"
	"sync"
	"time"
)

// ErrInvalidRate is returned by New when the refill rate is not positive.
var ErrInvalidRate = errors.New("rate must be > 0")

// ErrInvalidBurst is returned by New when the burst capacity is not positive.
var ErrInvalidBurst = errors.New("burst must be > 0")

// Clock returns the current time. Tests inject a fake clock so that behavior
// depending on elapsed time is deterministic. The default clock is time.Now.
type Clock func() time.Time

// Limiter is a token bucket rate limiter. It is safe for concurrent use.
type Limiter struct {
	mu         sync.Mutex
	rate       float64
	burst      float64
	tokens     float64
	lastRefill time.Time
	now        Clock
}

// New returns a Limiter that refills at rate tokens per second and can hold at
// most burst tokens. It returns an error if rate or burst is not positive.
//
// The optional clock argument is used to make tests deterministic. When nil,
// time.Now is used. At most one clock is used; passing more than one is ignored
// beyond the first to keep the constructor signature simple.
func New(rate float64, burst int, clock ...Clock) (*Limiter, error) {
	if rate <= 0 {
		return nil, ErrInvalidRate
	}
	if burst <= 0 {
		return nil, ErrInvalidBurst
	}
	c := time.Now
	if len(clock) > 0 && clock[0] != nil {
		c = clock[0]
	}
	return &Limiter{
		rate:       rate,
		burst:      float64(burst),
		tokens:     float64(burst),
		lastRefill: c(),
		now:        c,
	}, nil
}

// refill adds tokens for the time elapsed since the last refill. It must be
// called with l.mu held.
func (l *Limiter) refill(now time.Time) {
	elapsed := now.Sub(l.lastRefill).Seconds()
	if elapsed <= 0 {
		return
	}
	l.tokens += elapsed * l.rate
	if l.tokens > l.burst {
		l.tokens = l.burst
	}
	l.lastRefill = now
}

// Allow reports whether one token was available. If so, it consumes the token
// and returns true. Otherwise it returns false and leaves the bucket unchanged.
//
// The refill is calculated from the injected clock, so a fake clock advances
// the bucket deterministically in tests.
func (l *Limiter) Allow() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.refill(now)
	if l.tokens >= 1 {
		l.tokens--
		return true
	}
	return false
}

// Tokens returns the number of whole tokens currently available. The value is
// floored so that a partially refilled token is not reported as spendable.
func (l *Limiter) Tokens() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.refill(now)
	return int(l.tokens)
}
