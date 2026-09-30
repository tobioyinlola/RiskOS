// Package redisx owns the Redis client used for velocity counters and short-lived state.
package redisx

import (
	"context"
	"errors"
	"fmt"

	"github.com/redis/go-redis/v9"

	"github.com/tobioyinlola/RiskOS/backend/internal/platform/config"
)

// New creates a Redis client with explicit timeouts and verifies connectivity.
func New(ctx context.Context, cfg config.Redis) (*redis.Client, error) {
	c := redis.NewClient(&redis.Options{
		Addr:                  cfg.Addr,
		Password:              cfg.Password,
		DB:                    cfg.DB,
		PoolSize:              cfg.PoolSize,
		DialTimeout:           cfg.DialTimeout,
		ReadTimeout:           cfg.ReadTimeout,
		WriteTimeout:          cfg.WriteTimeout,
		ContextTimeoutEnabled: true,
		ClientName:            "riskos",
	})
	pingCtx, cancel := context.WithTimeout(ctx, cfg.DialTimeout)
	defer cancel()
	if err := Ping(pingCtx, c); err != nil {
		return nil, fmt.Errorf("connect to redis: %w", errors.Join(err, c.Close()))
	}
	return c, nil
}

// Ping checks that Redis answers a PING.
func Ping(ctx context.Context, c *redis.Client) error {
	if err := c.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis ping: %w", err)
	}
	return nil
}
