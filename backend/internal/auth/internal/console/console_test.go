package console_test

import (
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/auth/internal/console"
)

func TestPasswords(t *testing.T) {
	h, err := console.HashPassword("correct horse battery staple")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(h, "$argon2id$v=19$m=65536,t=3,p=2$"))
	ok, err := console.VerifyPassword("correct horse battery staple", h)
	require.NoError(t, err)
	assert.True(t, ok)
	ok, _ = console.VerifyPassword("correct horse battery stapl", h)
	assert.False(t, ok)
	h2, _ := console.HashPassword("correct horse battery staple")
	assert.NotEqual(t, h, h2, "salted")
	_, err = console.VerifyPassword("x", "$bcrypt$nope")
	assert.ErrorIs(t, err, console.ErrBadHash)

	assert.ErrorIs(t, console.ValidatePassword("short", "a@b.co"), console.ErrWeakPassword)
	assert.ErrorIs(t, console.ValidatePassword("analyst@bank.example", "analyst@bank.example"), console.ErrWeakPassword)
	assert.ErrorIs(t, console.ValidatePassword(strings.Repeat("x", 129), "a@b.co"), console.ErrWeakPassword)
	assert.NoError(t, console.ValidatePassword("a long enough passphrase", "a@b.co"))
}

// RFC 6238 Appendix B test vectors for HMAC-SHA1 (8 digits there; the last 6 here).
func TestTOTPVectors(t *testing.T) {
	secret := []byte("12345678901234567890")
	for unix, want8 := range map[int64]string{59: "94287082", 1111111109: "07081804", 1234567890: "89005924", 2000000000: "69279037"} {
		assert.Equal(t, want8[2:], console.TOTPCode(secret, console.TOTPStep(time.Unix(unix, 0))), "t=%d", unix)
	}
}

func TestVerifyTOTP(t *testing.T) {
	secret, err := console.NewTOTPSecret()
	require.NoError(t, err)
	now := time.Unix(1_790_000_000, 0)
	code := console.TOTPCode(secret, console.TOTPStep(now))
	step, ok := console.VerifyTOTP(secret, code, now, 0)
	require.True(t, ok)
	_, ok = console.VerifyTOTP(secret, code, now, step)
	assert.False(t, ok, "a code is accepted once")
	prev := console.TOTPCode(secret, console.TOTPStep(now)-1)
	_, ok = console.VerifyTOTP(secret, prev, now, 0)
	assert.True(t, ok, "30 s of clock skew")
	old := console.TOTPCode(secret, console.TOTPStep(now)-3)
	_, ok = console.VerifyTOTP(secret, old, now, 0)
	assert.False(t, ok)
	_, ok = console.VerifyTOTP(secret, "12345", now, 0)
	assert.False(t, ok)
	assert.Contains(t, console.OTPAuthURI(secret, "a@b.co"), "otpauth://totp/RiskOS:a@b.co?")
}

func TestSealer(t *testing.T) {
	s, err := console.NewSealer([]byte(strings.Repeat("k", 32)))
	require.NoError(t, err)
	ct, err := s.Seal([]byte("secret"), []byte("usr_1"))
	require.NoError(t, err)
	pt, err := s.Open(ct, []byte("usr_1"))
	require.NoError(t, err)
	assert.Equal(t, "secret", string(pt))
	_, err = s.Open(ct, []byte("usr_2"))
	assert.Error(t, err, "bound to the user")
	_, err = console.NewSealer([]byte("short"))
	assert.Error(t, err)
}

func TestTokensAndRecoveryCodes(t *testing.T) {
	tok, hash, err := console.NewToken()
	require.NoError(t, err)
	assert.Len(t, tok, 43)
	assert.True(t, console.EqualToken(tok, hash))
	assert.False(t, console.EqualToken(tok+"x", hash))
	assert.Equal(t, 64, len(hex.EncodeToString(hash)))

	codes, hashes, err := console.NewRecoveryCodes()
	require.NoError(t, err)
	require.Len(t, codes, 10)
	assert.Regexp(t, `^[a-z2-7]{4}-[a-z2-7]{4}-[a-z2-7]{4}-[a-z2-7]{4}$`, codes[0])
	assert.Equal(t, hashes[0], console.HashRecoveryCode(strings.ToUpper(strings.ReplaceAll(codes[0], "-", " "))), "normalized")
}
