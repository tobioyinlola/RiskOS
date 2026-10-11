package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base32"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/tobioyinlola/RiskOS/backend/internal/audit"
	"github.com/tobioyinlola/RiskOS/backend/internal/auth/internal/domain"
	"github.com/tobioyinlola/RiskOS/backend/internal/auth/internal/store"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/tenancy"
)

// dnsTimeout bounds the lookup that proves a domain.
const dnsTimeout = 5 * time.Second

// domainLabel is one label of a host name: letters, digits and inner hyphens.
var domainLabel = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`) //nolint:gochecknoglobals // compiled once

// allDigits rejects an address-like last label (1.2.3.4), which is never a public email domain.
var allDigits = regexp.MustCompile(`^[0-9]+$`) //nolint:gochecknoglobals // compiled once

// SSODomain is an email domain a workspace claims, and how to prove it.
type SSODomain struct {
	Domain     string
	Verified   bool
	VerifiedAt *time.Time
	CreatedAt  time.Time
	// TXTHost and TXTValue are the DNS record that proves the domain: a TXT record named TXTHost
	// whose value is TXTValue.
	TXTHost  string
	TXTValue string
}

// txtHost is where a domain publishes its proof, so the record is not mixed with the domain's
// other TXT records.
func txtHost(domainName string) string { return "_riskos-challenge." + domainName }

// domainToken is the value a workspace publishes to prove a domain: derived from the console
// secret, the workspace and the domain, so it needs no storage and cannot be published by
// anyone else for that workspace. Rotating the console secret invalidates pending proofs only;
// verified claims stay.
func (c *Console) domainToken(tenantID [16]byte, domainName string) string {
	m := hmac.New(sha256.New, c.cfg.Secret)
	m.Write([]byte("riskos-sso-domain\x00"))
	m.Write(tenantID[:])
	m.Write([]byte("\x00" + domainName))
	return "riskos-verification=" + strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(m.Sum(nil)[:20]))
}

// normalizeDomain returns domainName lower-cased and checked as a public email domain: at least
// two labels, no scheme, port, path or address.
func normalizeDomain(domainName string) (string, error) {
	d := strings.ToLower(strings.TrimSpace(domainName))
	d = strings.TrimSuffix(d, ".")
	bad := fmt.Errorf("%w: enter an email domain such as example.com", domain.ErrInvalidInput)
	if len(d) < 4 || len(d) > 253 {
		return "", bad
	}
	labels := strings.Split(d, ".")
	if len(labels) < 2 {
		return "", bad
	}
	for _, l := range labels {
		if !domainLabel.MatchString(l) {
			return "", bad
		}
	}
	if net.ParseIP(d) != nil || allDigits.MatchString(labels[len(labels)-1]) {
		return "", bad
	}
	return d, nil
}

func (c *Console) toDomain(tenantID [16]byte, d store.SSODomain) SSODomain {
	return SSODomain{
		Domain: d.Domain, Verified: d.VerifiedAt != nil, VerifiedAt: d.VerifiedAt, CreatedAt: d.CreatedAt,
		TXTHost: txtHost(d.Domain), TXTValue: c.domainToken(tenantID, d.Domain),
	}
}

// ListSSODomains returns the workspace's claimed domains. Requires team:read.
func (c *Console) ListSSODomains(ctx context.Context) ([]SSODomain, error) {
	if err := authz.Require(ctx, authz.TeamRead); err != nil {
		return nil, err
	}
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return nil, err
	}
	out := []SSODomain{}
	err = c.runner.Run(ctx, func(ctx context.Context) error {
		rows, err := c.store.SSODomains(ctx)
		if err != nil {
			return err
		}
		for _, r := range rows {
			out = append(out, c.toDomain(scope.TenantID, r))
		}
		return nil
	})
	return out, err
}

// AddSSODomain claims an email domain; it counts only once its TXT record is verified. Requires
// team:write; audited.
func (c *Console) AddSSODomain(ctx context.Context, domainName string) (SSODomain, error) {
	if err := authz.Require(ctx, authz.TeamWrite); err != nil {
		return SSODomain{}, err
	}
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return SSODomain{}, err
	}
	d, err := normalizeDomain(domainName)
	if err != nil {
		return SSODomain{}, err
	}
	now := c.clock.Now()
	var out SSODomain
	err = c.runner.Run(ctx, func(ctx context.Context) error {
		conn, err := c.store.SSOConnection(ctx)
		if err != nil {
			return err
		}
		if conn == nil {
			return fmt.Errorf("%w: set up single sign-on before claiming a domain", domain.ErrInvalidInput)
		}
		list, err := c.store.SSODomains(ctx)
		if err != nil {
			return err
		}
		if len(list) >= 20 {
			return fmt.Errorf("%w: a workspace can claim at most 20 domains", domain.ErrInvalidInput)
		}
		added, err := c.store.InsertSSODomain(ctx, d, now, scope.Principal.ID)
		if err != nil {
			return err
		}
		if !added {
			return fmt.Errorf("%w: this domain is already claimed by your workspace", domain.ErrConflict)
		}
		out = c.toDomain(scope.TenantID, store.SSODomain{TenantID: scope.TenantID, Domain: d, CreatedAt: now})
		return c.record(ctx, "sso.domain_added", "sso_domain", d, audit.OutcomeSuccess, nil)
	})
	return out, err
}

// VerifySSODomain looks for the proof in DNS and, if it is there, marks the claim verified. Only
// one workspace can hold a domain verified. Requires team:write; audited.
func (c *Console) VerifySSODomain(ctx context.Context, domainName string) (SSODomain, error) {
	if err := authz.Require(ctx, authz.TeamWrite); err != nil {
		return SSODomain{}, err
	}
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return SSODomain{}, err
	}
	d, err := normalizeDomain(domainName)
	if err != nil {
		return SSODomain{}, err
	}
	var claim *store.SSODomain
	if err := c.runner.Run(ctx, func(ctx context.Context) error {
		var err error
		claim, err = c.store.SSODomain(ctx, d)
		return err
	}); err != nil {
		return SSODomain{}, err
	}
	if claim == nil {
		return SSODomain{}, domain.ErrSSONotFound
	}
	if claim.VerifiedAt != nil {
		return c.toDomain(scope.TenantID, *claim), nil
	}

	// Network call: outside the transaction.
	want := c.domainToken(scope.TenantID, d)
	lookup := c.cfg.TXTLookup
	if lookup == nil {
		lookup = net.DefaultResolver.LookupTXT
	}
	lctx, cancel := context.WithTimeout(ctx, dnsTimeout)
	defer cancel()
	records, err := lookup(lctx, txtHost(d))
	var dnsErr *net.DNSError
	if err != nil && (!errors.As(err, &dnsErr) || !dnsErr.IsNotFound) {
		c.logger.InfoContext(ctx, "sso domain lookup failed", slog.String("domain", d), slog.Any("error", err))
		return SSODomain{}, fmt.Errorf("%w: RiskOS could not read DNS for %s just now; try again shortly", domain.ErrInvalidInput, txtHost(d))
	}
	if !slices.ContainsFunc(records, func(r string) bool { return strings.TrimSpace(r) == want }) {
		return SSODomain{}, fmt.Errorf("%w: no TXT record named %s with the value %s was found yet; DNS changes can take a while to appear",
			domain.ErrInvalidInput, txtHost(d), want)
	}

	now := c.clock.Now()
	var out SSODomain
	err = c.runner.Run(ctx, func(ctx context.Context) error {
		ok, err := c.store.VerifySSODomain(ctx, d, now)
		if err != nil {
			return err
		}
		if !ok {
			return domain.ErrSSONotFound
		}
		out = c.toDomain(scope.TenantID, store.SSODomain{TenantID: scope.TenantID, Domain: d, CreatedAt: claim.CreatedAt, VerifiedAt: &now})
		return c.record(ctx, "sso.domain_verified", "sso_domain", d, audit.OutcomeSuccess, nil)
	})
	if errors.Is(err, store.ErrDomainTaken) {
		return SSODomain{}, fmt.Errorf("%w: another workspace has already verified this domain", domain.ErrConflict)
	}
	return out, err
}

// RemoveSSODomain drops a claim. Accounts already created through it stay; with no verified
// domain left, nobody new can be let in just in time. Requires team:write; audited.
func (c *Console) RemoveSSODomain(ctx context.Context, domainName string) error {
	if err := authz.Require(ctx, authz.TeamWrite); err != nil {
		return err
	}
	d, err := normalizeDomain(domainName)
	if err != nil {
		return err
	}
	return c.runner.Run(ctx, func(ctx context.Context) error {
		ok, err := c.store.DeleteSSODomain(ctx, d)
		if err != nil {
			return err
		}
		if !ok {
			return domain.ErrSSONotFound
		}
		return c.record(ctx, "sso.domain_removed", "sso_domain", d, audit.OutcomeSuccess, nil)
	})
}
