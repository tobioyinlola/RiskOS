// Package config loads and validates process configuration from the environment (12-factor).
//
// Configuration is read once in main, validated, and passed down explicitly. Invalid or missing
// required values fail startup with an error that names every offending variable. Secret values
// are never included in error messages.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Environment identifies the deployment environment of the process.
type Environment string

// Supported environments.
const (
	EnvDevelopment Environment = "development"
	EnvTest        Environment = "test"
	EnvProduction  Environment = "production"
)

// Config is the complete, validated configuration shared by the api, worker and migrate binaries.
type Config struct {
	Env         Environment
	ServiceName string
	LogLevel    slog.Level

	HTTP      HTTP
	Worker    Worker
	Postgres  Postgres
	Redis     Redis
	Telemetry Telemetry
}

// Worker configures the background job worker.
type Worker struct {
	Concurrency     int
	JobTimeout      time.Duration
	ShutdownTimeout time.Duration
}

// HTTP configures the API server. Every timeout is explicit; see docs/ARCHITECTURE.md.
type HTTP struct {
	Addr              string
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	RequestTimeout    time.Duration
	MaxHeaderBytes    int
	MaxBodyBytes      int64
	// ShutdownDrainDelay is how long readiness reports "unavailable" before the listener closes,
	// giving load balancers time to stop routing new traffic.
	ShutdownDrainDelay time.Duration
	ShutdownTimeout    time.Duration
}

// Postgres configures the pgx connection pool.
type Postgres struct {
	URL             string
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
	MaxConnIdleTime time.Duration
	ConnectTimeout  time.Duration
	CheckTimeout    time.Duration
}

// Redis configures the Redis client.
type Redis struct {
	Addr         string
	Password     string
	DB           int
	PoolSize     int
	DialTimeout  time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	CheckTimeout time.Duration
}

// Telemetry configures OpenTelemetry export. When disabled, no-op providers are installed.
type Telemetry struct {
	Enabled      bool
	OTLPEndpoint string
	OTLPInsecure bool
	SampleRatio  float64
}

// Load reads configuration using lookup (normally os.LookupEnv) and validates it.
func Load(lookup func(string) (string, bool)) (Config, error) {
	r := reader{lookup: lookup}

	cfg := Config{
		Env:         Environment(r.str("RISKOS_ENV", string(EnvDevelopment))),
		ServiceName: r.str("RISKOS_SERVICE_NAME", "riskos"),
		LogLevel:    r.logLevel("RISKOS_LOG_LEVEL", slog.LevelInfo),
		HTTP: HTTP{
			Addr:               r.str("RISKOS_HTTP_ADDR", ":8080"),
			ReadHeaderTimeout:  r.duration("RISKOS_HTTP_READ_HEADER_TIMEOUT", 5*time.Second),
			ReadTimeout:        r.duration("RISKOS_HTTP_READ_TIMEOUT", 10*time.Second),
			WriteTimeout:       r.duration("RISKOS_HTTP_WRITE_TIMEOUT", 15*time.Second),
			IdleTimeout:        r.duration("RISKOS_HTTP_IDLE_TIMEOUT", 60*time.Second),
			RequestTimeout:     r.duration("RISKOS_HTTP_REQUEST_TIMEOUT", 10*time.Second),
			MaxHeaderBytes:     r.integer("RISKOS_HTTP_MAX_HEADER_BYTES", 16<<10),
			MaxBodyBytes:       r.int64("RISKOS_HTTP_MAX_BODY_BYTES", 64<<10), // ARCHITECTURE.md §11
			ShutdownDrainDelay: r.duration("RISKOS_HTTP_SHUTDOWN_DRAIN_DELAY", 0),
			ShutdownTimeout:    r.duration("RISKOS_HTTP_SHUTDOWN_TIMEOUT", 20*time.Second),
		},
		Worker: Worker{
			Concurrency:     r.integer("RISKOS_WORKER_CONCURRENCY", 10),
			JobTimeout:      r.duration("RISKOS_WORKER_JOB_TIMEOUT", time.Minute),
			ShutdownTimeout: r.duration("RISKOS_WORKER_SHUTDOWN_TIMEOUT", 30*time.Second),
		},
		Postgres: Postgres{
			URL:             r.required("RISKOS_POSTGRES_URL"),
			MaxConns:        r.int32("RISKOS_POSTGRES_MAX_CONNS", 20),
			MinConns:        r.int32("RISKOS_POSTGRES_MIN_CONNS", 2),
			MaxConnLifetime: r.duration("RISKOS_POSTGRES_MAX_CONN_LIFETIME", time.Hour),
			MaxConnIdleTime: r.duration("RISKOS_POSTGRES_MAX_CONN_IDLE_TIME", 15*time.Minute),
			ConnectTimeout:  r.duration("RISKOS_POSTGRES_CONNECT_TIMEOUT", 5*time.Second),
			CheckTimeout:    r.duration("RISKOS_POSTGRES_CHECK_TIMEOUT", time.Second),
		},
		Redis: Redis{
			Addr:         r.required("RISKOS_REDIS_ADDR"),
			Password:     r.str("RISKOS_REDIS_PASSWORD", ""),
			DB:           r.integer("RISKOS_REDIS_DB", 0),
			PoolSize:     r.integer("RISKOS_REDIS_POOL_SIZE", 20),
			DialTimeout:  r.duration("RISKOS_REDIS_DIAL_TIMEOUT", 2*time.Second),
			ReadTimeout:  r.duration("RISKOS_REDIS_READ_TIMEOUT", 500*time.Millisecond),
			WriteTimeout: r.duration("RISKOS_REDIS_WRITE_TIMEOUT", 500*time.Millisecond),
			CheckTimeout: r.duration("RISKOS_REDIS_CHECK_TIMEOUT", time.Second),
		},
		Telemetry: Telemetry{
			Enabled:      r.boolean("RISKOS_OTEL_ENABLED", false),
			OTLPEndpoint: r.str("RISKOS_OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4318"),
			OTLPInsecure: r.boolean("RISKOS_OTEL_EXPORTER_OTLP_INSECURE", false),
			SampleRatio:  r.float("RISKOS_OTEL_SAMPLE_RATIO", 1.0),
		},
	}

	errs := append(r.errs, cfg.validate()...)
	if len(errs) > 0 {
		return Config{}, fmt.Errorf("invalid configuration: %w", errors.Join(errs...))
	}
	return cfg, nil
}

func (c Config) validate() []error {
	var errs []error
	add := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }

	switch c.Env {
	case EnvDevelopment, EnvTest, EnvProduction:
	default:
		add("RISKOS_ENV: must be one of development, test, production")
	}
	if strings.TrimSpace(c.ServiceName) == "" {
		add("RISKOS_SERVICE_NAME: must not be empty")
	}

	for name, d := range map[string]time.Duration{
		"RISKOS_HTTP_READ_HEADER_TIMEOUT":    c.HTTP.ReadHeaderTimeout,
		"RISKOS_HTTP_READ_TIMEOUT":           c.HTTP.ReadTimeout,
		"RISKOS_HTTP_WRITE_TIMEOUT":          c.HTTP.WriteTimeout,
		"RISKOS_HTTP_IDLE_TIMEOUT":           c.HTTP.IdleTimeout,
		"RISKOS_HTTP_REQUEST_TIMEOUT":        c.HTTP.RequestTimeout,
		"RISKOS_HTTP_SHUTDOWN_TIMEOUT":       c.HTTP.ShutdownTimeout,
		"RISKOS_WORKER_JOB_TIMEOUT":          c.Worker.JobTimeout,
		"RISKOS_WORKER_SHUTDOWN_TIMEOUT":     c.Worker.ShutdownTimeout,
		"RISKOS_POSTGRES_MAX_CONN_LIFETIME":  c.Postgres.MaxConnLifetime,
		"RISKOS_POSTGRES_MAX_CONN_IDLE_TIME": c.Postgres.MaxConnIdleTime,
		"RISKOS_POSTGRES_CONNECT_TIMEOUT":    c.Postgres.ConnectTimeout,
		"RISKOS_POSTGRES_CHECK_TIMEOUT":      c.Postgres.CheckTimeout,
		"RISKOS_REDIS_DIAL_TIMEOUT":          c.Redis.DialTimeout,
		"RISKOS_REDIS_READ_TIMEOUT":          c.Redis.ReadTimeout,
		"RISKOS_REDIS_WRITE_TIMEOUT":         c.Redis.WriteTimeout,
		"RISKOS_REDIS_CHECK_TIMEOUT":         c.Redis.CheckTimeout,
	} {
		if d <= 0 {
			add("%s: must be a positive duration", name)
		}
	}
	if c.HTTP.ShutdownDrainDelay < 0 {
		add("RISKOS_HTTP_SHUTDOWN_DRAIN_DELAY: must not be negative")
	}
	if c.HTTP.RequestTimeout > c.HTTP.WriteTimeout {
		add("RISKOS_HTTP_REQUEST_TIMEOUT: must not exceed RISKOS_HTTP_WRITE_TIMEOUT")
	}
	if c.HTTP.MaxHeaderBytes < 1<<10 {
		add("RISKOS_HTTP_MAX_HEADER_BYTES: must be at least 1024")
	}
	if c.HTTP.MaxBodyBytes < 1 {
		add("RISKOS_HTTP_MAX_BODY_BYTES: must be positive")
	}

	if c.Worker.Concurrency < 1 {
		add("RISKOS_WORKER_CONCURRENCY: must be at least 1")
	}

	if c.Postgres.URL != "" {
		u, err := url.Parse(c.Postgres.URL)
		if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Host == "" {
			// The URL may contain a password, so it is never echoed back.
			add("RISKOS_POSTGRES_URL: must be a postgres:// URL with a host")
		}
	}
	if c.Postgres.MaxConns < 1 {
		add("RISKOS_POSTGRES_MAX_CONNS: must be at least 1")
	}
	if c.Postgres.MinConns < 0 || c.Postgres.MinConns > c.Postgres.MaxConns {
		add("RISKOS_POSTGRES_MIN_CONNS: must be between 0 and RISKOS_POSTGRES_MAX_CONNS")
	}

	if c.Redis.DB < 0 {
		add("RISKOS_REDIS_DB: must not be negative")
	}
	if c.Redis.PoolSize < 1 {
		add("RISKOS_REDIS_POOL_SIZE: must be at least 1")
	}

	if c.Telemetry.SampleRatio < 0 || c.Telemetry.SampleRatio > 1 {
		add("RISKOS_OTEL_SAMPLE_RATIO: must be between 0 and 1")
	}
	if c.Telemetry.Enabled && strings.TrimSpace(c.Telemetry.OTLPEndpoint) == "" {
		add("RISKOS_OTEL_EXPORTER_OTLP_ENDPOINT: required when RISKOS_OTEL_ENABLED=true")
	}

	if c.Env == EnvProduction {
		if u, err := url.Parse(c.Postgres.URL); err == nil {
			switch u.Query().Get("sslmode") {
			case "verify-full", "verify-ca":
			default:
				add("RISKOS_POSTGRES_URL: production requires sslmode=verify-full or verify-ca")
			}
		}
		if c.Redis.Password == "" {
			add("RISKOS_REDIS_PASSWORD: required in production")
		}
		if c.Telemetry.OTLPInsecure {
			add("RISKOS_OTEL_EXPORTER_OTLP_INSECURE: must be false in production")
		}
	}
	return errs
}

// reader accumulates parse errors so that all problems are reported at once.
type reader struct {
	lookup func(string) (string, bool)
	errs   []error
}

func (r *reader) raw(key string) (string, bool) {
	v, ok := r.lookup(key)
	v = strings.TrimSpace(v)
	return v, ok && v != ""
}

func (r *reader) str(key, def string) string {
	if v, ok := r.raw(key); ok {
		return v
	}
	return def
}

func (r *reader) required(key string) string {
	v, ok := r.raw(key)
	if !ok {
		r.errs = append(r.errs, fmt.Errorf("%s: required", key))
	}
	return v
}

func (r *reader) integer(key string, def int) int {
	v, ok := r.raw(key)
	if !ok {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		r.errs = append(r.errs, fmt.Errorf("%s: must be an integer", key))
		return def
	}
	return n
}

func (r *reader) int64(key string, def int64) int64 {
	v, ok := r.raw(key)
	if !ok {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		r.errs = append(r.errs, fmt.Errorf("%s: must be an integer", key))
		return def
	}
	return n
}

func (r *reader) int32(key string, def int32) int32 {
	v, ok := r.raw(key)
	if !ok {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 32)
	if err != nil {
		r.errs = append(r.errs, fmt.Errorf("%s: must be a 32-bit integer", key))
		return def
	}
	return int32(n)
}

func (r *reader) float(key string, def float64) float64 {
	v, ok := r.raw(key)
	if !ok {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		r.errs = append(r.errs, fmt.Errorf("%s: must be a number", key))
		return def
	}
	return f
}

func (r *reader) boolean(key string, def bool) bool {
	v, ok := r.raw(key)
	if !ok {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		r.errs = append(r.errs, fmt.Errorf("%s: must be a boolean", key))
		return def
	}
	return b
}

func (r *reader) duration(key string, def time.Duration) time.Duration {
	v, ok := r.raw(key)
	if !ok {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		r.errs = append(r.errs, fmt.Errorf("%s: must be a duration such as 500ms or 5s", key))
		return def
	}
	return d
}

func (r *reader) logLevel(key string, def slog.Level) slog.Level {
	v, ok := r.raw(key)
	if !ok {
		return def
	}
	var l slog.Level
	if err := l.UnmarshalText([]byte(v)); err != nil {
		r.errs = append(r.errs, fmt.Errorf("%s: must be one of debug, info, warn, error", key))
		return def
	}
	return l
}
