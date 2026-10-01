// Package mail sends transactional email (invites) over SMTP, to any provider or a local
// catcher such as Mailpit. Messages are plain text: nothing in them is rendered as HTML.
package mail

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

// Message is one plain-text email to one recipient.
type Message struct {
	To      string
	Subject string
	Text    string
}

// Sender delivers messages.
type Sender interface {
	Send(ctx context.Context, m Message) error
}

// TLS modes.
const (
	TLSStartTLS = "starttls" // plain connection upgraded with STARTTLS (port 587); required
	TLSImplicit = "tls"      // TLS from the first byte (port 465)
	TLSNone     = "none"     // no encryption: local catchers only (configuration refuses it in production)
)

// Config configures the SMTP sender.
type Config struct {
	Host     string
	Port     int
	Username string
	Password string
	// From is the sender, e.g. "RiskOS <no-reply@example.com>".
	From    string
	TLS     string
	Timeout time.Duration
}

// SMTP sends through an SMTP server.
type SMTP struct {
	cfg  Config
	from *mail.Address
}

// NewSMTP returns a sender. It fails if From is not a valid address.
func NewSMTP(cfg Config) (*SMTP, error) {
	from, err := mail.ParseAddress(cfg.From)
	if err != nil {
		return nil, fmt.Errorf("mail: invalid sender %q: %w", cfg.From, err)
	}
	return &SMTP{cfg: cfg, from: from}, nil
}

// Send delivers m, bounded by the configured timeout and ctx.
func (s *SMTP) Send(ctx context.Context, m Message) error {
	to, err := mail.ParseAddress(m.To)
	if err != nil {
		return fmt.Errorf("mail: invalid recipient: %w", err)
	}
	body, err := Compose(s.from, to, m, time.Now())
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, s.cfg.Timeout)
	defer cancel()
	deadline, _ := ctx.Deadline()

	addr := net.JoinHostPort(s.cfg.Host, strconv.Itoa(s.cfg.Port))
	tlsCfg := &tls.Config{ServerName: s.cfg.Host, MinVersion: tls.VersionTLS12}
	var conn net.Conn
	if s.cfg.TLS == TLSImplicit {
		conn, err = (&tls.Dialer{Config: tlsCfg}).DialContext(ctx, "tcp", addr)
	} else {
		conn, err = (&net.Dialer{}).DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("mail: connect: %w", err)
	}
	if err := conn.SetDeadline(deadline); err != nil {
		_ = conn.Close()
		return fmt.Errorf("mail: %w", err)
	}
	c, err := smtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("mail: greeting: %w", err)
	}
	defer func() { _ = c.Close() }()
	if s.cfg.TLS == TLSStartTLS {
		if err := c.StartTLS(tlsCfg); err != nil {
			return fmt.Errorf("mail: starttls: %w", err)
		}
	}
	if s.cfg.Username != "" {
		// PlainAuth refuses to send credentials over an unencrypted connection to another host.
		if err := c.Auth(smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)); err != nil {
			return fmt.Errorf("mail: authenticate: %w", err)
		}
	}
	if err := c.Mail(s.from.Address); err != nil {
		return fmt.Errorf("mail: sender refused: %w", err)
	}
	if err := c.Rcpt(to.Address); err != nil {
		return fmt.Errorf("mail: recipient refused: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("mail: data: %w", err)
	}
	if _, err := w.Write(body); err != nil {
		return fmt.Errorf("mail: write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("mail: message refused: %w", err)
	}
	return c.Quit()
}

// Compose renders m as an RFC 5322 message. Header values are encoded, so a subject cannot
// inject headers; the body is quoted-printable-free plain UTF-8 with CRLF line endings.
func Compose(from, to *mail.Address, m Message, now time.Time) ([]byte, error) {
	if strings.ContainsAny(m.Subject, "\r\n") {
		return nil, errors.New("mail: subject contains a line break")
	}
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return nil, fmt.Errorf("mail: message id: %w", err)
	}
	domain := from.Address[strings.LastIndex(from.Address, "@")+1:]
	var b strings.Builder
	header := func(k, v string) { b.WriteString(k + ": " + v + "\r\n") }
	header("From", from.String())
	header("To", to.String())
	header("Subject", mime.QEncoding.Encode("utf-8", m.Subject))
	header("Date", now.UTC().Format(time.RFC1123Z))
	header("Message-ID", "<"+hex.EncodeToString(id)+"@"+domain+">")
	header("MIME-Version", "1.0")
	header("Content-Type", `text/plain; charset="utf-8"`)
	header("Content-Transfer-Encoding", "8bit")
	header("Auto-Submitted", "auto-generated")
	b.WriteString("\r\n")
	text := strings.ReplaceAll(strings.ReplaceAll(m.Text, "\r\n", "\n"), "\n", "\r\n")
	b.WriteString(text)
	if !strings.HasSuffix(text, "\r\n") {
		b.WriteString("\r\n")
	}
	return []byte(b.String()), nil
}
