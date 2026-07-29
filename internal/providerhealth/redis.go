package providerhealth

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

type redisBackend struct {
	client    *redis.Client
	keyPrefix string
	ctx       context.Context
}

func newRedisBackend(redisURL, keyPrefix string) *redisBackend {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		opts = &redis.Options{Addr: redisURL}
	}
	if keyPrefix == "" {
		keyPrefix = "aiproxy:provider-health"
	}
	return &redisBackend{client: redis.NewClient(opts), keyPrefix: keyPrefix, ctx: context.Background()}
}

func (b *redisBackend) key(name string) string {
	return b.keyPrefix + ":" + name
}

func (b *redisBackend) MarkSuccess(name string) error {
	return b.client.Del(b.ctx, b.key(name)).Err()
}

func (b *redisBackend) MarkFailure(name string, cooldown time.Duration) error {
	return b.client.Set(b.ctx, b.key(name), "unhealthy", cooldown).Err()
}

func (b *redisBackend) IsHealthy(name string) (bool, error) {
	count, err := b.client.Exists(b.ctx, b.key(name)).Result()
	if err != nil {
		return true, err
	}
	return count == 0, nil
}
