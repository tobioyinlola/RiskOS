// Package sso signs console users in with a tenant's OpenID Connect provider: the authorization
// code flow with PKCE (S256), state and nonce, and ID token verification (signature through the
// provider's published keys, issuer, audience, expiry, nonce). Requests to providers go through
// platform/egress, so a provider URL cannot reach internal networks.
package sso

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/tobioyinlola/RiskOS/backend/internal/platform/egress"
)

// Errors.
var (
	// ErrIssuer means the issuer URL is not acceptable or its discovery document failed.
	ErrIssuer = errors.New("identity provider unavailable or not an OpenID Connect issuer")
	// ErrRejected means the provider's answer did not verify (code, token, nonce).
	ErrRejected = errors.New("identity provider response rejected")
)

// providerTTL is how long a discovered provider (endpoints and key set) is reused.
const providerTTL = time.Hour

// Client talks to identity providers.
type Client struct {
	http         *http.Client
	allowPrivate bool

	mu        sync.Mutex
	providers map[string]cachedProvider
	now       func() time.Time
}

type cachedProvider struct {
	p  *oidc.Provider
	at time.Time
}

// New returns a client whose requests time out after timeout. allowPrivate permits http issuers
// and private addresses, for local development and tests only.
func New(timeout time.Duration, allowPrivate bool) *Client {
	return &Client{
		http: egress.NewClient(timeout, allowPrivate), allowPrivate: allowPrivate,
		providers: map[string]cachedProvider{}, now: time.Now,
	}
}

// Connection is a tenant's provider registration.
type Connection struct {
	Issuer       string
	ClientID     string
	ClientSecret string
	RedirectURL  string
	// Scopes are requested in addition to openid and email (Google asks for profile).
	Scopes []string
}

// Identity is what a verified ID token says about the person.
type Identity struct {
	Subject       string
	Email         string
	EmailVerified bool
	// Name is the person's name when the provider shares it (the profile scope).
	Name string
}

// CheckIssuer validates an issuer URL's form: https (http only when private addresses are
// allowed), a host, no credentials, query or fragment.
func (c *Client) CheckIssuer(issuer string) error {
	u, err := url.Parse(issuer)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" ||
		(u.Scheme != "https" && (u.Scheme != "http" || !c.allowPrivate)) {
		return fmt.Errorf("%w: the issuer must be an https URL", ErrIssuer)
	}
	return nil
}

// Discover fetches the issuer's discovery document, proving it is an OIDC provider.
func (c *Client) Discover(ctx context.Context, issuer string) error {
	_, err := c.provider(ctx, issuer, true)
	return err
}

func (c *Client) provider(ctx context.Context, issuer string, fresh bool) (*oidc.Provider, error) {
	if err := c.CheckIssuer(issuer); err != nil {
		return nil, err
	}
	c.mu.Lock()
	cp, ok := c.providers[issuer]
	c.mu.Unlock()
	if ok && !fresh && c.now().Sub(cp.at) < providerTTL {
		return cp.p, nil
	}
	// The provider keeps the HTTP client (not ctx's deadline) for later key set fetches.
	p, err := oidc.NewProvider(oidc.ClientContext(ctx, c.http), strings.TrimRight(issuer, "/"))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrIssuer, err)
	}
	c.mu.Lock()
	c.providers[issuer] = cachedProvider{p: p, at: c.now()}
	c.mu.Unlock()
	return p, nil
}

func (c *Client) oauth(p *oidc.Provider, conn Connection) *oauth2.Config {
	return &oauth2.Config{
		ClientID: conn.ClientID, ClientSecret: conn.ClientSecret, Endpoint: p.Endpoint(),
		RedirectURL: conn.RedirectURL, Scopes: append([]string{oidc.ScopeOpenID, "email"}, conn.Scopes...),
	}
}

// AuthURL returns where to send the browser: the provider's authorization endpoint with the
// state, the nonce and the PKCE challenge of verifier.
func (c *Client) AuthURL(ctx context.Context, conn Connection, state, nonce, verifier string) (string, error) {
	p, err := c.provider(ctx, conn.Issuer, false)
	if err != nil {
		return "", err
	}
	return c.oauth(p, conn).AuthCodeURL(state, oauth2.S256ChallengeOption(verifier), oidc.Nonce(nonce)), nil
}

// Exchange redeems an authorization code (with the PKCE verifier) and verifies the ID token.
func (c *Client) Exchange(ctx context.Context, conn Connection, code, nonce, verifier string) (Identity, error) {
	p, err := c.provider(ctx, conn.Issuer, false)
	if err != nil {
		return Identity{}, err
	}
	ctx = oidc.ClientContext(context.WithValue(ctx, oauth2.HTTPClient, c.http), c.http)
	tok, err := c.oauth(p, conn).Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return Identity{}, fmt.Errorf("%w: code exchange: %w", ErrRejected, err)
	}
	raw, ok := tok.Extra("id_token").(string)
	if !ok || raw == "" {
		return Identity{}, fmt.Errorf("%w: no ID token", ErrRejected)
	}
	idt, err := p.Verifier(&oidc.Config{ClientID: conn.ClientID}).Verify(ctx, raw)
	if err != nil {
		return Identity{}, fmt.Errorf("%w: %w", ErrRejected, err)
	}
	if idt.Nonce != nonce {
		return Identity{}, fmt.Errorf("%w: nonce mismatch", ErrRejected)
	}
	var claims struct {
		Email         string `json:"email"`
		EmailVerified any    `json:"email_verified"`
		Name          string `json:"name"`
	}
	if err := idt.Claims(&claims); err != nil {
		return Identity{}, fmt.Errorf("%w: claims: %w", ErrRejected, err)
	}
	return Identity{
		Subject: idt.Subject, Email: strings.ToLower(strings.TrimSpace(claims.Email)),
		EmailVerified: claims.EmailVerified == true || claims.EmailVerified == "true",
		Name:          strings.TrimSpace(claims.Name),
	}, nil
}
