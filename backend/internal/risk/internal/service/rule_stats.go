package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/domain"
	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/rules"
	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/store"
)

// Bounds of rule statistics and of trying a condition on recent events.
const (
	statsDefaultDays = 7
	statsMaxDays     = 30
	recentMax        = 5000
	recentSample     = 25
)

// RuleDayCount is a rule's matches on one UTC day.
type RuleDayCount struct {
	Day     time.Time
	Matches int64
}

// RuleStat is how often one rule matched in a period.
type RuleStat struct {
	RuleID        uuid.UUID
	Matches       int64
	LastMatchedAt *time.Time
	// Daily has one entry per day of the period, oldest first.
	Daily []RuleDayCount
}

// RuleStats is how often each rule of the environment matched over the last days.
type RuleStats struct {
	Since       time.Time
	Days        int
	Evaluations int64
	Rules       []RuleStat
}

// days checks a period in days (0 means the default).
func days(n int) (int, error) {
	if n == 0 {
		return statsDefaultDays, nil
	}
	if n < 1 || n > statsMaxDays {
		return 0, fmt.Errorf("%w: days must be between 1 and %d", domain.ErrBacktestInvalid, statsMaxDays)
	}
	return n, nil
}

// RuleStats counts, for every rule of the environment, the evaluations it matched (active or
// monitoring) per UTC day over the last n days, from one aggregate query. Requires rules:read.
func (s *Service) RuleStats(ctx context.Context, n int) (RuleStats, error) {
	if err := authz.Require(ctx, authz.RulesRead); err != nil {
		return RuleStats{}, err
	}
	n, err := days(n)
	if err != nil {
		return RuleStats{}, err
	}
	today := s.clock.Now().UTC().Truncate(24 * time.Hour)
	out := RuleStats{Since: today.AddDate(0, 0, -(n - 1)), Days: n}
	err = s.runner.Run(ctx, func(ctx context.Context) error {
		list, err := s.store.ListRules(ctx)
		if err != nil {
			return err
		}
		codes := make([]string, len(list))
		for i, r := range list {
			codes[i] = r.Code
		}
		rows, err := s.store.RuleMatchesByDay(ctx, out.Since, codes)
		if err != nil {
			return err
		}
		if out.Evaluations, err = s.store.CountEvaluationsSince(ctx, out.Since); err != nil {
			return err
		}
		byCode := map[string][]int{}
		for i, r := range rows {
			byCode[r.Code] = append(byCode[r.Code], i)
		}
		for _, r := range list {
			st := RuleStat{RuleID: r.ID, Daily: make([]RuleDayCount, n)}
			for d := range n {
				st.Daily[d].Day = out.Since.AddDate(0, 0, d)
			}
			for _, i := range byCode[r.Code] {
				row := rows[i]
				st.Matches += row.Matches
				if d := int(row.Day.Sub(out.Since).Hours() / 24); d >= 0 && d < n {
					st.Daily[d].Matches = row.Matches
				}
				if st.LastMatchedAt == nil || row.LastAt.After(*st.LastMatchedAt) {
					at := row.LastAt
					st.LastMatchedAt = &at
				}
			}
			out.Rules = append(out.Rules, st)
		}
		return nil
	})
	return out, err
}

// RecentMatch is one recent evaluation a condition matched.
type RecentMatch struct {
	EvaluationID uuid.UUID
	EventID      *uuid.UUID
	At           time.Time
	EventType    string
	Amount       *int64
	Currency     *string
	Decision     string
	Score        int
}

// RecentTest is what a condition would have matched among recent evaluations.
type RecentTest struct {
	Since            time.Time
	Checked, Matched int
	// Truncated says the period held more evaluations than were checked (the newest were).
	Truncated bool
	Sample    []RecentMatch
}

// TestRecent runs a condition, saved or not, against the facts recorded with the environment's
// newest evaluations over the last n days (at most 5,000, in one read) and reports what it would
// have matched. Nothing is stored. Requires rules:read.
func (s *Service) TestRecent(ctx context.Context, condition json.RawMessage, n int) (RecentTest, error) {
	if err := authz.Require(ctx, authz.RulesRead); err != nil {
		return RecentTest{}, err
	}
	n, err := days(n)
	if err != nil {
		return RecentTest{}, err
	}
	compiled, err := rules.Compile(condition)
	if err != nil {
		return RecentTest{}, fmt.Errorf("%w: %s", domain.ErrRuleInvalid, strings.TrimPrefix(err.Error(), rules.ErrInvalid.Error()+": "))
	}
	out := RecentTest{Since: s.clock.Now().Add(-time.Duration(n) * 24 * time.Hour), Sample: []RecentMatch{}}
	err = s.runner.Run(ctx, func(ctx context.Context) error {
		rows, err := s.store.RecentFacts(ctx, out.Since, recentMax+1)
		if err != nil {
			return err
		}
		if len(rows) > recentMax {
			rows, out.Truncated = rows[:recentMax], true
		}
		for _, r := range rows {
			var facts rules.Facts
			if len(r.Facts) > 0 {
				if err := json.Unmarshal(r.Facts, &facts); err != nil {
					return fmt.Errorf("decode facts of evaluation %s: %w", r.ID, err)
				}
			}
			out.Checked++
			if !compiled.Match(facts) {
				continue
			}
			out.Matched++
			if len(out.Sample) < recentSample {
				out.Sample = append(out.Sample, recentMatch(r, facts))
			}
		}
		return nil
	})
	return out, err
}

func recentMatch(r store.RecentFacts, facts rules.Facts) RecentMatch {
	m := RecentMatch{EvaluationID: r.ID, EventID: r.EventID, At: r.At, Decision: r.Decision, Score: r.Score}
	m.EventType, _ = facts["event.type"].(string)
	if v, ok := facts["transaction.amount"].(float64); ok {
		amount := int64(v)
		m.Amount = &amount
	}
	if c, ok := facts["transaction.currency"].(string); ok {
		m.Currency = &c
	}
	return m
}
