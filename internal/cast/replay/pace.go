package replay

import (
	"context"
	"time"
)

// tokenBucket is a per-connection rate limiter. Tokens count bytes; the
// bucket refills at rate bytes/sec, capped at burst. A zero rate disables
// pacing entirely.
type tokenBucket struct {
	rate     int64
	capacity int64
	tokens   float64
	last     time.Time
}

func newTokenBucket(rate, burst int64) *tokenBucket {
	return &tokenBucket{
		rate:     rate,
		capacity: burst,
		tokens:   float64(burst),
		last:     time.Now(),
	}
}

func (b *tokenBucket) wait(ctx context.Context, need int64) error {
	if b.rate <= 0 {
		return nil
	}
	for {
		b.refill()
		if float64(need) <= b.tokens {
			b.tokens -= float64(need)
			return nil
		}
		missing := float64(need) - b.tokens
		sleep := time.Duration(missing / float64(b.rate) * float64(time.Second))
		if sleep <= 0 {
			sleep = time.Millisecond
		}
		t := time.NewTimer(sleep)
		select {
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		case <-t.C:
		}
	}
}

func (b *tokenBucket) refill() {
	now := time.Now()
	elapsed := now.Sub(b.last).Seconds()
	b.last = now
	b.tokens = min(float64(b.capacity), b.tokens+elapsed*float64(b.rate))
}
