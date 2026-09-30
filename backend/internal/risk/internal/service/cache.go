package service

import (
	"context"
	"sync"
	"time"

	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/domain"
	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/engine"
)

// configTTL bounds how long another API instance keeps using settings or rules after they
// change. The instance that made the change invalidates its own copy immediately.
const configTTL = 5 * time.Second

// config is an environment's settings and compiled current rules.
type config struct {
	settings domain.Settings
	rules    []engine.CompiledRule
}

type configEntry struct {
	cfg      config
	loadedAt time.Time
}

// configCache keeps compiled configuration per environment, so the synchronous path does not
// query rules and settings on every event.
type configCache struct {
	ttl     time.Duration
	mu      sync.Mutex
	entries map[string]configEntry
}

func newConfigCache(ttl time.Duration) *configCache {
	return &configCache{ttl: ttl, entries: map[string]configEntry{}}
}

func (c *configCache) get(ctx context.Context, load func(context.Context) (config, error)) (config, error) {
	key, err := scopeKey(ctx)
	if err != nil {
		return config{}, err
	}
	c.mu.Lock()
	e, ok := c.entries[key]
	c.mu.Unlock()
	if ok && time.Since(e.loadedAt) < c.ttl {
		return e.cfg, nil
	}
	cfg, err := load(ctx)
	if err != nil {
		return config{}, err
	}
	c.mu.Lock()
	c.entries[key] = configEntry{cfg: cfg, loadedAt: time.Now()}
	c.mu.Unlock()
	return cfg, nil
}

func (c *configCache) invalidate(ctx context.Context) {
	key, err := scopeKey(ctx)
	if err != nil {
		return
	}
	c.mu.Lock()
	delete(c.entries, key)
	c.mu.Unlock()
}
