// Package features turns a normalized event and its feature values into built-in signals and
// rule facts. It is pure: the feature values are read by the feature store.
package features

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/domain"
	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/rules"
	rulepack "github.com/tobioyinlola/RiskOS/backend/internal/rules"
)

// Event is the risk module's view of a normalized event (EVENTS.md). It is parsed from the
// normalized payload so the risk module does not depend on the events module.
type Event struct {
	Type       string    `json:"type"`
	OccurredAt time.Time `json:"occurred_at"`
	Actor      struct {
		UserID    string `json:"user_id"`
		AccountID string `json:"account_id"`
		DeviceID  string `json:"device_id"`
	} `json:"actor"`
	Context struct {
		Country string `json:"country"`
		IP      string `json:"ip"`
		Geo     *Geo   `json:"geo"`
	} `json:"context"`
	Data json.RawMessage `json:"data"`
	// Compliance is the event's compliance block (EVENTS.md#compliance), or nil.
	Compliance *rulepack.EventContext `json:"compliance"`

	Transaction *Transaction `json:"-"`
	Login       *Login       `json:"-"`
	Beneficiary *Beneficiary `json:"-"`
	Profile     *Profile     `json:"-"`
}

// Geo is where the end user was (rounded to about 1 km).
type Geo struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

// Transaction data.
type Transaction struct {
	Amount        int64  `json:"amount"`
	Currency      string `json:"currency"`
	Direction     string `json:"direction"`
	Channel       string `json:"channel"`
	BeneficiaryID string `json:"beneficiary_id"`
}

// Login data.
type Login struct {
	Outcome string `json:"outcome"`
	Method  string `json:"method"`
}

// Beneficiary data (beneficiary_added).
type Beneficiary struct {
	BeneficiaryID string `json:"beneficiary_id"`
	Country       string `json:"country"`
}

// Profile data (profile_change).
type Profile struct {
	Fields []string `json:"fields"`
}

// ParseEvent parses a normalized event.
func ParseEvent(normalized []byte) (Event, error) {
	var e Event
	if err := json.Unmarshal(normalized, &e); err != nil {
		return Event{}, fmt.Errorf("parse normalized event: %w", err)
	}
	var target any
	switch e.Type {
	case "transaction":
		e.Transaction = &Transaction{}
		target = e.Transaction
	case "login":
		e.Login = &Login{}
		target = e.Login
	case "beneficiary_added":
		e.Beneficiary = &Beneficiary{}
		target = e.Beneficiary
	case "profile_change":
		e.Profile = &Profile{}
		target = e.Profile
	}
	if target != nil {
		if err := json.Unmarshal(e.Data, target); err != nil {
			return Event{}, fmt.Errorf("parse %s data: %w", e.Type, err)
		}
	}
	return e, nil
}

// BeneficiaryID returns the beneficiary the event involves, if any.
func (e Event) BeneficiaryID() string {
	switch {
	case e.Transaction != nil:
		return e.Transaction.BeneficiaryID
	case e.Beneficiary != nil:
		return e.Beneficiary.BeneficiaryID
	}
	return ""
}

// IsDebit reports whether the event moves money out.
func (e Event) IsDebit() bool { return e.Transaction != nil && e.Transaction.Direction == "debit" }

// CredentialChange reports whether a profile change touched login credentials or recovery
// channels (email, phone, password): the first step of a typical account takeover.
func (e Event) CredentialChange() bool {
	if e.Profile == nil {
		return false
	}
	for _, f := range e.Profile.Fields {
		if f == "email" || f == "phone" || f == "password" {
			return true
		}
	}
	return false
}

// Values are the feature store's answers for one event. Counts include the event itself.
type Values struct {
	NewUser                bool
	NewDevice              bool
	NewBeneficiary         bool
	NewCountry             bool
	RecentCredentialChange bool
	// SenderRelated (an incoming payment) says whether the user paid this sender before;
	// ToPriorSender (a payment out) whether this payee paid the user before. Nil when the event
	// has no sender or payee.
	SenderRelated         *bool
	ToPriorSender         *bool
	AccountTxn1m          int64
	AccountTxn1h          int64
	AccountTxn24h         int64
	FailedLogins15m       int64
	DeviceUsers           int64
	BeneficiarySenders24h int64
	// Amount history for the transaction's currency, before this event.
	AmountCount int64
	AmountMean  float64
	AmountStd   float64
	// Degraded lists feature sources (domain.Source*) that could not be read.
	Degraded []string
	// Warming is set while history rebuilds after the feature store lost its state
	// (DISASTER_RECOVERY.md): every user looks new, so history signals would mislead.
	Warming bool
}

// warms reports the signals held back while history warms up: with the history gone they would fire
// for known users ("new device") or judge amounts against a baseline of a few events.
func warms(code domain.SignalCode) bool {
	switch code {
	case domain.SignalNewUser, domain.SignalNewDevice, domain.SignalNewBeneficiary,
		domain.SignalGeoAnomaly, domain.SignalAmountAnomaly:
		return true
	}
	return false
}

func (v Values) available(source string) bool { return !slices.Contains(v.Degraded, source) }

// AmountRatio is the amount relative to the user's mean in the same currency (0 when unknown).
func AmountRatio(e Event, v Values) float64 {
	if e.Transaction == nil || v.AmountCount == 0 || v.AmountMean <= 0 {
		return 0
	}
	return float64(e.Transaction.Amount) / v.AmountMean
}

// Signals derives built-in signals. A user with no history gets only the new_user signal from
// history-based checks (cold start, ARCHITECTURE.md §10). Signals whose source is degraded are
// returned in degraded instead, and history signals held back during a warm-up in warming.
func Signals(e Event, v Values, s domain.Settings) (signals []domain.Signal, degraded, warming []string) {
	add := func(code domain.SignalCode) {
		spec, _ := domain.BuiltinSignal(code)
		if w := s.Weight(spec); w > 0 {
			signals = append(signals, domain.Signal{Code: spec.Code, Label: spec.Label, Weight: w})
		}
	}
	check := func(code domain.SignalCode, applies, fires bool) {
		if !applies {
			return
		}
		spec, _ := domain.BuiltinSignal(code)
		if !v.available(spec.Source) {
			degraded = append(degraded, code)
			return
		}
		if v.Warming && warms(code) {
			warming = append(warming, code)
			return
		}
		if fires {
			add(code)
		}
	}

	// Cold start: with no history, "new X" is always true and meaningless, so only new_user fires.
	// Applicability does not depend on history, so an unavailable source is always reported.
	known := !v.NewUser
	check(domain.SignalNewUser, true, v.NewUser)
	check(domain.SignalNewDevice, e.Actor.DeviceID != "", known && v.NewDevice)
	check(domain.SignalNewBeneficiary, e.BeneficiaryID() != "", known && v.NewBeneficiary)
	check(domain.SignalGeoAnomaly, e.Context.Country != "", known && v.NewCountry)
	check(domain.SignalAmountAnomaly, e.IsDebit(), known && amountAnomaly(e, v))
	check(domain.SignalRecentCredentialChange, e.IsDebit(), v.RecentCredentialChange)
	check(domain.SignalVelocityAnomaly, e.Transaction != nil,
		v.AccountTxn1m > domain.VelocityMaxPerMinute || v.AccountTxn1h > domain.VelocityMaxPerHour)
	check(domain.SignalFailedLoginBurst, e.Login != nil, v.FailedLogins15m >= domain.FailedLoginsMax)
	check(domain.SignalSharedDevice, e.Actor.DeviceID != "", v.DeviceUsers >= domain.SharedDeviceMinUsers)
	check(domain.SignalBeneficiaryFanIn, e.Transaction != nil && e.BeneficiaryID() != "",
		v.BeneficiarySenders24h >= domain.FanInMinSenders)
	return signals, degraded, warming
}

func amountAnomaly(e Event, v Values) bool {
	if v.AmountCount < domain.AmountMinHistory || v.AmountMean <= 0 {
		return false
	}
	amount := float64(e.Transaction.Amount)
	return amount > v.AmountMean+domain.AmountSigmas*v.AmountStd && amount > domain.AmountMinMultiple*v.AmountMean
}

// Facts builds the facts rules are evaluated against (rules.Fields). Facts of a degraded source
// are omitted, so rules on them do not match rather than matching on a wrong default.
func Facts(e Event, v Values) rules.Facts {
	f := rules.Facts{"event.type": e.Type}
	if e.Context.Country != "" {
		f["context.country"] = e.Context.Country
	}
	if t := e.Transaction; t != nil {
		f["transaction.amount"] = float64(t.Amount)
		f["transaction.currency"] = t.Currency
		f["transaction.direction"] = t.Direction
		if t.Channel != "" {
			f["transaction.channel"] = t.Channel
		}
	}
	if l := e.Login; l != nil {
		f["login.outcome"] = l.Outcome
		f["login.method"] = l.Method
	}
	if b := e.Beneficiary; b != nil && b.Country != "" {
		f["beneficiary.country"] = b.Country
	}
	if e.Profile != nil {
		f["profile.credential_change"] = e.CredentialChange()
	}
	if v.available(domain.SourceHistory) {
		f["feature.new_user"] = v.NewUser
		f["feature.recent_credential_change"] = v.RecentCredentialChange
		if !v.NewUser {
			if e.Actor.DeviceID != "" {
				f["feature.new_device"] = v.NewDevice
			}
			if e.BeneficiaryID() != "" {
				f["feature.new_beneficiary"] = v.NewBeneficiary
			}
			if e.Context.Country != "" {
				f["feature.new_country"] = v.NewCountry
			}
		}
		if e.Transaction != nil {
			f["feature.amount_to_mean_ratio"] = math.Round(AmountRatio(e, v)*100) / 100
		}
	}
	if v.available(domain.SourceVelocity) {
		if e.Transaction != nil {
			f["feature.account_txn_1m"] = float64(v.AccountTxn1m)
			f["feature.account_txn_1h"] = float64(v.AccountTxn1h)
			f["feature.account_txn_24h"] = float64(v.AccountTxn24h)
		}
		if e.Login != nil {
			f["feature.failed_logins_15m"] = float64(v.FailedLogins15m)
		}
	}
	if v.available(domain.SourceNetwork) {
		if e.Actor.DeviceID != "" {
			f["feature.device_users"] = float64(v.DeviceUsers)
		}
		if e.Transaction != nil && e.BeneficiaryID() != "" {
			f["feature.beneficiary_senders_24h"] = float64(v.BeneficiarySenders24h)
		}
	}
	return f
}
