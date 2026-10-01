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
	"net/mail"
	"net/netip"
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
	Auth      Auth
	RateLimit RateLimit
	Risk      Risk
	Webhooks  Webhooks
	Console   Console
	SMTP      SMTP
}

// Console configures the dashboard realm (console sign-in and sessions).
type Console struct {
	// Secret keys MFA secret encryption and CSRF tokens (at least 32 bytes, from the secrets
	// manager). Rotating it invalidates enrolled authenticator apps and open sessions' CSRF tokens.
	Secret string
	// URL is the console's public base URL (invite links, passkey origin).
	URL string
	// Origins may make cookie-authenticated requests; defaults to URL's origin.
	Origins []string
	// RPID is the passkey relying party ID; defaults to URL's host name.
	RPID string
	// CookieSecure sets the Secure attribute on the session cookie. Required in production.
	CookieSecure bool
	// BreachedPasswordCheck is "pwned" (check new passwords against the Pwned Passwords range
	// API, k-anonymity) or "off". BreachedPasswordURL overrides the API's address.
	BreachedPasswordCheck string
	BreachedPasswordURL   string
	// SSOAllowPrivate lets single sign-on identity providers be http and on private addresses:
	// local development and tests only (refused in production).
	SSOAllowPrivate bool
}

// Validate checks the console settings; call it in binaries that serve the console API or
// create invites.
func (c Console) Validate(env Environment) error {
	var errs []error
	if len(c.Secret) < MinPepperBytes {
		errs = append(errs, fmt.Errorf("RISKOS_CONSOLE_SECRET: required, at least %d bytes", MinPepperBytes))
	}
	if c.SSOAllowPrivate && env == EnvProduction {
		errs = append(errs, errors.New("RISKOS_SSO_ALLOW_PRIVATE: not allowed in production"))
	}
	if c.BreachedPasswordCheck != "" && c.BreachedPasswordCheck != "pwned" && c.BreachedPasswordCheck != "off" {
		errs = append(errs, errors.New("RISKOS_BREACHED_PASSWORD_CHECK: must be pwned or off"))
	}
	u, err := url.Parse(c.URL)
	if err != nil || !u.IsAbs() || u.Host == "" {
		errs = append(errs, errors.New("RISKOS_CONSOLE_URL: must be an absolute URL"))
	} else if env == EnvProduction && u.Scheme != "https" {
		errs = append(errs, errors.New("RISKOS_CONSOLE_URL: production requires https"))
	}
	if env == EnvProduction && !c.CookieSecure {
		errs = append(errs, errors.New("RISKOS_SESSION_COOKIE_SECURE: must be true in production"))
	}
	if len(errs) > 0 {
		return fmt.Errorf("invalid configuration: %w", errors.Join(errs...))
	}
	return nil
}

// Webhooks configures webhook signing and delivery.
type Webhooks struct {
	// SigningKey derives every endpoint's signing secret (at least 32 bytes, from the secrets
	// manager). Rotating it changes every endpoint's secret at once.
	SigningKey string
	// Timeout bounds each delivery attempt.
	Timeout time.Duration
	// Concurrency is the number of deliveries the worker runs at once.
	Concurrency int
	// AllowPrivateTest lets test-environment endpoints use http:// and private addresses, for
	// local development only. Refused when RISKOS_ENV=production.
	AllowPrivateTest bool
}

// Validate checks the webhook settings; call it in binaries that sign or deliver webhooks.
func (w Webhooks) Validate() error {
	var errs []error
	if len(w.SigningKey) < MinPepperBytes {
		errs = append(errs, fmt.Errorf("RISKOS_WEBHOOK_SIGNING_KEY: required, at least %d bytes", MinPepperBytes))
	}
	if w.Timeout < time.Second || w.Timeout > 30*time.Second {
		errs = append(errs, errors.New("RISKOS_WEBHOOK_TIMEOUT: must be between 1s and 30s"))
	}
	if w.Concurrency < 1 {
		errs = append(errs, errors.New("RISKOS_WEBHOOK_CONCURRENCY: must be at least 1"))
	}
	if len(errs) > 0 {
		return fmt.Errorf("invalid configuration: %w", errors.Join(errs...))
	}
	return nil
}

// Risk configures the risk engine.
type Risk struct {
	// FeatureHashKey keys the HMAC that pseudonymizes identifiers before they reach Redis
	// (ARCHITECTURE.md §16.3). At least 32 bytes, from the secrets manager. Changing it resets
	// every user's behavioral history.
	FeatureHashKey string
	// FeatureTimeout bounds the feature-store round trip on the decision path.
	FeatureTimeout time.Duration
	// HistoryWarmup is how long history signals are held back after the feature store lost its
	// state (DISASTER_RECOVERY.md). Zero detects and records resets without holding back.
	HistoryWarmup time.Duration
}

// Validate checks the risk settings; call it in binaries that evaluate events.
func (r Risk) Validate() error {
	var errs []error
	if len(r.FeatureHashKey) < MinPepperBytes {
		errs = append(errs, fmt.Errorf("RISKOS_FEATURE_HASH_KEY: required, at least %d bytes", MinPepperBytes))
	}
	if r.FeatureTimeout <= 0 || r.FeatureTimeout > time.Second {
		errs = append(errs, errors.New("RISKOS_FEATURE_TIMEOUT: must be between 1ms and 1s"))
	}
	if r.HistoryWarmup < 0 || r.HistoryWarmup > 90*24*time.Hour {
		errs = append(errs, errors.New("RISKOS_HISTORY_WARMUP: must be between 0 and 90 days"))
	}
	if len(errs) > 0 {
		return fmt.Errorf("invalid configuration: %w", errors.Join(errs...))
	}
	return nil
}

// SMTP configures outgoing email (invites). Email is off when Host is empty.
type SMTP struct {
	Host     string
	Port     int
	Username string
	Password string
	// From is the sender, e.g. "RiskOS <no-reply@example.com>".
	From string
	// TLS is starttls (default), tls (implicit, port 465) or none (local catchers only).
	TLS     string
	Timeout time.Duration
}

// Enabled reports whether email is configured.
func (s SMTP) Enabled() bool { return s.Host != "" }

// Validate checks the email settings when email is enabled.
func (s SMTP) Validate(env Environment) error {
	if !s.Enabled() {
		return nil
	}
	var errs []error
	if s.Port < 1 || s.Port > 65535 {
		errs = append(errs, errors.New("RISKOS_SMTP_PORT: must be a port number"))
	}
	if addr, err := mail.ParseAddress(s.From); err != nil || addr.Address == "" {
		errs = append(errs, errors.New("RISKOS_SMTP_FROM: required, e.g. RiskOS <no-reply@example.com>"))
	}
	switch s.TLS {
	case "starttls", "tls":
	case "none":
		if env == EnvProduction {
			errs = append(errs, errors.New("RISKOS_SMTP_TLS: production requires starttls or tls"))
		}
	default:
		errs = append(errs, errors.New("RISKOS_SMTP_TLS: must be starttls, tls or none"))
	}
	if s.Timeout <= 0 || s.Timeout > time.Minute {
		errs = append(errs, errors.New("RISKOS_SMTP_TIMEOUT: must be between 1ms and 1m"))
	}
	if len(errs) > 0 {
		return fmt.Errorf("invalid configuration: %w", errors.Join(errs...))
	}
	return nil
}

// RateLimit configures per-API-key and per-tenant request limits (sustained rate and burst).
type RateLimit struct {
	KeyPerSecond    float64
	KeyBurst        int
	TenantPerSecond float64
	TenantBurst     int
	// Timeout bounds each limit check; past it the request is allowed (limits fail open).
	Timeout time.Duration
	// AuthFailuresPerMinute and AuthFailuresBurst limit failed authentications per client
	// address; past them the address gets 429 before any credential lookup.
	AuthFailuresPerMinute float64
	AuthFailuresBurst     int
}

// Auth configures API key authentication. It is validated only by the binaries that
// authenticate (Auth.Validate), so processes that do not need the pepper never receive it.
type Auth struct {
	// APIKeyPepper keys the HMAC of stored API key secrets. It comes from the secrets manager,
	// must be at least 32 bytes, and must never change without a rotation plan (every key would
	// stop verifying).
	APIKeyPepper string
	// APIKeyCacheTTL bounds revocation staleness per instance (ARCHITECTURE.md §9: 5 s maximum).
	APIKeyCacheTTL time.Duration
}

// MinPepperBytes is the minimum API key pepper length.
const MinPepperBytes = 32

// MaxAPIKeyCacheTTL is the longest a revoked key may keep working on an instance.
const MaxAPIKeyCacheTTL = 5 * time.Second

// Validate checks the auth settings; call it in binaries that authenticate API keys.
func (a Auth) Validate() error {
	var errs []error
	if len(a.APIKeyPepper) < MinPepperBytes {
		errs = append(errs, fmt.Errorf("RISKOS_API_KEY_PEPPER: required, at least %d bytes", MinPepperBytes))
	}
	if a.APIKeyCacheTTL < 0 || a.APIKeyCacheTTL > MaxAPIKeyCacheTTL {
		errs = append(errs, fmt.Errorf("RISKOS_API_KEY_CACHE_TTL: must be between 0s and %s", MaxAPIKeyCacheTTL))
	}
	if len(errs) > 0 {
		return fmt.Errorf("invalid configuration: %w", errors.Join(errs...))
	}
	return nil
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
	// TrustedProxies are the networks of proxies in front of the API (load balancer, the
	// console's server). Only their X-Forwarded-For is believed; see httpx.ClientIP.
	TrustedProxies []netip.Prefix
	// ShutdownDrainDelay is how long readiness reports "unavailable" before the listener closes,
	// giving load balancers time to stop routing new traffic.
	ShutdownDrainDelay time.Duration
	ShutdownTimeout    time.Duration
}

// Postgres configures the pgx connection pool.
type Postgres struct {
	// URL connects as the application role (riskos_app): subject to row-level security, owns
	// nothing. Used by api, worker and riskosctl.
	URL string
	// MigrationURL connects as the schema owner. Only cmd/migrate and operator verification
	// (riskosctl audit verify) use it; it is never given to the api or worker.
	MigrationURL    string
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
	MaxConnIdleTime time.Duration
	ConnectTimeout  time.Duration
	CheckTimeout    time.Duration
	// AcquireTimeout bounds getting a connection and beginning a transaction. Past it the
	// request fails with 503 instead of queueing while the database is unreachable.
	AcquireTimeout time.Duration
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
	// BreakerCooldown is how long commands fail at once after Redis proves unreachable
	// (0 disables the circuit breaker).
	BreakerCooldown time.Duration
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
		Console: consoleConfig(&r),
		SMTP: SMTP{
			Host:     r.str("RISKOS_SMTP_HOST", ""),
			Port:     r.integer("RISKOS_SMTP_PORT", 587),
			Username: r.str("RISKOS_SMTP_USERNAME", ""),
			Password: r.str("RISKOS_SMTP_PASSWORD", ""),
			From:     r.str("RISKOS_SMTP_FROM", ""),
			TLS:      r.str("RISKOS_SMTP_TLS", "starttls"),
			Timeout:  r.duration("RISKOS_SMTP_TIMEOUT", 10*time.Second),
		},
		Webhooks: Webhooks{
			SigningKey:       r.str("RISKOS_WEBHOOK_SIGNING_KEY", ""),
			Timeout:          r.duration("RISKOS_WEBHOOK_TIMEOUT", 10*time.Second),
			Concurrency:      r.integer("RISKOS_WEBHOOK_CONCURRENCY", 10),
			AllowPrivateTest: r.boolean("RISKOS_WEBHOOK_ALLOW_PRIVATE_TEST", false),
		},
		Risk: Risk{
			FeatureHashKey: r.str("RISKOS_FEATURE_HASH_KEY", ""),
			FeatureTimeout: r.duration("RISKOS_FEATURE_TIMEOUT", 50*time.Millisecond),
			HistoryWarmup:  r.duration("RISKOS_HISTORY_WARMUP", 7*24*time.Hour),
		},
		RateLimit: RateLimit{
			KeyPerSecond:          r.float("RISKOS_RATE_LIMIT_KEY_PER_SECOND", 100),
			KeyBurst:              r.integer("RISKOS_RATE_LIMIT_KEY_BURST", 200),
			TenantPerSecond:       r.float("RISKOS_RATE_LIMIT_TENANT_PER_SECOND", 500),
			TenantBurst:           r.integer("RISKOS_RATE_LIMIT_TENANT_BURST", 1000),
			Timeout:               r.duration("RISKOS_RATE_LIMIT_TIMEOUT", 50*time.Millisecond),
			AuthFailuresPerMinute: r.float("RISKOS_RATE_LIMIT_AUTH_FAILURES_PER_MINUTE", 30),
			AuthFailuresBurst:     r.integer("RISKOS_RATE_LIMIT_AUTH_FAILURES_BURST", 30),
		},
		Auth: Auth{
			APIKeyPepper:   r.str("RISKOS_API_KEY_PEPPER", ""),
			APIKeyCacheTTL: r.duration("RISKOS_API_KEY_CACHE_TTL", MaxAPIKeyCacheTTL),
		},
		Env:         Environment(r.str("RISKOS_ENV", string(EnvDevelopment))),
		ServiceName: r.str("RISKOS_SERVICE_NAME", "riskos"),
		LogLevel:    r.logLevel("RISKOS_LOG_LEVEL", slog.LevelInfo),
		HTTP: HTTP{
			Addr:               r.str("RISKOS_HTTP_ADDR", ":8080"),
			TrustedProxies:     r.prefixes("RISKOS_TRUSTED_PROXIES"),
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
			MigrationURL:    r.str("RISKOS_POSTGRES_MIGRATION_URL", ""),
			MaxConns:        r.int32("RISKOS_POSTGRES_MAX_CONNS", 20),
			MinConns:        r.int32("RISKOS_POSTGRES_MIN_CONNS", 2),
			MaxConnLifetime: r.duration("RISKOS_POSTGRES_MAX_CONN_LIFETIME", time.Hour),
			MaxConnIdleTime: r.duration("RISKOS_POSTGRES_MAX_CONN_IDLE_TIME", 15*time.Minute),
			ConnectTimeout:  r.duration("RISKOS_POSTGRES_CONNECT_TIMEOUT", 5*time.Second),
			CheckTimeout:    r.duration("RISKOS_POSTGRES_CHECK_TIMEOUT", time.Second),
			AcquireTimeout:  r.duration("RISKOS_POSTGRES_ACQUIRE_TIMEOUT", time.Second),
		},
		Redis: Redis{
			Addr:            r.required("RISKOS_REDIS_ADDR"),
			Password:        r.str("RISKOS_REDIS_PASSWORD", ""),
			DB:              r.integer("RISKOS_REDIS_DB", 0),
			PoolSize:        r.integer("RISKOS_REDIS_POOL_SIZE", 20),
			DialTimeout:     r.duration("RISKOS_REDIS_DIAL_TIMEOUT", 2*time.Second),
			ReadTimeout:     r.duration("RISKOS_REDIS_READ_TIMEOUT", 500*time.Millisecond),
			WriteTimeout:    r.duration("RISKOS_REDIS_WRITE_TIMEOUT", 500*time.Millisecond),
			CheckTimeout:    r.duration("RISKOS_REDIS_CHECK_TIMEOUT", time.Second),
			BreakerCooldown: r.duration("RISKOS_REDIS_BREAKER_COOLDOWN", time.Second),
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
	for _, p := range c.HTTP.TrustedProxies {
		if p.Bits() == 0 && c.Env == EnvProduction {
			add("RISKOS_TRUSTED_PROXIES: production must name its proxies, not trust every address")
		}
	}
	if c.RateLimit.AuthFailuresPerMinute < 0 || c.RateLimit.AuthFailuresBurst < 1 {
		add("RISKOS_RATE_LIMIT_AUTH_FAILURES_PER_MINUTE: must be at least 0 (0 disables), with a burst of at least 1")
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
		"RISKOS_POSTGRES_ACQUIRE_TIMEOUT":    c.Postgres.AcquireTimeout,
		"RISKOS_RATE_LIMIT_TIMEOUT":          c.RateLimit.Timeout,
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

	if c.RateLimit.KeyPerSecond <= 0 || c.RateLimit.TenantPerSecond <= 0 {
		add("RISKOS_RATE_LIMIT_*_PER_SECOND: must be positive")
	}
	if c.RateLimit.KeyBurst < 1 || c.RateLimit.TenantBurst < 1 {
		add("RISKOS_RATE_LIMIT_*_BURST: must be at least 1")
	}

	if c.Worker.Concurrency < 1 {
		add("RISKOS_WORKER_CONCURRENCY: must be at least 1")
	}

	// URLs may contain passwords, so they are never echoed back in errors.
	if c.Postgres.URL != "" && !isPostgresURL(c.Postgres.URL) {
		add("RISKOS_POSTGRES_URL: must be a postgres:// URL with a host")
	}
	if c.Postgres.MigrationURL != "" && !isPostgresURL(c.Postgres.MigrationURL) {
		add("RISKOS_POSTGRES_MIGRATION_URL: must be a postgres:// URL with a host")
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

	if c.Env == EnvProduction && c.Webhooks.AllowPrivateTest {
		add("RISKOS_WEBHOOK_ALLOW_PRIVATE_TEST: must not be set in production")
	}
	if c.Env == EnvProduction {
		for name, raw := range map[string]string{"RISKOS_POSTGRES_URL": c.Postgres.URL, "RISKOS_POSTGRES_MIGRATION_URL": c.Postgres.MigrationURL} {
			if raw == "" {
				continue
			}
			if u, err := url.Parse(raw); err == nil {
				switch u.Query().Get("sslmode") {
				case "verify-full", "verify-ca":
				default:
					add("%s: production requires sslmode=verify-full or verify-ca", name)
				}
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

func isPostgresURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "postgres" || u.Scheme == "postgresql") && u.Host != ""
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

// prefixes reads a comma-separated list of CIDR prefixes (a bare address is a single host).
func (r *reader) prefixes(key string) []netip.Prefix {
	v, ok := r.raw(key)
	if !ok {
		return nil
	}
	var out []netip.Prefix
	for _, s := range strings.Split(v, ",") {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		p, err := netip.ParsePrefix(s)
		if err != nil {
			a, aerr := netip.ParseAddr(s)
			if aerr != nil {
				r.errs = append(r.errs, fmt.Errorf("%s: %q is not a CIDR prefix or address", key, s))
				continue
			}
			p = netip.PrefixFrom(a, a.BitLen())
		}
		out = append(out, p.Masked())
	}
	return out
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

func consoleConfig(r *reader) Console {
	c := Console{
		Secret:       r.str("RISKOS_CONSOLE_SECRET", ""),
		URL:          strings.TrimRight(r.str("RISKOS_CONSOLE_URL", "http://localhost:3000"), "/"),
		RPID:         r.str("RISKOS_WEBAUTHN_RP_ID", ""),
		CookieSecure: r.boolean("RISKOS_SESSION_COOKIE_SECURE", true),

		BreachedPasswordCheck: r.str("RISKOS_BREACHED_PASSWORD_CHECK", "pwned"),
		BreachedPasswordURL:   r.str("RISKOS_BREACHED_PASSWORD_URL", ""),
		SSOAllowPrivate:       r.boolean("RISKOS_SSO_ALLOW_PRIVATE", false),
	}
	if raw := r.str("RISKOS_CONSOLE_ORIGINS", ""); raw != "" {
		for _, o := range strings.Split(raw, ",") {
			if o = strings.TrimSpace(o); o != "" {
				c.Origins = append(c.Origins, strings.TrimRight(o, "/"))
			}
		}
	}
	if u, err := url.Parse(c.URL); err == nil && u.Host != "" {
		if len(c.Origins) == 0 {
			c.Origins = []string{u.Scheme + "://" + u.Host}
		}
		if c.RPID == "" {
			c.RPID = u.Hostname()
		}
	}
	return c
}
