package service

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/domain"
	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/engine"
	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/store"
	rulepack "github.com/tobioyinlola/RiskOS/backend/internal/rules"
)

// Backtest bounds: how far back, and how many events at most.
const (
	MaxBacktestDays   = 30
	MaxBacktestEvents = 20_000
	backtestSamples   = 20
)

// BacktestInput asks what a rule would have done over recent events.
type BacktestInput struct {
	RuleID string
	// Settings to try; nil replays the environment's current configuration only.
	Settings map[string]float64
	Days     int
}

// BacktestTally is one configuration's result.
type BacktestTally struct {
	Triggered, Fired, Clear, NotEvaluated int
	Missing                               map[string]int
	// Samples are events the rule fired on (at most 20, oldest first).
	Samples []uuid.UUID
}

// BacktestResult compares a candidate configuration with the current one over the same events.
type BacktestResult struct {
	RuleID   string
	From, To time.Time
	// Replayed events had a rule pack replay record; Truncated is set when the event limit cut
	// the period short.
	Replayed  int
	Truncated bool
	Candidate BacktestTally
	Current   BacktestTally
}

// replayFacts restores stored facts to the interpreter's types: amounts were stored as
// {"Minor", "Currency"} and lists as JSON arrays.
func replayFacts(stored map[string]any) rulepack.Facts {
	out := rulepack.Facts{}
	for k, v := range stored {
		switch x := v.(type) {
		case map[string]any:
			minor, _ := x["Minor"].(float64)
			currency, _ := x["Currency"].(string)
			out[k] = rulepack.Money{Minor: minor, Currency: currency}
		case []any:
			list := make([]string, 0, len(x))
			for _, item := range x {
				if s, ok := item.(string); ok {
					list = append(list, s)
				}
			}
			out[k] = list
		default:
			out[k] = v
		}
	}
	return out
}

func tallyOf(t engine.Tally, events []uuid.UUID) BacktestTally {
	out := BacktestTally{Triggered: t.Triggered, Fired: t.Fired, Clear: t.Clear, NotEvaluated: t.NotEvaluated, Missing: t.Missing}
	for _, i := range t.FiredAt {
		if len(out.Samples) == backtestSamples {
			break
		}
		if events[i] != uuid.Nil {
			out.Samples = append(out.Samples, events[i])
		}
	}
	return out
}

// Backtest replays the environment's recent ingested events through one pack rule, with the given
// settings and with the current configuration, windows kept in memory. Nothing is stored and no
// live state is read or changed. Requires compliance:read and events:read.
func (s *Service) Backtest(ctx context.Context, in BacktestInput) (BacktestResult, error) {
	if err := authz.Require(ctx, authz.EventsRead); err != nil {
		return BacktestResult{}, err
	}
	if in.Days < 1 || in.Days > MaxBacktestDays {
		return BacktestResult{}, fmt.Errorf("%w: days must be 1 to %d", domain.ErrBacktestInvalid, MaxBacktestDays)
	}
	if s.packs == nil {
		return BacktestResult{}, fmt.Errorf("%w: rule packs are not enabled", domain.ErrBacktestInvalid)
	}
	now := s.clock.Now()
	out := BacktestResult{RuleID: in.RuleID, From: now.Add(-time.Duration(in.Days) * 24 * time.Hour), To: now}
	err := s.runner.Run(ctx, func(ctx context.Context) error {
		candidate, current, err := s.packs.Candidates(ctx, in.RuleID, in.Settings)
		if err != nil {
			return err
		}
		rows, err := s.store.PackReplays(ctx, out.From, MaxBacktestEvents+1)
		if err != nil {
			return err
		}
		if len(rows) > MaxBacktestEvents {
			rows, out.Truncated = rows[:MaxBacktestEvents], true
		}
		type replayed struct {
			id uuid.UUID
			ev engine.ReplayEvent
		}
		list := make([]replayed, 0, len(rows))
		for _, r := range rows {
			var f storedFeatures
			if err := json.Unmarshal(r.Features, &f); err != nil || f.Pack == nil || f.Pack.Replay == nil {
				continue
			}
			id := uuid.Nil
			if r.EventID != nil {
				id = *r.EventID
			}
			list = append(list, replayed{id: id, ev: engine.ReplayEvent{Kinds: f.Pack.Kinds, Facts: replayFacts(f.Pack.Facts), Replay: *f.Pack.Replay}})
		}
		// Windows run on event time; ingestion order can differ.
		slices.SortStableFunc(list, func(a, b replayed) int { return a.ev.Replay.At.Compare(b.ev.Replay.At) })
		events := make([]engine.ReplayEvent, len(list))
		eventIDs := make([]uuid.UUID, len(list))
		for i, r := range list {
			events[i], eventIDs[i] = r.ev, r.id
		}
		out.Replayed = len(events)
		out.Candidate = tallyOf(engine.Backtest(candidate, events), eventIDs)
		out.Current = tallyOf(engine.Backtest(current, events), eventIDs)
		return nil
	})
	return out, err
}

// RestrictedSignals lists the environment's evaluations with compliance-only signals in the last
// days days (GOV-09). Requires compliance:sensitive, held only by compliance officers.
func (s *Service) RestrictedSignals(ctx context.Context, days int) ([]store.RestrictedEvaluation, error) {
	if err := authz.Require(ctx, authz.ComplianceSensitive); err != nil {
		return nil, err
	}
	if days < 1 || days > MaxBacktestDays {
		return nil, fmt.Errorf("%w: days must be 1 to %d", domain.ErrBacktestInvalid, MaxBacktestDays)
	}
	var out []store.RestrictedEvaluation
	err := s.runner.Run(ctx, func(ctx context.Context) error {
		var err error
		out, err = s.store.RestrictedSignals(ctx, s.clock.Now().Add(-time.Duration(days)*24*time.Hour))
		return err
	})
	return out, err
}
