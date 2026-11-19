package ratelimit

import (
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/identity"
	"github.com/google/uuid"
)

func TestNoopLimiterAlwaysAllows(t *testing.T) {
	l := New(config.Auth{Mode: config.AuthModeNone})
	allowed, retryAfter := l.Allow(identity.CredentialID{})
	if !allowed || retryAfter != 0 {
		t.Fatalf("allowed=%v retry_after=%v", allowed, retryAfter)
	}
}

func TestTokenBucketLimiterRefillsPerKey(t *testing.T) {
	clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	l := &tokenBucketLimiter{
		ratePerSecond: 1,
		burst:         2,
		entries:       make(map[identity.CredentialID]bucketState),
		now: func() time.Time {
			return clock
		},
	}
	a, b := identity.StaticClientID("client-a"), identity.StaticClientID("client-b")
	if allowed, _ := l.Allow(a); !allowed {
		t.Fatal("first request should pass")
	}
	if allowed, _ := l.Allow(a); !allowed {
		t.Fatal("second request should pass")
	}
	if allowed, retryAfter := l.Allow(a); allowed || retryAfter < time.Second {
		t.Fatalf("third request should be limited, retry_after=%v", retryAfter)
	}
	if allowed, _ := l.Allow(b); !allowed {
		t.Fatal("different key should have its own bucket")
	}
	clock = clock.Add(1 * time.Second)
	if allowed, _ := l.Allow(a); !allowed {
		t.Fatal("bucket should refill after time passes")
	}
}

func TestBucketSeparatesCredentialSourcesAndAnonymous(t *testing.T) {
	l := New(config.Auth{RateLimit: &config.RateLimit{RequestsPerMinute: 1, Burst: 1}})
	l.(*tokenBucketLimiter).now = func() time.Time { return time.Unix(1, 0) }
	key := uuid.New()
	for _, id := range []identity.CredentialID{
		{},
		identity.StaticClientID("anonymous"),
		identity.StaticClientID(key.String()),
		identity.DatabaseKeyID(key),
		identity.DatabaseKeyID(uuid.New()),
	} {
		if allowed, _ := l.Allow(id); !allowed {
			t.Errorf("credential %+v did not have an independent bucket", id)
		}
		if allowed, retryAfter := l.Allow(id); allowed || retryAfter != time.Minute {
			t.Errorf("credential %+v did not exhaust its bucket: allowed=%v retry_after=%v", id, allowed, retryAfter)
		}
	}
}

func TestConfigEqualComparesRateLimitOnly(t *testing.T) {
	a := config.Auth{Mode: config.AuthModeNone, RateLimit: &config.RateLimit{RequestsPerMinute: 60, Burst: 1}}
	b := config.Auth{Mode: config.AuthModeBearerStatic, RateLimit: &config.RateLimit{RequestsPerMinute: 60, Burst: 1}}
	if !ConfigEqual(a, b) {
		t.Fatal("same rate limit settings should match")
	}
	b.RateLimit.Burst = 2
	if ConfigEqual(a, b) {
		t.Fatal("different rate limit settings should not match")
	}
	if ConfigEqual(config.Auth{}, b) {
		t.Fatal("nil and non-nil rate limit settings should not match")
	}
}
