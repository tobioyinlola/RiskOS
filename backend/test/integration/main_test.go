//go:build integration

// Package integration tests the backend against real PostgreSQL and Redis started with
// testcontainers. Run with `make test-integration` (requires Docker). Dependencies are never
// mocked here: these tests exist to catch what mocks hide.
package integration

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"

	"github.com/tobioyinlola/RiskOS/backend/internal/dbmigrate"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/config"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/db"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/redisx"
)

// Images match docker-compose.yml so tests run against what developers and CI deploy.
const (
	postgresImage = "postgres:16.15-alpine@sha256:721873c34ceb9f8d8fc265984940dc982404c105f19ad51be9fdc5970a6080ea"
	redisImage    = "redis:7.4.11-alpine@sha256:858f009f9709ce576febc734aa78b8f6d624b82571f9ddb6bda4377c833b3499"
)

// env holds the shared containers and clients for the package's tests.
type env struct {
	postgres *tcpostgres.PostgresContainer
	redis    *tcredis.RedisContainer
	pgCfg    config.Postgres
	redisCfg config.Redis
	// pool connects as riskos_app, the application role: row-level security applies, exactly as
	// for the api and worker.
	pool *pgxpool.Pool
	// owner connects as the schema owner, for migrations and for tests that must bypass RLS on
	// purpose (tampering, suspending a tenant).
	owner  *pgxpool.Pool
	rdb    *redis.Client
	logger *slog.Logger
}

const appRolePassword = "integration-app-only"

// shared is initialized once by TestMain and read-only afterwards.
var shared env //nolint:gochecknoglobals // test fixture shared across the package's tests

func TestMain(m *testing.M) {
	code, err := setup(m)
	if err != nil {
		fmt.Fprintln(os.Stderr, "integration setup:", err)
		os.Exit(1)
	}
	os.Exit(code)
}

func setup(m *testing.M) (int, error) {
	ctx := context.Background()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	pg, err := tcpostgres.Run(ctx, postgresImage,
		tcpostgres.WithDatabase("riskos"),
		tcpostgres.WithUsername("riskos"),
		tcpostgres.WithPassword("integration-test-only"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		return 0, fmt.Errorf("start postgres: %w", err)
	}
	defer func() { _ = testcontainers.TerminateContainer(pg) }()

	rc, err := tcredis.Run(ctx, redisImage)
	if err != nil {
		return 0, fmt.Errorf("start redis: %w", err)
	}
	defer func() { _ = testcontainers.TerminateContainer(rc) }()

	// Migrate as the owner, then give the application role a login, as the platform does in
	// deployed environments (TENANCY.md).
	ownerDSN, err := pg.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return 0, err
	}
	owner, err := db.NewPool(ctx, poolConfig(ownerDSN))
	if err != nil {
		return 0, err
	}
	if _, err := dbmigrate.Up(ctx, owner, logger); err != nil {
		owner.Close()
		return 0, err
	}
	_, err = owner.Exec(ctx, "ALTER ROLE "+dbmigrate.AppRole+" LOGIN PASSWORD '"+appRolePassword+"'")
	owner.Close()
	if err != nil {
		return 0, fmt.Errorf("enable application role login: %w", err)
	}

	shared = env{postgres: pg, redis: rc, logger: logger}
	if err := shared.connect(ctx); err != nil {
		return 0, err
	}
	defer shared.close()
	return m.Run(), nil
}

func poolConfig(dsn string) config.Postgres {
	return config.Postgres{
		URL: dsn, MaxConns: 10, MinConns: 0, MaxConnLifetime: time.Hour, MaxConnIdleTime: time.Minute,
		ConnectTimeout: 5 * time.Second, CheckTimeout: time.Second,
	}
}

// connect (re)creates the pool and Redis client from the containers' current endpoints. Docker
// assigns a new host port when a container restarts, so tests that stop a dependency call this
// after starting it again.
func (e *env) connect(ctx context.Context) error {
	e.close()
	ownerDSN, err := e.postgres.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return err
	}
	u, err := url.Parse(ownerDSN)
	if err != nil {
		return err
	}
	u.User = url.UserPassword(dbmigrate.AppRole, appRolePassword)
	redisEndpoint, err := e.redis.Endpoint(ctx, "")
	if err != nil {
		return err
	}
	e.pgCfg = poolConfig(u.String())
	e.redisCfg = config.Redis{
		Addr: redisEndpoint, PoolSize: 5, DialTimeout: time.Second,
		ReadTimeout: 500 * time.Millisecond, WriteTimeout: 500 * time.Millisecond, CheckTimeout: time.Second,
		BreakerCooldown: time.Second,
	}
	if e.pool, err = db.NewPool(ctx, e.pgCfg); err != nil {
		return err
	}
	if e.owner, err = db.NewPool(ctx, poolConfig(ownerDSN)); err != nil {
		return err
	}
	if e.rdb, err = redisx.New(ctx, e.redisCfg, slog.New(slog.DiscardHandler)); err != nil {
		return err
	}
	return nil
}

func (e *env) close() {
	for _, p := range []**pgxpool.Pool{&e.pool, &e.owner} {
		if *p != nil {
			(*p).Close()
			*p = nil
		}
	}
	if e.rdb != nil {
		_ = e.rdb.Close()
		e.rdb = nil
	}
}
