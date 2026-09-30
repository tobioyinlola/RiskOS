// Package console holds the dashboard realm's credential primitives: password hashing, TOTP,
// recovery codes, session and invite tokens, and secret encryption. It has no I/O.
package console

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1" //nolint:gosec // RFC 6238 TOTP uses HMAC-SHA1; authenticator apps require it
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

// Errors.
var (
	ErrWeakPassword = errors.New("password does not meet the policy")
	ErrBadHash      = errors.New("malformed password hash")
)

// Password policy (NIST SP 800-63B): length over composition rules.
const (
	MinPasswordLength = 12
	MaxPasswordLength = 128
)

// Argon2id parameters (RFC 9106 second recommended option, memory-constrained).
const (
	argonTime    = 3
	argonMemory  = 64 * 1024 // KiB
	argonThreads = 2
	argonKeyLen  = 32
	argonSaltLen = 16
)

// ValidatePassword checks the password policy.
func ValidatePassword(password, email string) error {
	n := utf8.RuneCountInString(password)
	switch {
	case n < MinPasswordLength:
		return fmt.Errorf("%w: at least %d characters", ErrWeakPassword, MinPasswordLength)
	case n > MaxPasswordLength:
		return fmt.Errorf("%w: at most %d characters", ErrWeakPassword, MaxPasswordLength)
	case strings.EqualFold(strings.TrimSpace(password), strings.TrimSpace(email)):
		return fmt.Errorf("%w: must not be your email address", ErrWeakPassword)
	}
	return nil
}

// HashPassword returns an Argon2id hash in PHC string format.
func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

// VerifyPassword reports whether password matches a hash from HashPassword, in constant time.
func VerifyPassword(password, encoded string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, ErrBadHash
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false, ErrBadHash
	}
	var memory, iterations uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &threads); err != nil {
		return false, ErrBadHash
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, ErrBadHash
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, ErrBadHash
	}
	got := argon2.IDKey([]byte(password), salt, iterations, memory, threads, uint32(len(want))) //nolint:gosec // key length is 32
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

// dummyHash is verified against when an email is unknown, so sign-in takes the same time whether
// or not the account exists.
var dummyHash = func() string { //nolint:gochecknoglobals // computed once
	h, err := HashPassword("riskos-timing-equalizer-password")
	if err != nil {
		panic(err)
	}
	return h
}()

// VerifyDummy spends the same work as VerifyPassword and always fails.
func VerifyDummy(password string) {
	_, _ = VerifyPassword(password, dummyHash)
}

// NewToken returns a 256-bit random token and its SHA-256 hash for storage. The token is hex:
// invite links travel through chat, email and Markdown, which mangle `_` and `-`.
func NewToken() (token string, hash []byte, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", nil, fmt.Errorf("generate token: %w", err)
	}
	token = hex.EncodeToString(b)
	return token, HashToken(token), nil
}

// HashToken hashes a session, CSRF, invite or recovery token for storage and lookup.
func HashToken(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}

// EqualToken compares a presented token with a stored hash in constant time.
func EqualToken(token string, hash []byte) bool {
	return hmac.Equal(HashToken(token), hash)
}

// Sealer encrypts TOTP secrets at rest with AES-256-GCM under a server-side key.
type Sealer struct{ aead cipher.AEAD }

// NewSealer derives the AES key from key (at least 32 bytes).
func NewSealer(key []byte) (*Sealer, error) {
	if len(key) < 32 {
		return nil, errors.New("encryption key must be at least 32 bytes")
	}
	k := sha256.Sum256(append([]byte("riskos-mfa-secret\x00"), key...))
	block, err := aes.NewCipher(k[:])
	if err != nil {
		return nil, fmt.Errorf("aes: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("gcm: %w", err)
	}
	return &Sealer{aead: aead}, nil
}

// Seal encrypts plaintext, binding it to aad (the user ID) so a ciphertext cannot be moved to
// another user.
func (s *Sealer) Seal(plaintext, aad []byte) ([]byte, error) {
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}
	return s.aead.Seal(nonce, nonce, plaintext, aad), nil
}

// Open decrypts a Seal output.
func (s *Sealer) Open(ciphertext, aad []byte) ([]byte, error) {
	n := s.aead.NonceSize()
	if len(ciphertext) < n {
		return nil, errors.New("ciphertext too short")
	}
	out, err := s.aead.Open(nil, ciphertext[:n], ciphertext[n:], aad)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}
	return out, nil
}

// TOTP (RFC 6238): HMAC-SHA1, 6 digits, 30-second steps.
const (
	totpStep   = 30
	totpDigits = 6
	// totpSkew accepts the previous and next step, tolerating clock drift of about 30 seconds.
	totpSkew = 1
)

// NewTOTPSecret returns a 160-bit secret.
func NewTOTPSecret() ([]byte, error) {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("generate totp secret: %w", err)
	}
	return b, nil
}

// EncodeSecret is the base32 form authenticator apps take.
func EncodeSecret(secret []byte) string {
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secret)
}

// OTPAuthURI is the enrollment URI shown as a QR code.
func OTPAuthURI(secret []byte, account string) string {
	v := url.Values{}
	v.Set("secret", EncodeSecret(secret))
	v.Set("issuer", "RiskOS")
	v.Set("algorithm", "SHA1")
	v.Set("digits", strconv.Itoa(totpDigits))
	v.Set("period", strconv.Itoa(totpStep))
	return "otpauth://totp/" + url.PathEscape("RiskOS:"+account) + "?" + v.Encode()
}

// TOTPCode returns the code of a time step.
func TOTPCode(secret []byte, step int64) string {
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(step)) //nolint:gosec // steps are positive
	m := hmac.New(sha1.New, secret)
	m.Write(msg[:])
	sum := m.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	v := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", v%1_000_000)
}

// TOTPStep returns the time step of t.
func TOTPStep(t time.Time) int64 { return t.Unix() / totpStep }

// VerifyTOTP checks code against the steps around now and returns the matching step. Steps at
// or before lastStep are rejected, so a code is accepted at most once.
func VerifyTOTP(secret []byte, code string, now time.Time, lastStep int64) (int64, bool) {
	code = strings.ReplaceAll(strings.TrimSpace(code), " ", "")
	if len(code) != totpDigits {
		return 0, false
	}
	cur := TOTPStep(now)
	for s := cur - totpSkew; s <= cur+totpSkew; s++ {
		if s <= lastStep {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(TOTPCode(secret, s)), []byte(code)) == 1 {
			return s, true
		}
	}
	return 0, false
}

// RecoveryCodeCount is how many recovery codes a user gets.
const RecoveryCodeCount = 10

// NewRecoveryCodes returns codes (shown once) and their hashes (stored).
func NewRecoveryCodes() (codes []string, hashes [][]byte, err error) {
	enc := base32.StdEncoding.WithPadding(base32.NoPadding)
	for range RecoveryCodeCount {
		b := make([]byte, 10)
		if _, err := rand.Read(b); err != nil {
			return nil, nil, fmt.Errorf("generate recovery code: %w", err)
		}
		s := strings.ToLower(enc.EncodeToString(b)) // 16 characters, 80 bits
		code := s[0:4] + "-" + s[4:8] + "-" + s[8:12] + "-" + s[12:16]
		codes = append(codes, code)
		hashes = append(hashes, HashRecoveryCode(code))
	}
	return codes, hashes, nil
}

// HashRecoveryCode normalizes (case, dashes, spaces) and hashes a recovery code.
func HashRecoveryCode(code string) []byte {
	norm := strings.ToLower(strings.NewReplacer("-", "", " ", "").Replace(strings.TrimSpace(code)))
	return HashToken("recovery:" + norm)
}
