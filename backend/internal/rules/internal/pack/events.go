package pack

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"slices"
)

// How events reach rules (EVENTS.md#compliance): an event's API type, the pack event kinds it
// counts as, and the facts and references a customer may send with it.

// ErrSensitive means a reference looks like the personal data it stands for.
var ErrSensitive = errors.New("sensitive data")

// MoneyKinds are the money movements: they go with transaction events.
func MoneyKinds() []string {
	return []string{
		"transfer_outbound", "transfer_inbound", "transfer_international", "cash_payment", "cash_withdrawal",
		"cash_deposit", "atm_withdrawal", "pos_purchase", "card_authorization", "crypto_purchase",
		"crypto_withdrawal", "crypto_transfer", "fiat_inflow", "funding", "loan_disbursement", "loan_repayment",
	}
}

// KindsFor returns the kinds a customer may name for an event of API type t.
func KindsFor(t string) []string {
	switch t {
	case "transaction":
		return MoneyKinds()
	case "login":
		return []string{"login", "auth_failure", "auth_success"}
	case "device_change":
		return []string{"device_bound", "device_reregistered"}
	case "beneficiary_added":
		return []string{"beneficiary_added"}
	case "session_change":
		return []string{"session_activity"}
	case "profile_change":
		return []string{"profile_change"}
	case "activity":
		return []string{
			"loan_application", "mfa_prompt", "sim_reregistered", "account_opened", "account_reactivated",
			"customer_onboarded", "customer_review", "credit_requested", "list_updated", "telco_lookup",
			"data_export", "api_request", "api_lookup", "config_change", "role_change", "staff_action",
			"fraud_report", "merchant_daily", "terminal_activity",
		}
	}
	return nil
}

// KindInput is what kind derivation reads from an event.
type KindInput struct {
	// Type is the API event type; Kind the kind the customer named, if any.
	Type, Kind string
	// Direction and Channel of a transaction; Outcome of a login.
	Direction, Channel, Outcome string
}

// Kinds returns the pack event kinds an event counts as: its own kind (named, or derived from
// the event), transaction_any for money movements and activity_any for everything. An activity
// without a kind counts as nothing.
func Kinds(in KindInput) []string {
	var own []string
	switch {
	case in.Kind != "":
		own = []string{in.Kind}
	case in.Type == "transaction" && in.Direction == "credit":
		own = []string{"transfer_inbound"}
	case in.Type == "transaction" && in.Channel == "card":
		own = []string{"card_authorization"}
	case in.Type == "transaction":
		own = []string{"transfer_outbound"}
	case in.Type == "login" && in.Outcome == "success":
		own = []string{"login", "auth_success"}
	case in.Type == "login":
		own = []string{"auth_failure"}
	case in.Type != "activity" && len(KindsFor(in.Type)) > 0:
		own = KindsFor(in.Type)[:1]
	default:
		return nil
	}
	if slices.ContainsFunc(own, func(k string) bool { return slices.Contains(MoneyKinds(), k) }) {
		own = append(own, "transaction_any")
	}
	return append(own, "activity_any")
}

// derivedFacts are facts RiskOS reads from the event itself; a customer cannot send them.
func derivedFacts() []string { return []string{"txn.amount", "txn.local_hour", "profile.field"} }

// CheckSuppliedFact checks a fact a customer sent: a catalog fact from a provider outside
// RiskOS, with a value of its type. JSON numbers arrive as float64; money as
// {"amount": <minor units>, "currency": "XXX"}.
func CheckSuppliedFact(name string, v any) error {
	f, ok := Facts[name]
	switch {
	case !ok:
		return fmt.Errorf("%s is not a fact in the rule pack's catalog", name)
	case f.Input == "history" || f.Input == "workflow" || f.Type == FactID || slices.Contains(derivedFacts(), name):
		return fmt.Errorf("%s is computed by RiskOS and cannot be sent", name)
	}
	switch f.Type {
	case FactBool:
		if _, ok := v.(bool); !ok {
			return fmt.Errorf("%s must be true or false", name)
		}
	case FactEnum:
		if s, ok := v.(string); !ok || !slices.Contains(f.Values, s) {
			return fmt.Errorf("%s must be one of %v", name, f.Values)
		}
	case FactMoney:
		if _, _, err := MoneyValue(v); err != nil {
			return fmt.Errorf("%s %w", name, err)
		}
	default:
		n, ok := v.(float64)
		if !ok || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 {
			return fmt.Errorf("%s must be a number of at least 0", name)
		}
		if limit := maxFor(f.Type); limit > 0 && n > limit {
			return fmt.Errorf("%s must be at most %v", name, limit)
		}
	}
	return nil
}

func maxFor(t FactType) float64 {
	switch t {
	case FactHour:
		return 24
	case FactScore:
		return 100
	default:
		return 0
	}
}

var currencyPattern = regexp.MustCompile(`^[A-Z]{3}$`) //nolint:gochecknoglobals // compiled once, never mutated

// MoneyValue reads {"amount": <whole minor units>, "currency": "XXX"}.
func MoneyValue(v any) (amount float64, currency string, err error) {
	m, ok := v.(map[string]any)
	if !ok || len(m) != 2 {
		return 0, "", errors.New(`must be {"amount": <minor units>, "currency": "XXX"}`)
	}
	a, ok := m["amount"].(float64)
	if !ok || a < 0 || a != math.Trunc(a) || a > 1e15 {
		return 0, "", errors.New("amount must be a whole number of minor units")
	}
	c, ok := m["currency"].(string)
	if !ok || !currencyPattern.MatchString(c) {
		return 0, "", errors.New("currency must be an ISO 4217 code")
	}
	return a, c, nil
}

var (
	bvnPattern   = regexp.MustCompile(`^[0-9]{11}$`)      //nolint:gochecknoglobals // compiled once, never mutated
	phonePattern = regexp.MustCompile(`^\+?[0-9]{7,15}$`) //nolint:gochecknoglobals // compiled once, never mutated
)

// CheckRef rejects a reference that looks like the data it should stand for: a BVN is 11
// digits, a phone number 7 to 15 digits. Customers send their own reference instead.
func CheckRef(kind, ref string) error {
	switch {
	case kind == "bvn" && bvnPattern.MatchString(ref):
		return fmt.Errorf("%w: refs.bvn looks like a BVN; send your own reference for it", ErrSensitive)
	case kind == "phone" && phonePattern.MatchString(ref):
		return fmt.Errorf("%w: refs.phone looks like a phone number; send your own reference for it", ErrSensitive)
	}
	return nil
}
