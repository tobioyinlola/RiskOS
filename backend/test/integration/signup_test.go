//go:build integration

package integration

import (
	"net/http"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
	"github.com/tobioyinlola/RiskOS/backend/internal/auth"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/tenancy"
)

var codePattern = regexp.MustCompile(`\b(\d{6})\b`)

// signupMail runs the queued sign-up email jobs through the auth module's handler and returns
// what was sent, newest last.
func signupMail(t *testing.T, s stack, q *jobRecorder) []string {
	t.Helper()
	box := &mailbox{}
	q.mu.Lock()
	queued := append([]any(nil), anySlice(q.jobs)...)
	q.mu.Unlock()
	job := s.auth.SignupEmailJob(box)
	for _, j := range queued {
		if a, ok := j.(auth.SignupEmailArgs); ok {
			require.NoError(t, job(t.Context(), a))
		}
	}
	out := make([]string, len(box.sent))
	for i, m := range box.sent {
		out[i] = m.Subject + "\n" + m.Text
	}
	return out
}

func anySlice[T any](in []T) []any {
	out := make([]any, len(in))
	for i, v := range in {
		out[i] = v
	}
	return out
}

func lastCode(t *testing.T, mails []string) string {
	t.Helper()
	require.NotEmpty(t, mails)
	m := codePattern.FindStringSubmatch(mails[len(mails)-1])
	require.NotNil(t, m, "the latest email carries a code: %s", mails[len(mails)-1])
	return m[1]
}

// enrollTOTP finishes a fresh sign-up's mandatory second factor and returns the session.
func enrollTOTP(t *testing.T, b *browser) apigen.ConsoleSession {
	t.Helper()
	r := b.do(t, http.MethodPost, "/api/v1/auth/mfa/totp/enroll", nil)
	require.Equal(t, http.StatusOK, r.status, r.String())
	enroll := decode[apigen.TotpEnrollment](t, r)
	r = b.do(t, http.MethodPost, "/api/v1/auth/mfa/totp/confirm", map[string]any{"code": totp(t, enroll.Secret, time.Now())})
	require.Equal(t, http.StatusOK, r.status, r.String())
	return b.refresh(t)
}

func completeAnswers() map[string]any {
	return map[string]any{
		"legal_name": "Acme Payments Ltd", "country": "NG", "registration_number": "RC123456",
		"website": "https://acme.example", "industry": "payments", "regulated": true, "regulator": "Central Bank",
		"licence_number": "PSP-0042", "aml_program": true, "compliance_officer_name": "Grace Hopper",
		"compliance_officer_email": "grace@acme.example", "monitored_events": []string{"payments", "logins"},
		"monthly_volume": "100k_1m", "regions_served": []string{"NG", "GH"}, "go_live": "within_3_months",
		"data_protection": []string{"ndpr"}, "data_residency": "africa", "accept_terms": true, "accept_dpa": true,
	}
}

// TestSelfServeSignup covers sign-up with email end to end: the emailed code, the workspace
// with locked production, onboarding, and production opening on approval.
func TestSelfServeSignup(t *testing.T) {
	q := &jobRecorder{}
	s := newStackWith(t, time.Second, generousLimits(), shared.rdb, withInviteEmails(q), withSignup(nil))
	b := s.browser()

	r := b.do(t, http.MethodGet, "/api/v1/auth/signup/options", nil)
	require.Equal(t, http.StatusOK, r.status, r.String())
	assert.Equal(t, apigen.SignupOptions{Email: true, Google: false}, decode[apigen.SignupOptions](t, r))

	email := "founder-" + time.Now().Format("150405.000000") + "@acme.example"
	r = b.do(t, http.MethodPost, "/api/v1/auth/signup", map[string]any{
		"name": "Ada Lovelace", "email": email, "business_name": "Acme Payments", "password": testPassword,
	})
	require.Equal(t, http.StatusAccepted, r.status, r.String())
	require.NotEmpty(t, b.signup, "the sign-up is tied to this browser")
	code := lastCode(t, signupMail(t, s, q))

	// Nothing exists yet; a wrong code is refused, and a cookie-less browser gets nowhere.
	wrong := "000000"
	if code == wrong {
		wrong = "111111"
	}
	r = b.do(t, http.MethodPost, "/api/v1/auth/signup/verify", map[string]any{"code": wrong})
	require.Equal(t, http.StatusBadRequest, r.status, r.String())
	assert.Contains(t, r.String(), "CODE_INVALID")
	r = s.browser().do(t, http.MethodPost, "/api/v1/auth/signup/verify", map[string]any{"code": code})
	require.Equal(t, http.StatusUnauthorized, r.status, r.String())

	r = b.do(t, http.MethodPost, "/api/v1/auth/signup/verify", map[string]any{"code": code})
	require.Equal(t, http.StatusOK, r.status, r.String())
	assert.Equal(t, apigen.SessionState("enrollment_required"), decode[apigen.AuthState](t, r).State)
	sess := enrollTOTP(t, b)
	assert.Equal(t, "Acme Payments", sess.Tenant.Name)
	assert.Equal(t, apigen.Role("customer_admin"), sess.User.Role)
	assert.False(t, sess.Tenant.ProductionEnabled)
	assert.Equal(t, []apigen.Environment{"test"}, sess.Environments)

	// The code was single use: the sign-up is finished.
	r = b.do(t, http.MethodPost, "/api/v1/auth/signup/verify", map[string]any{"code": code})
	require.Equal(t, http.StatusUnauthorized, r.status, r.String())

	// Production is locked until onboarding is approved.
	b.env = "production"
	r = b.do(t, http.MethodGet, "/api/v1/events", nil)
	require.Equal(t, http.StatusForbidden, r.status, r.String())
	assert.Contains(t, r.String(), "PRODUCTION_LOCKED")
	b.env = "test"

	r = b.do(t, http.MethodGet, "/api/v1/onboarding", nil)
	require.Equal(t, http.StatusOK, r.status, r.String())
	ob := decode[apigen.Onboarding](t, r)
	assert.Equal(t, apigen.OnboardingStatus("draft"), ob.Status)
	assert.Contains(t, ob.Missing, "legal_name")

	r = b.do(t, http.MethodPost, "/api/v1/onboarding/submit", nil)
	require.Equal(t, http.StatusBadRequest, r.status, "an incomplete onboarding cannot be submitted: %s", r.String())
	r = b.do(t, http.MethodPut, "/api/v1/onboarding", map[string]any{"country": "Nigeria"})
	require.Equal(t, http.StatusBadRequest, r.status, r.String())
	r = b.do(t, http.MethodPut, "/api/v1/onboarding", completeAnswers())
	require.Equal(t, http.StatusOK, r.status, r.String())
	assert.Empty(t, decode[apigen.Onboarding](t, r).Missing)
	r = b.do(t, http.MethodPost, "/api/v1/onboarding/submit", nil)
	require.Equal(t, http.StatusOK, r.status, r.String())
	assert.Equal(t, apigen.OnboardingStatus("submitted"), decode[apigen.Onboarding](t, r).Status)
	r = b.do(t, http.MethodPut, "/api/v1/onboarding", completeAnswers())
	require.Equal(t, http.StatusConflict, r.status, "a submitted onboarding is frozen: %s", r.String())

	// Staff approve (the admin realm calls the tenants module in a staff tenant scope).
	tenantID, err := ids.Parse(ids.PrefixTenant, sess.Tenant.Id)
	require.NoError(t, err)
	staffCtx, err := s.tenants.StaffScope(t.Context(), tenantID, tenancy.EnvironmentTest, "stf_integration", nil)
	require.NoError(t, err)
	_, err = s.tenants.ReviewOnboarding(staffCtx, false, "short", time.Now())
	require.Error(t, err, "a rejection needs a real note")
	approved, err := s.tenants.ReviewOnboarding(staffCtx, true, "", time.Now())
	require.NoError(t, err)
	assert.Equal(t, "approved", approved.Status)
	assert.True(t, approved.ProductionEnabled)

	sess = b.refresh(t)
	assert.True(t, sess.Tenant.ProductionEnabled)
	assert.Equal(t, []apigen.Environment{"test", "production"}, sess.Environments)
	b.env = "production"
	r = b.do(t, http.MethodGet, "/api/v1/events", nil)
	require.Equal(t, http.StatusOK, r.status, r.String())

	// Signing up again with the same email reveals nothing and creates nothing: the email says the
	// account exists, and no code works.
	again := s.browser()
	r = again.do(t, http.MethodPost, "/api/v1/auth/signup", map[string]any{
		"name": "Someone", "email": email, "business_name": "Other Co", "password": testPassword,
	})
	require.Equal(t, http.StatusAccepted, r.status, r.String())
	mails := signupMail(t, s, q)
	assert.Contains(t, mails[len(mails)-1], "already has an account")
	assert.Nil(t, codePattern.FindStringSubmatch(mails[len(mails)-1]))
}

// TestGoogleSignup covers signing up with Google (no RiskOS password, a business name after
// returning) and signing in with Google afterwards, which still needs the second factor.
func TestGoogleSignup(t *testing.T) {
	idp := newMockIDP(t)
	google := &auth.GoogleConfig{ClientID: "riskos-test", ClientSecret: "secret", Issuer: idp.srv.URL}
	s := newStackWith(t, time.Second, generousLimits(), shared.rdb,
		withConsole(auth.ConsoleConfig{SSOAllowPrivate: true}, false), withSignup(google))
	email := "google-" + time.Now().Format("150405.000000") + "@acme.example"
	claims := map[string]any{"sub": "g-" + email, "email": email, "email_verified": true, "name": "Grace Hopper"}

	viaGoogle := func(b *browser, claims map[string]any) apigen.GoogleResult {
		t.Helper()
		r := b.do(t, http.MethodPost, "/api/v1/auth/google/start", nil)
		require.Equal(t, http.StatusOK, r.status, r.String())
		code, state := idp.authorizeAt(t, decode[apigen.SSOStart](t, r).RedirectUrl, "http://console.test/signup/google", claims)
		r = b.do(t, http.MethodPost, "/api/v1/auth/google/callback", map[string]any{"code": code, "state": state})
		require.Equal(t, http.StatusOK, r.status, r.String())
		return decode[apigen.GoogleResult](t, r)
	}

	r := s.browser().do(t, http.MethodGet, "/api/v1/auth/signup/options", nil)
	assert.Equal(t, apigen.SignupOptions{Email: false, Google: true}, decode[apigen.SignupOptions](t, r))

	// An unverified Google email is refused.
	unverified := map[string]any{"sub": "g-x", "email": "x@acme.example", "email_verified": false}
	b := s.browser()
	r = b.do(t, http.MethodPost, "/api/v1/auth/google/start", nil)
	code, state := idp.authorizeAt(t, decode[apigen.SSOStart](t, r).RedirectUrl, "http://console.test/signup/google", unverified)
	r = b.do(t, http.MethodPost, "/api/v1/auth/google/callback", map[string]any{"code": code, "state": state})
	require.Equal(t, http.StatusUnauthorized, r.status, r.String())

	b = s.browser()
	res := viaGoogle(b, claims)
	require.Equal(t, apigen.NeedsBusiness, res.Outcome)
	assert.Equal(t, "Grace Hopper", *res.Name)
	r = b.do(t, http.MethodPost, "/api/v1/auth/signup/complete", map[string]any{"business_name": "Hopper Lending"})
	require.Equal(t, http.StatusOK, r.status, r.String())
	sess := enrollTOTP(t, b)
	assert.Equal(t, "Hopper Lending", sess.Tenant.Name)
	assert.Equal(t, "Grace Hopper", sess.User.Name)

	// Password sign-in does not work for a Google-only account; Google does, with the second factor.
	r = s.browser().do(t, http.MethodPost, "/api/v1/auth/login", map[string]any{"email": email, "password": testPassword})
	require.Equal(t, http.StatusUnauthorized, r.status, r.String())
	again := viaGoogle(s.browser(), claims)
	require.Equal(t, apigen.SignedIn, again.Outcome)
	assert.Equal(t, apigen.SessionState("mfa_required"), again.Auth.State)

	// Another Google account claiming the same email cannot take the account.
	hijack := map[string]any{"sub": "g-other", "email": email, "email_verified": true}
	b = s.browser()
	r = b.do(t, http.MethodPost, "/api/v1/auth/google/start", nil)
	code, state = idp.authorizeAt(t, decode[apigen.SSOStart](t, r).RedirectUrl, "http://console.test/signup/google", hijack)
	r = b.do(t, http.MethodPost, "/api/v1/auth/google/callback", map[string]any{"code": code, "state": state})
	require.Equal(t, http.StatusUnauthorized, r.status, r.String())
}
