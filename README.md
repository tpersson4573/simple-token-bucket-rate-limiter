# simple-token-bucket-rate-limiter

A token bucket that controls how many operations per second are allowed, with burst support.

```go
package main

import (
	"fmt"
	"time"

	limiter "simple-token-bucket-rate-limiter"
)

func main() {
	// Allow 5 operations per second, with a burst of up to 10.
	lim, err := limiter.New(5, 10)
	if err != nil {
		panic(err)
	}

	if lim.Allow() {
		fmt.Println("operation allowed")
	} else {
		fmt.Println("rate limited")
	}

	fmt.Println("tokens available:", lim.Tokens())

	// The bucket refills continuously based on elapsed time.
	time.Sleep(200 * time.Millisecond)
	fmt.Println("tokens after 200ms:", lim.Tokens())
}
```

## Why this exists

A token bucket answers a simple question: can this operation run right now? The
bucket holds a fixed number of tokens, each operation costs one token, and tokens
are replenished at a steady rate up to a maximum. That gives you both a long-term
average rate and a short-term burst allowance, which is what most callers actually
want from a rate limiter.

The trade-off here is that the limiter is time-based and stateful. It computes
refills lazily from the elapsed time since the last refill rather than running a
ticker goroutine, so an idle limiter costs nothing and there is no background
work to shut down. The cost is that the caller must accept a small amount of
floating-point arithmetic when measuring refills, and the limiter must be told
what time it is. By default it uses `time.Now`, but the constructor also accepts
an optional clock so that tests can advance time deterministically.

## Awkward edge

The bucket refills continuously, so `Tokens` reports the floor of the current
token count. A partial token is not spendable and is not reported. This means
`Tokens` can show `0` while a subsequent `Allow` after a few more milliseconds
succeeds; that is expected. If you need to know exactly when the next token will
be available, track the refill rate yourself rather than polling `Tokens`.

## API

- `New(rate float64, burst int, clock ...Clock) (*Limiter, error)` creates a
  limiter. `rate` is tokens per second, `burst` is the maximum number of tokens
  the bucket can hold. The optional clock is used for deterministic tests; when
  omitted, `time.Now` is used. Returns `ErrInvalidRate` or `ErrInvalidBurst` if
  either argument is not positive.
- `(*Limiter) Allow() bool` reports whether one token was available and consumes
  it if so.
- `(*Limiter) Tokens() int` returns the number of whole tokens currently
  available.
- `Clock` is a function type returning `time.Time`.
- `ErrInvalidRate` and `ErrInvalidBurst` are sentinel errors returned by `New`.
