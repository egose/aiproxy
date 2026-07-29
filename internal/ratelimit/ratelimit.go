package ratelimit

import (
	"sync"
	"time"

	"github.com/egose/aiproxy/internal/config"
)

type Limiter interface {
	Allow(key string) (bool, time.Duration)
}

type noopLimiter struct{}

func New(cfg config.Auth) Limiter {
	if cfg.RateLimit == nil {
		return noopLimiter{}
	}
	return &tokenBucketLimiter{
		ratePerSecond: float64(cfg.RateLimit.RequestsPerMinute) / 60,
		burst:         float64(cfg.RateLimit.Burst),
		entries:       make(map[string]bucketState),
		now:           time.Now,
	}
}

func (noopLimiter) Allow(string) (bool, time.Duration) {
	return true, 0
}

type tokenBucketLimiter struct {
	mu            sync.Mutex
	ratePerSecond float64
	burst         float64
	entries       map[string]bucketState
	now           func() time.Time
}

type bucketState struct {
	tokens float64
	last   time.Time
}

func (l *tokenBucketLimiter) Allow(key string) (bool, time.Duration) {
	if key == "" {
		key = "anonymous"
	}
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	state, ok := l.entries[key]
	if !ok {
		state = bucketState{tokens: l.burst, last: now}
	} else {
		elapsed := now.Sub(state.last).Seconds()
		state.tokens += elapsed * l.ratePerSecond
		if state.tokens > l.burst {
			state.tokens = l.burst
		}
		state.last = now
	}
	if state.tokens >= 1 {
		state.tokens--
		l.entries[key] = state
		return true, 0
	}
	deficit := 1 - state.tokens
	retryAfter := time.Duration(deficit / l.ratePerSecond * float64(time.Second))
	if retryAfter < time.Second {
		retryAfter = time.Second
	}
	l.entries[key] = state
	return false, retryAfter
}
