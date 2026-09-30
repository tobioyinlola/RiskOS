// Package logging builds the process logger: JSON via log/slog, request-ID correlation from
// context, and redaction of sensitive data before anything reaches the output.
//
// Redaction is defense in depth, not permission to log secrets. Code must still avoid logging
// credentials, payload bodies and personal data; see docs/SECURITY.md.
package logging

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/tobioyinlola/RiskOS/backend/internal/platform/requestctx"
)

// Redacted replaces sensitive values in log output.
const Redacted = "[REDACTED]"

// sensitiveKeyParts are matched case-insensitively as substrings of attribute keys, after
// removing '-' and '_' (so "X-API-Key", "api_key" and "apiKey" all match "apikey").
func sensitiveKeyParts() [10]string {
	return [...]string{
		"authorization", "apikey", "secret", "password", "passwd", "token", "cookie", "credential",
		"privatekey", "signature",
	}
}

// New returns a JSON logger at level writing to w, with request-ID enrichment and redaction.
func New(w io.Writer, level slog.Leveler, service string) *slog.Logger {
	h := slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level:       level,
		ReplaceAttr: redactAttr,
	})
	return slog.New(contextHandler{Handler: h}).With(slog.String("service", service))
}

// contextHandler adds request-scoped correlation attributes from the record's context.
type contextHandler struct{ slog.Handler }

func (h contextHandler) Handle(ctx context.Context, r slog.Record) error {
	if id := requestctx.RequestID(ctx); id != "" {
		r.AddAttrs(slog.String("request_id", id))
	}
	r.Message = redactString(r.Message)
	return h.Handler.Handle(ctx, r)
}

func (h contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return contextHandler{Handler: h.Handler.WithAttrs(attrs)}
}

func (h contextHandler) WithGroup(name string) slog.Handler {
	return contextHandler{Handler: h.Handler.WithGroup(name)}
}

// redactAttr is a slog ReplaceAttr hook. It is applied to every attribute, including those
// nested inside groups, so a sensitive key is redacted wherever it appears.
func redactAttr(_ []string, a slog.Attr) slog.Attr {
	if IsSensitiveKey(a.Key) {
		return slog.String(a.Key, Redacted)
	}
	switch a.Value.Kind() {
	case slog.KindString:
		if s := a.Value.String(); s != "" {
			if r := redactString(s); r != s {
				return slog.String(a.Key, r)
			}
		}
	case slog.KindAny:
		if h, ok := a.Value.Any().(http.Header); ok {
			return slog.Any(a.Key, redactHeader(h))
		}
	}
	return a
}

// redactHeader returns a copy of h with sensitive header values replaced.
func redactHeader(h http.Header) http.Header {
	out := make(http.Header, len(h))
	for k, vs := range h {
		if IsSensitiveKey(k) {
			out[k] = []string{Redacted}
			continue
		}
		cp := make([]string, len(vs))
		for i, v := range vs {
			cp[i] = redactString(v)
		}
		out[k] = cp
	}
	return out
}

// IsSensitiveKey reports whether an attribute or header name denotes a secret.
func IsSensitiveKey(key string) bool {
	var buf [64]byte
	k := buf[:0]
	for i := 0; i < len(key); i++ {
		c := key[i]
		if c == '-' || c == '_' {
			continue
		}
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		k = append(k, c)
	}
	normalized := string(k)
	for _, part := range sensitiveKeyParts() {
		if strings.Contains(normalized, part) {
			return true
		}
	}
	return false
}

// credentialSchemes are HTTP auth schemes whose following token is redacted inside free text,
// e.g. an error message that embeds "Authorization: Bearer abc".
func credentialSchemes() [2]string { return [...]string{"bearer ", "basic "} }

// redactString replaces the token following a credential scheme with Redacted. It runs on every
// logged string, so the common case (no scheme present) neither allocates nor copies.
func redactString(s string) string {
	var found bool
	for _, scheme := range credentialSchemes() {
		if indexFold(s, scheme) >= 0 {
			found = true
			break
		}
	}
	if !found {
		return s
	}

	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		next, schemeLen := -1, 0
		for _, scheme := range credentialSchemes() {
			if j := indexFold(s[i:], scheme); j >= 0 && (next == -1 || i+j < next) {
				next, schemeLen = i+j, len(scheme)
			}
		}
		if next == -1 {
			b.WriteString(s[i:])
			break
		}
		tokenStart := next + schemeLen
		b.WriteString(s[i:tokenStart])
		tokenEnd := tokenStart
		for tokenEnd < len(s) && !isTokenBoundary(s[tokenEnd]) {
			tokenEnd++
		}
		if tokenEnd > tokenStart {
			b.WriteString(Redacted)
		}
		i = tokenEnd
	}
	return b.String()
}

// indexFold returns the index of the first ASCII case-insensitive occurrence of the lowercase
// needle in s, or -1. Unlike strings.ToLower + strings.Index it does not allocate.
func indexFold(s, needle string) int {
	n := len(needle)
outer:
	for i := 0; i+n <= len(s); i++ {
		for j := 0; j < n; j++ {
			c := s[i+j]
			if c >= 'A' && c <= 'Z' {
				c += 'a' - 'A'
			}
			if c != needle[j] {
				continue outer
			}
		}
		return i
	}
	return -1
}

func isTokenBoundary(c byte) bool {
	switch c {
	case ' ', '\t', '\n', '\r', '"', '\'', ',', ';':
		return true
	}
	return false
}
