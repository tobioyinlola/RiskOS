package mail_test

import (
	"mime"
	"net/mail"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	riskmail "github.com/tobioyinlola/RiskOS/backend/internal/platform/mail"
)

func TestCompose(t *testing.T) {
	from := &mail.Address{Name: "RiskOS", Address: "no-reply@riskos.test"}
	to := &mail.Address{Address: "ada@example.com"}
	body, err := riskmail.Compose(from, to, riskmail.Message{
		Subject: "Join Acme Payments — on RiskOS",
		Text:    "Hello\nOpen this link:\nhttps://console.test/invite#abc",
	}, time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	s := string(body)
	assert.Contains(t, s, "From: \"RiskOS\" <no-reply@riskos.test>\r\n")
	assert.Contains(t, s, "To: <ada@example.com>\r\n")
	assert.Contains(t, s, "Subject: =?utf-8?q?")
	assert.Contains(t, s, "Content-Type: text/plain; charset=\"utf-8\"\r\n")
	assert.Contains(t, s, "@riskos.test>\r\n")
	assert.True(t, strings.HasSuffix(s, "\r\nOpen this link:\r\nhttps://console.test/invite#abc\r\n"))

	parsed, err := mail.ReadMessage(strings.NewReader(s))
	require.NoError(t, err)
	dec, err := new(mime.WordDecoder).DecodeHeader(parsed.Header.Get("Subject"))
	require.NoError(t, err)
	assert.Equal(t, "Join Acme Payments — on RiskOS", dec)
}

func TestComposeRefusesHeaderInjection(t *testing.T) {
	from := &mail.Address{Address: "no-reply@riskos.test"}
	_, err := riskmail.Compose(from, &mail.Address{Address: "a@example.com"},
		riskmail.Message{Subject: "Hi\r\nBcc: victim@example.com", Text: "x"}, time.Now())
	assert.Error(t, err)
}

func TestNewSMTPValidatesSender(t *testing.T) {
	_, err := riskmail.NewSMTP(riskmail.Config{From: "not an address"})
	assert.Error(t, err)
	_, err = riskmail.NewSMTP(riskmail.Config{From: "RiskOS <no-reply@riskos.test>"})
	assert.NoError(t, err)
}
