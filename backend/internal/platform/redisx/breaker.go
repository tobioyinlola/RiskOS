package redisx

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

// ErrCircuitOpen is returned, without contacting Redis, while the breaker is open.
var ErrCircuitOpen = errors.New("redis unavailable: circuit open")

// breakerThreshold is how many consecutive connectivity failures open the breaker.
const breakerThreshold = 5

// breaker is a circuit breaker installed as a go-redis hook. When Redis is unreachable every
// command would otherwise wait for a dial or a timeout (a stopped container can take seconds
// just to fail a DNS lookup), and requests pile up behind it. After breakerThreshold
// consecutive connectivity failures the breaker fails every command at once for the cooldown,
// then lets traffic through again; one success resets it. Callers already treat Redis errors
// as degradation (rate limits fail open, risk signals are reported as degraded).
type breaker struct {
	cooldown  time.Duration
	now       func() time.Time
	logger    *slog.Logger
	failures  atomic.Int32
	openUntil atomic.Int64 // unix nanoseconds; zero when closed
}

func newBreaker(cooldown time.Duration, logger *slog.Logger) *breaker {
	return &breaker{cooldown: cooldown, now: time.Now, logger: logger}
}

func (b *breaker) allow() bool {
	until := b.openUntil.Load()
	return until == 0 || b.now().UnixNano() >= until
}

// record updates the breaker with a command's outcome. After a cooldown the breaker is
// half-open: traffic flows, and the first failure reopens it at once.
func (b *breaker) record(err error) {
	if err == nil || !unreachable(err) {
		b.failures.Store(0)
		if b.openUntil.Swap(0) != 0 {
			b.logger.Info("redis reachable again")
		}
		return
	}
	halfOpen := b.openUntil.Load() != 0
	if halfOpen || b.failures.Add(1) >= breakerThreshold {
		b.failures.Store(0)
		if prev := b.openUntil.Swap(b.now().Add(b.cooldown).UnixNano()); prev == 0 {
			b.logger.Warn("redis unreachable; failing fast", slog.Duration("cooldown", b.cooldown), slog.Any("error", err))
		}
	}
}

// unreachable reports whether err means Redis could not be reached in time, as opposed to a
// reply (a missing key, a script error) from a working server.
func unreachable(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) ||
		errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, redis.ErrPoolTimeout) || errors.Is(err, redis.ErrClosed) ||
		strings.HasPrefix(err.Error(), "LOADING")
}

var _ redis.Hook = (*breaker)(nil)

func (b *breaker) DialHook(next redis.DialHook) redis.DialHook {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		if !b.allow() {
			return nil, ErrCircuitOpen
		}
		conn, err := next(ctx, network, addr)
		if err != nil {
			b.record(err)
		}
		return conn, err
	}
}

func (b *breaker) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if !b.allow() {
			cmd.SetErr(ErrCircuitOpen)
			return ErrCircuitOpen
		}
		err := next(ctx, cmd)
		if !errors.Is(err, ErrCircuitOpen) {
			b.record(err)
		}
		return err
	}
}

func (b *breaker) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		if !b.allow() {
			for _, c := range cmds {
				c.SetErr(ErrCircuitOpen)
			}
			return ErrCircuitOpen
		}
		err := next(ctx, cmds)
		if !errors.Is(err, ErrCircuitOpen) {
			b.record(err)
		}
		return err
	}
}
