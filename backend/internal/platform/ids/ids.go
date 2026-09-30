// Package ids generates opaque, prefixed identifiers.
package ids

import "crypto/rand"

// alphabet is lowercase Crockford base32: no padding and no ambiguous characters (i, l, o, u),
// so IDs are URL-, header- and log-safe.
const alphabet = "0123456789abcdefghjkmnpqrstvwxyz"

// encodedLen is the base32 length of 128 bits.
const encodedLen = 26

// New returns prefix + "_" + 26 characters encoding 128 bits of cryptographic randomness.
// crypto/rand.Read never returns an error (it aborts the program on catastrophic failure), so
// the signature stays error-free.
func New(prefix string) string {
	var b [16]byte
	_, _ = rand.Read(b[:])

	out := make([]byte, 0, len(prefix)+1+encodedLen)
	out = append(out, prefix...)
	out = append(out, '_')
	var acc uint32
	bits := 0
	for _, v := range b {
		acc = acc<<8 | uint32(v)
		bits += 8
		for bits >= 5 {
			bits -= 5
			out = append(out, alphabet[(acc>>bits)&31])
		}
	}
	if bits > 0 {
		out = append(out, alphabet[(acc<<(5-bits))&31])
	}
	return string(out)
}

// NewRequestID returns a new request correlation ID ("req_…").
func NewRequestID() string { return New("req") }

// IsSafeExternalID reports whether a caller-supplied correlation ID may be echoed and logged:
// 1–64 characters drawn from [A-Za-z0-9._-]. This prevents log injection and header abuse.
func IsSafeExternalID(s string) bool {
	if len(s) == 0 || len(s) > 64 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '.', c == '_', c == '-':
		default:
			return false
		}
	}
	return true
}
