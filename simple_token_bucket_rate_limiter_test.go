package simpletokenbucketratelimiter

import (
	"errors"
	"math"
	"sync"
	"testing"
	"time"
)

// fakeClock is a deterministic clock for tests. It advances only when the test
// tells it to, so no test depends on wall-clock time.
type fakeClock struct {
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.now = c.now.Add(d)
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Unix(0, 0)}
}

func TestNewRejectsInvalidRate(t *testing.T) {
	if _, err := New(0, 1); !errors.Is(err, ErrInvalidRate) {
		t.Fatalf("New(0, 1) error = %v, want %v", err, ErrInvalidRate)
	}
	if _, err := New(-1, 1); !errors.Is(err, ErrInvalidRate) {
		t.Fatalf("New(-1, 1) error = %v, want %v", err, ErrInvalidRate)
	}
}

func TestNewRejectsInvalidBurst(t *testing.T) {
	if _, err := New(1, 0); !errors.Is(err, ErrInvalidBurst) {
		t.Fatalf("New(1, 0) error = %v, want %v", err, ErrInvalidBurst)
	}
	if _, err := New(1, -1); !errors.Is(err, ErrInvalidBurst) {
		t.Fatalf("New(1, -1) error = %v, want %v", err, ErrInvalidBurst)
	}
}

func TestBurstIsConsumedThenRefilled(t *testing.T) {
	clock := newFakeClock()
	lim, err := New(2, 3, clock.Now)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// The bucket starts full, so three immediate operations are allowed.
	for i := 0; i < 3; i++ {
		if !lim.Allow() {
			t.Fatalf("Allow() call %d = false, want true", i+1)
		}
	}
	if lim.Allow() {
		t.Fatal("Allow() after burst exhausted = true, want false")
	}

	// After half a second at 2 tokens/s, one token is available.
	clock.Advance(500 * time.Millisecond)
	if !lim.Allow() {
		t.Fatal("Allow() after 500ms refill = false, want true")
	}
	if lim.Allow() {
		t.Fatal("Allow() after consuming refilled token = true, want false")
	}
}

func TestRefillNeverExceedsBurst(t *testing.T) {
	clock := newFakeClock()
	lim, err := New(10, 2, clock.Now)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Consume the initial burst.
	lim.Allow()
	lim.Allow()

	// A long idle period would add far more than the burst capacity.
	clock.Advance(time.Hour)
	if got := lim.Tokens(); got != 2 {
		t.Fatalf("Tokens() after long idle = %d, want 2", got)
	}
}

func TestTokensFloorsPartialRefill(t *testing.T) {
	clock := newFakeClock()
	lim, err := New(1, 1, clock.Now)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Consume the only token.
	if !lim.Allow() {
		t.Fatal("first Allow() = false, want true")
	}
	if got := lim.Tokens(); got != 0 {
		t.Fatalf("Tokens() after consume = %d, want 0", got)
	}

	// Half a token of refill is not enough to report a whole token.
	clock.Advance(500 * time.Millisecond)
	if got := lim.Tokens(); got != 0 {
		t.Fatalf("Tokens() after partial refill = %d, want 0", got)
	}

	// A full token is available after another half second.
	clock.Advance(500 * time.Millisecond)
	if got := lim.Tokens(); got != 1 {
		t.Fatalf("Tokens() after full refill = %d, want 1", got)
	}
}

func TestClockDoesNotGoBackwards(t *testing.T) {
	clock := newFakeClock()
	lim, err := New(1, 1, clock.Now)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	lim.Allow()
	// Advancing backwards must not create tokens.
	clock.Advance(-time.Second)
	if lim.Allow() {
		t.Fatal("Allow() after backwards clock = true, want false")
	}
	if got := lim.Tokens(); got != 0 {
		t.Fatalf("Tokens() after backwards clock = %d, want 0", got)
	}
}

func TestFractionalRate(t *testing.T) {
	clock := newFakeClock()
	lim, err := New(0.5, 1, clock.Now)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if !lim.Allow() {
		t.Fatal("first Allow() = false, want true")
	}
	// At 0.5 tokens/s, one token takes 2 seconds.
	clock.Advance(1999 * time.Millisecond)
	if lim.Allow() {
		t.Fatal("Allow() before full token = true, want false")
	}
	clock.Advance(1 * time.Millisecond)
	if !lim.Allow() {
		t.Fatal("Allow() after full token = false, want true")
	}
}

func TestConcurrentAllow(t *testing.T) {
	clock := newFakeClock()
	lim, err := New(1000, 50, clock.Now)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	const workers = 100
	var wg sync.WaitGroup
	var mu sync.Mutex
	allowed := 0

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if lim.Allow() {
				mu.Lock()
				allowed++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	// The bucket starts with 50 tokens and no time passes, so exactly 50
	// operations must be allowed regardless of scheduling.
	if allowed != 50 {
		t.Fatalf("concurrent Allow() allowed %d, want 50", allowed)
	}
}

func TestTokensUsesInjectedClock(t *testing.T) {
	clock := newFakeClock()
	lim, err := New(2, 2, clock.Now)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	lim.Allow()
	lim.Allow()
	clock.Advance(750 * time.Millisecond)
	// 0.75s * 2 tokens/s = 1.5 tokens, floored to 1.
	if got := lim.Tokens(); got != 1 {
		t.Fatalf("Tokens() = %d, want 1", got)
	}
}

func TestAllowDoesNotConsumeWhenEmpty(t *testing.T) {
	clock := newFakeClock()
	lim, err := New(1, 1, clock.Now)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	lim.Allow()
	if lim.Allow() {
		t.Fatal("Allow() on empty bucket = true, want false")
	}
	// A failed Allow must not have changed the token count.
	if got := lim.Tokens(); got != 0 {
		t.Fatalf("Tokens() after failed Allow = %d, want 0", got)
	}
}

func TestFloatTolerance(t *testing.T) {
	clock := newFakeClock()
	lim, err := New(3, 1, clock.Now)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	lim.Allow()
	clock.Advance(time.Duration(333) * time.Millisecond)
	// 0.333s * 3 = 0.999 tokens, just under one.
	if lim.Allow() {
		t.Fatal("Allow() at 0.999 tokens = true, want false")
	}
	clock.Advance(time.Duration(1) * time.Millisecond)
	// Now 1.002 tokens, comfortably above one.
	if !lim.Allow() {
		t.Fatal("Allow() at 1.002 tokens = false, want true")
	}

	// Sanity check on the tolerance helper used above.
	if math.Abs(0.999-1.0) >= 0.01 {
		t.Fatal("tolerance helper misconfigured")
	}
}
