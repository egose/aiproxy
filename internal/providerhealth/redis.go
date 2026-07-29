package providerhealth

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

const redisOperationTimeout = 2 * time.Second

type redisBackend struct {
	client    *redis.Client
	keyPrefix string
}

func newRedisBackend(redisURL, keyPrefix string) *redisBackend {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		opts = &redis.Options{Addr: redisURL}
	}
	if keyPrefix == "" {
		keyPrefix = "aiproxy:provider-health"
	}
	return &redisBackend{client: redis.NewClient(opts), keyPrefix: keyPrefix}
}

func (b *redisBackend) key(name string) string {
	return b.keyPrefix + ":" + name
}

func (b *redisBackend) MarkSuccess(ctx context.Context, name string) error {
	ctx, cancel := redisOperationContext(ctx)
	defer cancel()
	return b.client.Del(ctx, b.key(name)).Err()
}

func (b *redisBackend) MarkFailure(ctx context.Context, name string, cooldown time.Duration) error {
	ctx, cancel := redisOperationContext(ctx)
	defer cancel()
	return b.client.Set(ctx, b.key(name), "unhealthy", cooldown).Err()
}

func (b *redisBackend) IsHealthy(ctx context.Context, name string) (bool, error) {
	ctx, cancel := redisOperationContext(ctx)
	defer cancel()
	count, err := b.client.Exists(ctx, b.key(name)).Result()
	if err != nil {
		return true, err
	}
	return count == 0, nil
}

func redisOperationContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); ok {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, redisOperationTimeout)
}
