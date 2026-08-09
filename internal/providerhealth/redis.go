package providerhealth

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const redisOperationTimeout = 2 * time.Second

type redisBackend struct {
	client    *redis.Client
	keyPrefix string
}

func newRedisBackend(redisURL, keyPrefix string) (*redisBackend, error) {
	opts, err := parseRedisOptions(redisURL)
	if err != nil {
		return nil, err
	}
	if keyPrefix == "" {
		keyPrefix = "aiproxy:provider-health"
	}
	return &redisBackend{client: redis.NewClient(opts), keyPrefix: keyPrefix}, nil
}

func parseRedisOptions(redisURL string) (*redis.Options, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse redis_url: %w", err)
	}
	return opts, nil
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

func (b *redisBackend) Snapshot(ctx context.Context, names []string) (map[string]bool, error) {
	ctx, cancel := redisOperationContext(ctx)
	defer cancel()
	pipe := b.client.Pipeline()
	cmds := make(map[string]*redis.IntCmd, len(names))
	for _, name := range names {
		cmds[name] = pipe.Exists(ctx, b.key(name))
	}
	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		return nil, err
	}
	out := make(map[string]bool, len(names))
	for _, name := range names {
		count, err := cmds[name].Result()
		if err != nil {
			return nil, err
		}
		out[name] = count == 0
	}
	return out, nil
}

func (b *redisBackend) Close() error {
	return b.client.Close()
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
