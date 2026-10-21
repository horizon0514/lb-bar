package feed

import (
	"context"
	"sync"
	"time"
)

// Limiter keeps to the OpenAPI's request limits: a token bucket of
// Rate calls a second with Burst at once, and at most Concurrent calls
// in flight (10/s, 5 and 5 for quotes, by the API's rules).
type Limiter struct {
	Rate       float64
	Burst      int
	Concurrent int
	now        func() time.Time

	mu     sync.Mutex
	tokens float64
	last   time.Time
	slots  chan struct{}
}

// NewLimiter is the quote API's limit, a little under it.
func NewLimiter() *Limiter { return &Limiter{Rate: 8, Burst: 4, Concurrent: 4} }

// Wait waits for a token and a slot; done gives the slot back.
func (l *Limiter) Wait(ctx context.Context) (done func(), err error) {
	l.mu.Lock()
	if l.slots == nil {
		l.slots = make(chan struct{}, max(1, l.Concurrent))
		l.tokens = float64(max(1, l.Burst))
		if l.now == nil {
			l.now = time.Now
		}
		l.last = l.now()
	}
	slots := l.slots
	l.mu.Unlock()
	select {
	case slots <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	for {
		d := l.take()
		if d == 0 {
			return func() { <-slots }, nil
		}
		t := time.NewTimer(d)
		select {
		case <-t.C:
		case <-ctx.Done():
			t.Stop()
			<-slots
			return nil, ctx.Err()
		}
	}
}

// take takes a token, or says how long until there is one.
func (l *Limiter) take() time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.tokens = min(float64(max(1, l.Burst)), l.tokens+now.Sub(l.last).Seconds()*l.Rate)
	l.last = now
	if l.tokens >= 1 {
		l.tokens--
		return 0
	}
	return time.Duration((1 - l.tokens) / l.Rate * float64(time.Second))
}
