package redisx

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
)

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time { return c.t }

func TestBreaker(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_000, 0)}
	b := newBreaker(time.Second, slog.New(slog.DiscardHandler))
	b.now = clk.now

	var calls int
	var reply error
	process := b.ProcessHook(func(context.Context, redis.Cmder) error { calls++; return reply })
	run := func() error { return process(context.Background(), redis.NewStatusCmd(context.Background(), "ping")) }

	// Replies from a working server never open it, and a success resets the count.
	reply = redis.Nil
	for range 10 {
		assert.ErrorIs(t, run(), redis.Nil)
	}
	reply = &net.OpError{Op: "dial", Err: errors.New("no such host")}
	for range breakerThreshold - 1 {
		assert.Error(t, run())
	}
	reply = nil
	assert.NoError(t, run())

	// Consecutive connectivity failures open it: commands fail without reaching Redis.
	reply = &net.OpError{Op: "dial", Err: errors.New("no such host")}
	for range breakerThreshold {
		assert.Error(t, run())
	}
	before := calls
	assert.ErrorIs(t, run(), ErrCircuitOpen)
	assert.Equal(t, before, calls, "an open breaker does not call Redis")

	// After the cooldown one command goes through; a failure reopens it at once.
	clk.t = clk.t.Add(time.Second)
	assert.NotErrorIs(t, run(), ErrCircuitOpen)
	assert.ErrorIs(t, run(), ErrCircuitOpen)

	// A success after the next cooldown closes it.
	clk.t = clk.t.Add(time.Second)
	reply = nil
	assert.NoError(t, run())
	assert.NoError(t, run())
	assert.Equal(t, int64(0), b.openUntil.Load())
}

func TestUnreachable(t *testing.T) {
	assert.True(t, unreachable(context.DeadlineExceeded))
	assert.True(t, unreachable(redis.ErrPoolTimeout))
	assert.True(t, unreachable(errors.New("LOADING Redis is loading the dataset in memory")))
	assert.False(t, unreachable(redis.Nil))
	assert.False(t, unreachable(errors.New("NOSCRIPT No matching script")))
}
