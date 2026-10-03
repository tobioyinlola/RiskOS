package features

import (
	"crypto/sha256"
	"encoding/base64"
	"time"
)

// Replay is what a backtest needs to put an event through pack windows again: its time and
// amount, and the entities windows are kept for and the identifiers they count. Identifiers are
// hashed: a backtest only compares them, so evaluations never hold a second copy of the values.
type Replay struct {
	At       time.Time         `json:"at"`
	Amount   int64             `json:"amount,omitempty"`
	Currency string            `json:"currency,omitempty"`
	Entities map[string]string `json:"entities,omitempty"`
	Fields   map[string]string `json:"fields,omitempty"`
}

// replayEntities and replayFields are every entity kind and counted identifier of the catalog.
func replayEntities() []string {
	return []string{
		"account", "customer", "user", "device", "ip", "beneficiary", "network", "card", "bvn", "phone",
		"address", "loan", "inflow", "merchant", "api_key",
	}
}

func replayFields() []string {
	return []string{"account.id", "beneficiary.id", "card.id", "sender.id", "reporter.id", "funding_source.id", "txn.amount"}
}

func digest(kind, value string) string {
	h := sha256.Sum256([]byte(kind + "\x00" + value))
	return base64.RawURLEncoding.EncodeToString(h[:12])
}

// Replay returns the event's replay record.
func (e Event) Replay() Replay {
	r := Replay{At: e.OccurredAt, Entities: map[string]string{}, Fields: map[string]string{}}
	if t := e.Transaction; t != nil {
		r.Amount, r.Currency = t.Amount, t.Currency
	}
	for _, k := range replayEntities() {
		if v := e.Entity(k); v != "" {
			r.Entities[k] = digest(k, v)
		}
	}
	for _, f := range replayFields() {
		v := e.FieldValue(f)
		switch {
		case v == "":
		case f == "txn.amount":
			// Distinct counts group equal amounts: the amount itself is not identifying.
			r.Fields[f] = v
		default:
			r.Fields[f] = digest(f, v)
		}
	}
	return r
}

// MemoryWindows keeps pack windows in memory for a backtest, with the semantics of the Redis
// windows: entries by event time, unknown entries make a window unavailable, sums count only the
// event's currency.
type MemoryWindows struct {
	entries map[string][]memoryEntry
}

type memoryEntry struct {
	at       time.Time
	unknown  bool
	field    string
	per      string
	amount   int64
	currency string
}

// NewMemoryWindows returns empty windows.
func NewMemoryWindows() *MemoryWindows { return &MemoryWindows{entries: map[string][]memoryEntry{}} }

// Record adds an event to the window key of entity.
func (m *MemoryWindows) Record(key, entity string, op WindowOp, r Replay) {
	k := key + "|" + entity
	m.entries[k] = append(m.entries[k], memoryEntry{
		at: r.At, unknown: op.Unknown, field: op.Field, per: op.Per, amount: r.Amount, currency: r.Currency,
	})
}

// Value computes fn over the entity's window of the given length ending at at.
func (m *MemoryWindows) Value(key, entity, fn string, window time.Duration, at time.Time, currency string) WindowValue {
	count, sum := 0, 0.0
	fields := map[string]int{}
	per := map[string]map[string]bool{}
	for _, e := range m.entries[key+"|"+entity] {
		if e.at.Before(at.Add(-window)) || e.at.After(at) {
			continue
		}
		if e.unknown {
			return WindowValue{}
		}
		count++
		if e.currency == currency {
			sum += float64(e.amount)
		}
		if e.field != "" {
			fields[e.field]++
			if e.per != "" {
				if per[e.per] == nil {
					per[e.per] = map[string]bool{}
				}
				per[e.per][e.field] = true
			}
		}
	}
	var v float64
	switch fn {
	case "count":
		v = float64(count)
	case "exists":
		if count > 0 {
			v = 1
		}
	case "sum":
		v = sum
	case "daily_avg":
		v = sum / (window.Hours() / 24)
	case "distinct_count":
		v = float64(len(fields))
	case "distinct_count_once":
		for _, n := range fields {
			if n == 1 {
				v++
			}
		}
	case "max_distinct_per_value":
		for _, set := range per {
			v = max(v, float64(len(set)))
		}
	default:
		return WindowValue{}
	}
	return WindowValue{Value: v, OK: true}
}
