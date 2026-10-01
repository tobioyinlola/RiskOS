package store

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/tobioyinlola/RiskOS/backend/internal/platform/clock"
	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/store/sqlcgen"
)

// epochKey holds the feature store's epoch in Redis. It is written once and never expires, so it
// disappears only when Redis loses its data.
const epochKey = "rk:epoch"

// stateCheckTimeout bounds one check; checks run in the background, never on the decision path.
const stateCheckTimeout = 2 * time.Second

// StateWatcher detects a loss of the feature store's state (DISASTER_RECOVERY.md) by comparing
// an epoch kept in Redis with the one recorded in PostgreSQL, and the hash key's fingerprint
// with the recorded one. After a reset, history signals warm up for the configured period.
type StateWatcher struct {
	rdb         Redis
	q           *sqlcgen.Queries
	fingerprint string
	warmup      time.Duration
	clock       clock.Clock
	logger      *slog.Logger
	// warmUntil is the end of the current warm-up (unix nanoseconds); zero when there is none.
	warmUntil atomic.Int64
}

// StateDB runs the watcher's statements.
type StateDB = sqlcgen.DBTX

// NewStateWatcher returns a watcher. db must allow platform-wide statements (the application
// pool does: the state table has no tenant).
func NewStateWatcher(rdb Redis, db sqlcgen.DBTX, hashKey []byte, warmup time.Duration, clk clock.Clock, logger *slog.Logger) *StateWatcher {
	return &StateWatcher{
		rdb: rdb, q: sqlcgen.New(db), fingerprint: keyFingerprint(hashKey),
		warmup: warmup, clock: clk, logger: logger,
	}
}

// keyFingerprint identifies the hash key without revealing it.
func keyFingerprint(key []byte) string {
	m := hmac.New(sha256.New, key)
	m.Write([]byte("riskos feature-store key fingerprint"))
	return hex.EncodeToString(m.Sum(nil)[:8])
}

// Warming reports whether history signals are warming up after a reset. Safe on a nil watcher.
func (w *StateWatcher) Warming(now time.Time) bool {
	return w != nil && now.UnixNano() < w.warmUntil.Load()
}

// Check compares the epochs once and updates the warm-up. On failure the previous answer stays.
func (w *StateWatcher) Check(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, stateCheckTimeout)
	defer cancel()
	epoch, err := w.rdb.Get(ctx, epochKey).Result()
	if errors.Is(err, redis.Nil) {
		// No marker: a new Redis, or one that lost its data. Every instance converges on the
		// first marker written.
		if err := w.rdb.SetNX(ctx, epochKey, uuid.NewString(), 0).Err(); err != nil {
			return fmt.Errorf("write feature-store epoch: %w", err)
		}
		epoch, err = w.rdb.Get(ctx, epochKey).Result()
	}
	if err != nil {
		return fmt.Errorf("read feature-store epoch: %w", err)
	}
	if err := w.q.SyncFeatureStoreState(ctx, sqlcgen.SyncFeatureStoreStateParams{
		Epoch: epoch, KeyFingerprint: w.fingerprint, Now: w.clock.Now(),
	}); err != nil {
		return fmt.Errorf("record feature-store epoch: %w", err)
	}
	resetAt, err := w.q.GetFeatureStoreReset(ctx)
	if err != nil {
		return fmt.Errorf("read feature-store reset: %w", err)
	}
	var until int64
	if resetAt != nil {
		until = resetAt.Add(w.warmup).UnixNano()
	}
	if prev := w.warmUntil.Swap(until); until > prev && w.Warming(w.clock.Now()) {
		w.logger.ErrorContext(ctx, "feature store state was reset; history signals are warming up",
			slog.Time("reset_at", *resetAt), slog.Time("warm_until", time.Unix(0, until).UTC()))
	}
	return nil
}

// Run checks at once, then every interval until ctx ends.
func (w *StateWatcher) Run(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		if err := w.Check(ctx); err != nil && ctx.Err() == nil {
			w.logger.WarnContext(ctx, "feature-store state check failed", slog.Any("error", err))
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
