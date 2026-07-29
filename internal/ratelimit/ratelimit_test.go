package ratelimit

import (
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/config"
)

func TestNoopLimiterAlwaysAllows(t *testing.T) {
	l := New(config.Auth{Mode: config.AuthModeNone})
	allowed, retryAfter := l.Allow("anonymous")
	if !allowed || retryAfter != 0 {
		t.Fatalf("allowed=%v retry_after=%v", allowed, retryAfter)
	}
}

func TestTokenBucketLimiterRefillsPerKey(t *testing.T) {
	clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	l := &tokenBucketLimiter{
		ratePerSecond: 1,
		burst:         2,
		entries:       make(map[string]bucketState),
		now: func() time.Time {
			return clock
		},
	}
	if allowed, _ := l.Allow("client-a"); !allowed {
		t.Fatal("first request should pass")
	}
	if allowed, _ := l.Allow("client-a"); !allowed {
		t.Fatal("second request should pass")
	}
	if allowed, retryAfter := l.Allow("client-a"); allowed || retryAfter < time.Second {
		t.Fatalf("third request should be limited, retry_after=%v", retryAfter)
	}
	if allowed, _ := l.Allow("client-b"); !allowed {
		t.Fatal("different key should have its own bucket")
	}
	clock = clock.Add(1 * time.Second)
	if allowed, _ := l.Allow("client-a"); !allowed {
		t.Fatal("bucket should refill after time passes")
	}
}
