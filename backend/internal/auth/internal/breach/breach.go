// Package breach checks whether a password appears in known data breaches, using the Pwned
// Passwords range API (Have I Been Pwned) with k-anonymity: only the first five hexadecimal
// characters of the password's SHA-1 hash leave RiskOS, and responses are padded, so neither
// the password nor whether it was found can be learned from the traffic.
package breach

import (
	"bufio"
	"context"
	"crypto/sha1" //nolint:gosec // the range API is keyed by SHA-1; this is lookup, not protection
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DefaultURL is the Pwned Passwords API.
const DefaultURL = "https://api.pwnedpasswords.com"

// maxBody bounds a range response (padded responses are about 30-40 KB).
const maxBody = 1 << 20

// Checker queries the range API.
type Checker struct {
	base   string
	client *http.Client
}

// New returns a checker for the range API at base (DefaultURL in production).
func New(base string, timeout time.Duration) *Checker {
	return &Checker{base: strings.TrimRight(base, "/"), client: &http.Client{Timeout: timeout}}
}

// Breached reports whether password appears in the breach corpus.
func (c *Checker) Breached(ctx context.Context, password string) (bool, error) {
	sum := sha1.Sum([]byte(password)) //nolint:gosec // see the import
	digest := strings.ToUpper(hex.EncodeToString(sum[:]))
	prefix, suffix := digest[:5], digest[5:]

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/range/"+prefix, nil)
	if err != nil {
		return false, fmt.Errorf("breach check: %w", err)
	}
	req.Header.Set("Add-Padding", "true")
	req.Header.Set("User-Agent", "RiskOS")
	res, err := c.client.Do(req)
	if err != nil {
		return false, fmt.Errorf("breach check: %w", err)
	}
	defer res.Body.Close() //nolint:errcheck // read-only body
	if res.StatusCode != http.StatusOK {
		return false, fmt.Errorf("breach check: status %d", res.StatusCode)
	}
	sc := bufio.NewScanner(io.LimitReader(res.Body, maxBody))
	for sc.Scan() {
		hash, count, ok := strings.Cut(strings.TrimSpace(sc.Text()), ":")
		// Padding entries have a count of 0.
		if ok && strings.EqualFold(hash, suffix) && strings.TrimSpace(count) != "0" {
			return true, nil
		}
	}
	if err := sc.Err(); err != nil && !errors.Is(err, io.EOF) {
		return false, fmt.Errorf("breach check: %w", err)
	}
	return false, nil
}
