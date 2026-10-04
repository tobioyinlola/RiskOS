package features

import (
	"math"
	"strconv"
	"time"

	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/domain"
	rulepack "github.com/tobioyinlola/RiskOS/backend/internal/rules"
)

// Rule pack support (ADR-0006): the pack event kinds an event counts as, the facts pack rules
// see, and the entities their windows are kept for.

// Kinds returns the pack event kinds the event counts as.
func (e Event) Kinds() []string {
	in := rulepack.KindInput{Type: e.Type}
	if e.Compliance != nil {
		in.Kind = e.Compliance.Kind
	}
	if t := e.Transaction; t != nil {
		in.Direction, in.Channel = t.Direction, t.Channel
	}
	if l := e.Login; l != nil {
		in.Outcome = l.Outcome
	}
	return rulepack.Kinds(in)
}

// authFactors maps login methods to the pack's authentication factors.
func authFactor(method string) (string, bool) {
	switch method {
	case "password", "otp", "passkey", "biometric":
		return method, true
	}
	return "", false
}

// txnSource maps transaction channels to the pack's sources of funds; instant payments do not
// say whether they were person to person, so they stay unknown unless the customer says.
func txnSource(channel string) (string, bool) {
	switch channel {
	case "card", "bank_transfer", "wallet", "other":
		return channel, true
	}
	return "", false
}

// PackFacts builds the facts pack rules see: what the event says, what the customer sent in its
// compliance block, behaviour history from the feature store and the activity history. Hours of
// the day are in loc, the governing jurisdiction's time zone. History is left out while it is
// degraded or warming up, so rules on it are not evaluated rather than judged on wrong values.
func PackFacts(e Event, v Values, p Activity, loc *time.Location) rulepack.Facts {
	f := rulepack.Facts{}
	if e.Compliance != nil {
		for name, value := range e.Compliance.Facts {
			if m, ok := value.(map[string]any); ok {
				amount, _ := m["amount"].(float64)
				currency, _ := m["currency"].(string)
				f[name] = rulepack.Money{Minor: amount, Currency: currency}
				continue
			}
			f[name] = value
		}
	}
	at := e.OccurredAt.In(loc)
	if kinds := e.Kinds(); len(kinds) > 0 {
		f["event.kind"] = kinds[0]
	}
	if t := e.Transaction; t != nil {
		f["txn.amount"] = rulepack.Money{Minor: float64(t.Amount), Currency: t.Currency}
		if t.Direction == "credit" || t.Direction == "debit" {
			f["txn.direction"] = t.Direction
		}
		f["txn.local_hour"] = float64(at.Hour())
		if s, ok := txnSource(t.Channel); ok {
			if _, sent := f["txn.source"]; !sent {
				f["txn.source"] = s
			}
		}
	}
	if l := e.Login; l != nil {
		if a, ok := authFactor(l.Method); ok {
			if _, sent := f["auth.factor"]; !sent {
				f["auth.factor"] = a
			}
		}
	}
	if e.Profile != nil {
		f["profile.field"] = e.Profile.Fields
	}
	if !v.available(domain.SourceHistory) || v.Warming {
		return f
	}
	// For a user with no history every device and payee is new: true, unlike the scoring
	// signals, which ignore it as meaningless (a regulatory control still applies to it).
	if e.Actor.DeviceID != "" {
		f["device.is_new"] = v.NewUser || v.NewDevice
	}
	if e.BeneficiaryID() != "" {
		f["beneficiary.is_new"] = v.NewUser || v.NewBeneficiary
		if e.Transaction != nil {
			f["txn.first_to_beneficiary"] = v.NewUser || v.NewBeneficiary
		}
	}
	if v.SenderRelated != nil {
		f["sender.related"] = *v.SenderRelated
	}
	if v.ToPriorSender != nil {
		f["txn.to_prior_sender"] = *v.ToPriorSender
	}
	if e.Context.Country != "" && !v.NewUser {
		f["geo.country_usual"] = !v.NewCountry
	}
	if e.IsDebit() && v.AmountCount >= domain.AmountMinHistory && v.AmountStd > 0 {
		z := (float64(e.Transaction.Amount) - v.AmountMean) / v.AmountStd
		f["history.amount_zscore"] = math.Round(z*100) / 100
	}
	if p.UserFirst != nil {
		f["customer.history_age"] = math.Max(0, e.OccurredAt.Sub(*p.UserFirst).Seconds())
	} else if v.NewUser {
		f["customer.history_age"] = 0.0
	}
	if p.AccountLast != nil {
		f["account.inactive_for"] = math.Max(0, e.OccurredAt.Sub(*p.AccountLast).Seconds())
	}
	// A usual hour is one the user was active in before (the jurisdiction's time).
	if p.Hours != nil && *p.Hours != 0 {
		f["history.hour_usual"] = (*p.Hours>>at.Hour())&1 == 1
	}
	if g := e.Context.Geo; g != nil && p.LastGeo != nil && p.LastGeoAt != nil {
		if v, ok := speedKmh(*p.LastGeo, *p.LastGeoAt, *g, e.OccurredAt); ok {
			f["geo.speed_from_previous"] = v
		}
	}
	if rows := e.ExportRows(); rows != nil {
		if r, ok := ratioTo(*rows, p.ExportUser); ok {
			f["export.rows_to_user_baseline"] = r
		}
		if r, ok := ratioTo(*rows, p.ExportTenant); ok {
			f["export.rows_to_tenant_baseline"] = r
		}
	}
	if e.Transaction != nil && p.MaxAmount != nil && *p.MaxAmount > 0 {
		f["history.amount_to_max"] = math.Round(float64(e.Transaction.Amount) / *p.MaxAmount * 100) / 100
	}
	return f
}

// Entity returns the identifier of the entity a window is kept for ("" when the event has none):
// the actor's user, account and device, the address, the payee, the whole environment
// ("network"), or a reference the customer sent.
func (e Event) Entity(kind string) string {
	switch kind {
	case "account":
		return e.Actor.AccountID
	case "customer", "user":
		return e.Actor.UserID
	case "device":
		return e.Actor.DeviceID
	case "ip":
		return e.Context.IP
	case "beneficiary":
		return e.BeneficiaryID()
	case "network":
		return "*"
	}
	return e.ref(kind)
}

// FieldValue returns the identifier a distinct count counts ("" when the event has none).
func (e Event) FieldValue(field string) string {
	switch field {
	case "account.id":
		return e.Actor.AccountID
	case "customer.id":
		return e.Actor.UserID
	case "agent.id":
		return e.ref("agent")
	case "beneficiary.id":
		return e.BeneficiaryID()
	case "card.id":
		return e.ref("card")
	case "sender.id":
		return e.ref("sender")
	case "reporter.id":
		return e.ref("reporter")
	case "funding_source.id":
		return e.ref("funding_source")
	case "txn.amount":
		if e.Transaction != nil {
			return strconv.FormatInt(e.Transaction.Amount, 10)
		}
	}
	return ""
}

func (e Event) ref(kind string) string {
	if e.Compliance == nil {
		return ""
	}
	return e.Compliance.Refs[kind]
}

// WindowQuery asks one aggregate of a window.
type WindowQuery struct {
	Fn     string
	Window time.Duration
}

// WindowOp is one window an event touches: recorded in (Record), read (Queries), or both.
type WindowOp struct {
	// Key identifies the window definition; EntityKind and EntityID the entity it is kept for.
	Key, EntityKind, EntityID string
	Retention                 time.Duration
	// Record stores the event; Unknown stores it as "filter not checked".
	Record, Unknown bool
	// Field is the identifier distinct counts count (hashed before it reaches Redis); Per the
	// value distinct counts are grouped by.
	Field, Per string
	Queries    []WindowQuery
}

// WindowEvent is the event as windows store it.
type WindowEvent struct {
	// ID identifies the event (hashed before it reaches Redis).
	ID       string
	At       time.Time
	Amount   int64
	Currency string
	// UserID and AccountID key the activity profile.
	UserID, AccountID string
	// Hour is the hour of the day in the jurisdiction's time zone (-1: none recorded); ExportRows the rows of a data
	// export, when the event says; Geo where the end user was.
	Hour       int
	ExportRows *float64
	Geo        *Geo
}

// Baseline is a running mean and how many observations it has.
type Baseline struct {
	Count int64
	Mean  float64
}

// WindowValue is one aggregate: OK is false when the store failed or the window holds an event
// whose filter could not be checked.
type WindowValue struct {
	Value float64
	OK    bool
}

// Activity is the event's activity history: the user's first activity, the account's previous
// activity and the user's largest amount before; nil when unknown.
type Activity struct {
	UserFirst, AccountLast *time.Time
	MaxAmount              *float64
	// Hours is the user's mask of hours of the day (the jurisdiction's time) active in before
	// this event.
	Hours *int64
	// ExportUser and ExportTenant are the rows-per-export baselines before this event.
	ExportUser, ExportTenant Baseline
	// LastGeo is the user's previous location and LastGeoAt when it was.
	LastGeo   *Geo
	LastGeoAt *time.Time
}

// earthRadiusKm is the mean radius of the Earth.
const earthRadiusKm = 6371.0

// distanceKm is the great-circle distance between two points (haversine).
func distanceKm(a, b Geo) float64 {
	rad := func(d float64) float64 { return d * math.Pi / 180 }
	dLat, dLon := rad(b.Lat-a.Lat), rad(b.Lon-a.Lon)
	h := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(rad(a.Lat))*math.Cos(rad(b.Lat))*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * earthRadiusKm * math.Asin(math.Sqrt(h))
}

// speedKmh is the speed needed to travel from the previous location to this one in the time
// between them; the time counts as at least a minute, so rounding cannot make it infinite.
func speedKmh(prev Geo, prevAt time.Time, cur Geo, at time.Time) (float64, bool) {
	if at.Before(prevAt) {
		return 0, false
	}
	hours := math.Max(at.Sub(prevAt).Hours(), 1.0/60)
	return math.Round(distanceKm(prev, cur)/hours*10) / 10, true
}

// ExportRows returns the rows of a data export the event sends.
func (e Event) ExportRows() *float64 {
	if e.Compliance == nil {
		return nil
	}
	if n, ok := e.Compliance.Facts["export.rows"].(float64); ok {
		return &n
	}
	return nil
}

// ratioTo returns value over a baseline's mean, once it has enough history.
func ratioTo(value float64, b Baseline) (float64, bool) {
	if b.Count < domain.AmountMinHistory || b.Mean <= 0 {
		return 0, false
	}
	return math.Round(value/b.Mean*100) / 100, true
}
