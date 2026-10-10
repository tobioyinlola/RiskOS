package service

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"net/netip"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"

	"github.com/tobioyinlola/RiskOS/backend/internal/admin/internal/store"
	"github.com/tobioyinlola/RiskOS/backend/internal/admin/internal/store/sqlcgen"
	"github.com/tobioyinlola/RiskOS/backend/internal/audit"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/clock"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/tenancy"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/txn"
	"github.com/tobioyinlola/RiskOS/backend/internal/tenants"
)

// Errors.
var (
	ErrUnauthenticated = errors.New("staff session required")
	ErrInvalid         = errors.New("invalid input")
	ErrNotFound        = errors.New("not found")
	ErrConflict        = errors.New("not allowed in the current state")
	ErrForbidden       = errors.New("forbidden")
	ErrTenantNotActive = errors.New("tenant is not active")
	ErrPasskey         = errors.New("passkey verification failed")
	ErrRateLimited     = errors.New("too many attempts")
)

// Config configures the realm.
type Config struct {
	// Secret derives the CSRF key (the console secret; the derivation keeps the keys distinct).
	Secret []byte
	// SecretPrevious, during a rotation (ADR-0008), is the console secret in force before Secret;
	// CSRF tokens derived from it are still accepted.
	SecretPrevious []byte
	// RPID and Origins configure passkeys (the console's).
	RPID    string
	Origins []string
	// IdleTimeout ends a session not used for this long; MaxAge ends it regardless.
	IdleTimeout time.Duration
	MaxAge      time.Duration
	// EnrollTTL bounds the enrollment link the operator CLI prints.
	EnrollTTL time.Duration
}

// DefaultConfig returns the realm's defaults: 15 minutes idle, 4 hours at most, 24-hour links.
func DefaultConfig() Config {
	return Config{IdleTimeout: 15 * time.Minute, MaxAge: 4 * time.Hour, EnrollTTL: 24 * time.Hour}
}

// Tenants gives staff a tenant scope (the tenants module).
type Tenants interface {
	StaffScope(ctx context.Context, tenantID uuid.UUID, kind tenancy.EnvironmentKind, staffRef string, permissions []string) (context.Context, error)
	StaffOnboarding(ctx context.Context) (tenants.Onboarding, error)
	ReviewOnboarding(ctx context.Context, approve bool, note string, now time.Time) (tenants.Onboarding, error)
}

// TenantErrors classifies the tenants module's errors.
type TenantErrors struct {
	NotFound  error
	NotActive error
}

// Limiter throttles unauthenticated requests per client.
type Limiter interface {
	Allow(ctx context.Context, key string) bool
}

// Service is the admin realm.
type Service struct {
	store     store.Store
	tenants   Tenants
	tenantErr TenantErrors
	runner    txn.Runner
	audit     audit.Recorder
	clock     clock.Clock
	wa        *webauthn.WebAuthn
	csrfKey   []byte
	// csrfKeyPrevious is derived from Config.SecretPrevious during a rotation, nil otherwise.
	csrfKeyPrevious []byte
	cfg             Config
	limiter         Limiter
	logger          *slog.Logger
}

// New returns the service.
func New(st store.Store, tenants Tenants, tenantErr TenantErrors, runner txn.Runner, rec audit.Recorder, clk clock.Clock,
	limiter Limiter, cfg Config, logger *slog.Logger,
) (*Service, error) {
	if len(cfg.Secret) < 32 {
		// Operator tooling issues no sessions; the API's configuration requires the secret.
		cfg.Secret = make([]byte, 32)
		if _, err := rand.Read(cfg.Secret); err != nil {
			return nil, fmt.Errorf("admin: generate secret: %w", err)
		}
	}
	if cfg.RPID == "" {
		cfg.RPID = "localhost"
	}
	if len(cfg.Origins) == 0 {
		cfg.Origins = []string{"http://localhost:3000"}
	}
	wa, err := webauthn.New(&webauthn.Config{RPID: cfg.RPID, RPDisplayName: "RiskOS staff", RPOrigins: cfg.Origins})
	if err != nil {
		return nil, fmt.Errorf("admin: configure passkeys: %w", err)
	}
	m := hmac.New(sha256.New, cfg.Secret)
	m.Write([]byte("riskos-staff-csrf-key"))
	var previous []byte
	if len(cfg.SecretPrevious) > 0 {
		p := hmac.New(sha256.New, cfg.SecretPrevious)
		p.Write([]byte("riskos-staff-csrf-key"))
		previous = p.Sum(nil)
	}
	return &Service{
		store: st, tenants: tenants, tenantErr: tenantErr, runner: runner, audit: rec, clock: clk,
		wa: wa, csrfKey: m.Sum(nil), csrfKeyPrevious: previous, cfg: cfg, limiter: limiter, logger: logger,
	}, nil
}

// Client describes where a request came from.
type Client struct {
	IP        *netip.Addr
	UserAgent string
}

func newToken() (token string, hash []byte, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", nil, fmt.Errorf("generate token: %w", err)
	}
	token = hex.EncodeToString(b)
	return token, hashToken(token), nil
}

func hashToken(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}

func staffRef(id uuid.UUID) string { return ids.Format(ids.PrefixStaff, id) }

// record writes the staff audit log with q, so the entry commits with the change it records.
func (s *Service) record(ctx context.Context, q *sqlcgen.Queries, actor, action string, tenantID *uuid.UUID, resource, outcome string, meta map[string]any) error {
	if meta == nil {
		meta = map[string]any{}
	}
	raw, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("encode staff audit metadata: %w", err)
	}
	return store.Classify(q.InsertStaffAudit(ctx, sqlcgen.InsertStaffAuditParams{
		ID: ids.NewV7(), Now: s.clock.Now(), Actor: actor, Action: action, TenantID: tenantID,
		ResourceID: resource, Outcome: outcome, Metadata: raw,
	}), "record staff audit")
}

// Staff is a staff member.
type Staff = sqlcgen.Staff

func validStaffRole(r authz.Role) bool {
	return r == authz.RoleOperationsAdmin || r == authz.RoleSecurityAdmin
}

// CreateStaff creates a staff member (operator CLI only) and returns the single-use token of
// their enrollment link.
func (s *Service) CreateStaff(ctx context.Context, email, name string, role authz.Role, operator string) (string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if addr, err := mail.ParseAddress(email); err != nil || addr.Address != email || len(email) > 254 {
		return "", fmt.Errorf("%w: email is not a valid address", ErrInvalid)
	}
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 100 {
		return "", fmt.Errorf("%w: name must be 1 to 100 characters", ErrInvalid)
	}
	if !validStaffRole(role) {
		return "", fmt.Errorf("%w: role must be operations_admin or security_admin", ErrInvalid)
	}
	if strings.TrimSpace(operator) == "" {
		return "", fmt.Errorf("%w: the operator is required", ErrInvalid)
	}
	token, hash, err := newToken()
	if err != nil {
		return "", err
	}
	now := s.clock.Now()
	id := ids.NewV7()
	err = s.store.Tx(ctx, func(q *sqlcgen.Queries) error {
		exp := now.Add(s.cfg.EnrollTTL)
		if err := store.Classify(q.InsertStaff(ctx, sqlcgen.InsertStaffParams{
			ID: id, Email: email, Name: name, Role: string(role), EnrollTokenHash: hash, EnrollExpiresAt: &exp,
			Now: now, CreatedBy: "riskosctl:" + operator,
		}), "insert staff"); err != nil {
			return err
		}
		return s.record(ctx, q, "riskosctl:"+operator, "staff.created", nil, staffRef(id), "success", map[string]any{"role": string(role)})
	})
	if errors.Is(err, store.ErrDuplicate) {
		return "", fmt.Errorf("%w: a staff member with this email exists", ErrConflict)
	}
	return token, err
}

// ReissueEnrollment gives an active staff member a new enrollment link (for example to add a
// passkey after losing one). Operator CLI only.
func (s *Service) ReissueEnrollment(ctx context.Context, email, operator string) (string, error) {
	token, hash, err := newToken()
	if err != nil {
		return "", err
	}
	err = s.store.Tx(ctx, func(q *sqlcgen.Queries) error {
		st, err := q.StaffByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
		if err := store.Classify(err, "get staff"); err != nil {
			return err
		}
		if st.Status != "active" {
			return fmt.Errorf("%w: the staff member is disabled", ErrConflict)
		}
		exp := s.clock.Now().Add(s.cfg.EnrollTTL)
		if _, err := q.SetEnrollToken(ctx, sqlcgen.SetEnrollTokenParams{ID: st.ID, TokenHash: hash, ExpiresAt: &exp}); err != nil {
			return store.Classify(err, "set enrollment")
		}
		return s.record(ctx, q, "riskosctl:"+operator, "staff.enrollment_issued", nil, staffRef(st.ID), "success", nil)
	})
	if errors.Is(err, store.ErrNotFound) {
		return "", ErrNotFound
	}
	return token, err
}

// SetStaffStatus enables or disables a staff member (operator CLI only); disabling ends their
// sessions at once.
func (s *Service) SetStaffStatus(ctx context.Context, email string, active bool, operator string) error {
	status := "disabled"
	if active {
		status = "active"
	}
	err := s.store.Tx(ctx, func(q *sqlcgen.Queries) error {
		st, err := q.StaffByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
		if err := store.Classify(err, "get staff"); err != nil {
			return err
		}
		if _, err := q.SetStaffStatus(ctx, sqlcgen.SetStaffStatusParams{ID: st.ID, Status: status}); err != nil {
			return store.Classify(err, "set staff status")
		}
		if !active {
			if err := q.RevokeStaffSessions(ctx, sqlcgen.RevokeStaffSessionsParams{StaffID: st.ID, Now: s.clock.Now()}); err != nil {
				return store.Classify(err, "revoke staff sessions")
			}
		}
		return s.record(ctx, q, "riskosctl:"+operator, "staff."+map[bool]string{true: "enabled", false: "disabled"}[active], nil, staffRef(st.ID), "success", nil)
	})
	if errors.Is(err, store.ErrNotFound) {
		return ErrNotFound
	}
	return err
}

// ListStaff returns every staff member.
func (s *Service) ListStaff(ctx context.Context) ([]Staff, error) {
	out, err := s.store.Q().ListStaff(ctx)
	return out, store.Classify(err, "list staff")
}
