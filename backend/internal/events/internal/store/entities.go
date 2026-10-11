package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/tobioyinlola/RiskOS/backend/internal/events/internal/domain"
	"github.com/tobioyinlola/RiskOS/backend/internal/events/internal/store/sqlcgen"
)

// profileRow is what the three per-kind profile queries return (sqlc generates one type each).
type profileRow struct {
	EventCount                               int64
	FirstSeen, LastSeen                      time.Time
	MaxScore, MeanScore                      int32
	Decisions                                []byte
	Users, Accounts, Devices, Countries, Ips []byte
}

// Profile summarizes an entity's most recent events; domain.ErrNotFound when it has none.
func (Store) Profile(ctx context.Context, kind domain.EntityKind, ref string) (domain.Profile, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return domain.Profile{}, err
	}
	var r profileRow
	switch kind {
	case domain.EntityUser:
		row, e := q.UserProfile(ctx, sqlcgen.UserProfileParams{TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, Ref: ref})
		r, err = profileRow(row), e
	case domain.EntityAccount:
		row, e := q.AccountProfile(ctx, sqlcgen.AccountProfileParams{TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, Ref: &ref})
		r, err = profileRow(row), e
	case domain.EntityDevice:
		row, e := q.DeviceProfile(ctx, sqlcgen.DeviceProfileParams{TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, Ref: &ref})
		r, err = profileRow(row), e
	default:
		return domain.Profile{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Profile{}, fmt.Errorf("entity profile: %w", err)
	}
	if r.EventCount == 0 {
		return domain.Profile{}, domain.ErrNotFound
	}
	p := domain.Profile{
		Kind: kind, Ref: ref, Events: int(r.EventCount), FirstSeen: r.FirstSeen, LastSeen: r.LastSeen,
		MaxScore: int(r.MaxScore), MeanScore: int(r.MeanScore),
	}
	if err := json.Unmarshal(r.Decisions, &p.Decisions); err != nil {
		return domain.Profile{}, fmt.Errorf("decode decision counts: %w", err)
	}
	for dst, raw := range map[*[]domain.Linked][]byte{
		&p.Users: r.Users, &p.Accounts: r.Accounts, &p.Devices: r.Devices, &p.Countries: r.Countries, &p.IPs: r.Ips,
	} {
		if err := json.Unmarshal(raw, dst); err != nil {
			return domain.Profile{}, fmt.Errorf("decode linked entities: %w", err)
		}
	}
	return p, nil
}

// Timeline counts events per bucket since since. Hours the rollup has computed are read from it
// and the rest from events (see RollupEvents), so the answer is the same either way.
func (s Store) Timeline(ctx context.Context, since time.Time, bucket time.Duration) ([]domain.Point, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return nil, err
	}
	// Without a rollup both bounds are since: the rollup part is empty and everything is raw.
	covered, through := since, since
	if st, err := s.RollupState(ctx, RollupAnalytics); err != nil {
		return nil, err
	} else if st != nil {
		covered, through = st.CoveredFrom, st.RolledThrough
	}
	rows, err := q.EventTimeline(ctx, sqlcgen.EventTimelineParams{
		Step: pgtype.Interval{Microseconds: bucket.Microseconds(), Valid: true}, Origin: since,
		TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, Since: since,
		CoveredFrom: covered, RolledThrough: through,
	})
	if err != nil {
		return nil, fmt.Errorf("event timeline: %w", err)
	}
	out := make([]domain.Point, len(rows))
	for i, r := range rows {
		mean := 0
		if r.Events > 0 {
			mean = int(math.Round(float64(r.ScoreSum) / float64(r.Events)))
		}
		out[i] = domain.Point{
			At: r.Bucket, Events: int(r.Events), MeanScore: mean,
			Minimal: int(r.Minimal), Low: int(r.Low), Elevated: int(r.Elevated), High: int(r.High), Critical: int(r.Critical),
			Decisions: map[string]int{
				"allow": int(r.Allow), "fee": int(r.Fee), "flag": int(r.Flag), "step_up": int(r.StepUp),
				"review": int(r.Review), "restrict": int(r.Restrict), "hold": int(r.Hold),
				"block_session": int(r.BlockSession), "decline": int(r.Decline), "block": int(r.Block),
			},
		}
	}
	return out, nil
}

// RollupAnalytics is this module's row in rollup_state (the risk module keeps its own).
const RollupAnalytics = "analytics"

// RollupEnvironment names an environment the rollup job visits.
type RollupEnvironment struct {
	TenantID, EnvironmentID uuid.UUID
}

// RollupEnvironments lists the environments of active tenants. It runs outside any tenant scope
// and returns identifiers only.
func RollupEnvironments(ctx context.Context, db sqlcgen.DBTX) ([]RollupEnvironment, error) {
	rows, err := db.Query(ctx, `SELECT tenant_id, environment_id FROM rollup_environments()`)
	if err != nil {
		return nil, fmt.Errorf("list rollup environments: %w", err)
	}
	defer rows.Close()
	var out []RollupEnvironment
	for rows.Next() {
		var e RollupEnvironment
		if err := rows.Scan(&e.TenantID, &e.EnvironmentID); err != nil {
			return nil, fmt.Errorf("scan rollup environment: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list rollup environments: %w", err)
	}
	return out, nil
}

// RollupWindow is how far a rollup has been computed: whole hours in [CoveredFrom, RolledThrough).
type RollupWindow struct {
	CoveredFrom, RolledThrough time.Time
}

// RollupState returns the scope's rollup window, or nil if it has none yet.
func (Store) RollupState(ctx context.Context, name string) (*RollupWindow, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return nil, err
	}
	row, err := q.GetRollupState(ctx, sqlcgen.GetRollupStateParams{TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, Name: name})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil //nolint:nilnil // not rolled up yet is a normal outcome
	}
	if err != nil {
		return nil, fmt.Errorf("get rollup state: %w", err)
	}
	return &RollupWindow{CoveredFrom: row.CoveredFrom, RolledThrough: row.RolledThrough}, nil
}

// RollupEvents recomputes the hourly aggregates of the whole hours in [from, until) and moves the
// scope's rollup window to cover through until (starting at coveredFrom when it has none).
func (s Store) RollupEvents(ctx context.Context, coveredFrom, from, until, now time.Time) error {
	q, scope, err := scoped(ctx)
	if err != nil {
		return err
	}
	if _, err := q.RollupEventHours(ctx, sqlcgen.RollupEventHoursParams{
		TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, RangeStart: from, RangeEnd: until,
	}); err != nil {
		return fmt.Errorf("roll up events: %w", err)
	}
	if err := q.PutRollupState(ctx, sqlcgen.PutRollupStateParams{
		TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, Name: RollupAnalytics,
		CoveredFrom: coveredFrom, RolledThrough: until, Now: now,
	}); err != nil {
		return fmt.Errorf("put rollup state: %w", err)
	}
	return nil
}

// Bounds of the top-users walk: how many of the highest-scoring events are read before giving up
// on finding the limit's worth of users, and how many users tied at the cutoff are followed.
const (
	topScoreRows = 5000
	topTieUsers  = 20000
)

// TopUsers ranks users by their highest score since since, then by when they were last seen.
//
// It does not aggregate the window. It reads the window's events in score order (events_by_score),
// which yields the users with the highest scores first; once limit distinct users are known, the
// score of the last of them is the cutoff, every user at or above it is a candidate, and only
// the candidates' statistics are computed. The result is the same as aggregating every event.
// When the walk cannot tell (fewer than limit users in the first topScoreRows events, so the rest
// of the window might hold more), it aggregates the window as before.
func (s Store) TopUsers(ctx context.Context, since time.Time, limit int) ([]domain.TopUser, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return nil, err
	}
	walked, err := q.TopScoreEvents(ctx, sqlcgen.TopScoreEventsParams{
		TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, Since: since, RowLimit: topScoreRows,
	})
	if err != nil {
		return nil, fmt.Errorf("top score events: %w", err)
	}
	var distinct []string
	seen := map[string]bool{}
	cutoff := int32(0)
	for _, r := range walked {
		if seen[r.UserRef] {
			continue
		}
		seen[r.UserRef] = true
		distinct = append(distinct, r.UserRef)
		if len(distinct) == limit {
			cutoff = r.RiskScore
			break
		}
	}
	complete := len(walked) < topScoreRows // every event of the window was read
	if len(distinct) < limit && !complete {
		return s.topUsersByAggregate(ctx, since, limit)
	}

	candidates := distinct
	if len(distinct) == limit {
		// Users tied with the last one may not have been reached: take everyone at the cutoff.
		more, err := q.UsersAtScore(ctx, sqlcgen.UsersAtScoreParams{
			TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, Since: since, MinScore: int16(cutoff), RowLimit: topTieUsers, //nolint:gosec // 0..100
		})
		if err != nil {
			return nil, fmt.Errorf("users at score: %w", err)
		}
		if len(more) >= topTieUsers {
			return s.topUsersByAggregate(ctx, since, limit)
		}
		candidates = more
	}
	stats, err := q.UserWindowStats(ctx, sqlcgen.UserWindowStatsParams{
		TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, Since: since, Users: candidates,
	})
	if err != nil {
		return nil, fmt.Errorf("user window stats: %w", err)
	}
	out := make([]domain.TopUser, len(stats))
	for i, r := range stats {
		out[i] = domain.TopUser{User: r.UserRef, MaxScore: int(r.MaxScore), Events: int(r.Events), Flagged: int(r.Flagged), LastSeen: r.LastSeen}
	}
	slices.SortFunc(out, func(a, b domain.TopUser) int {
		switch {
		case a.MaxScore != b.MaxScore:
			return b.MaxScore - a.MaxScore
		case !a.LastSeen.Equal(b.LastSeen):
			if a.LastSeen.After(b.LastSeen) {
				return -1
			}
			return 1
		default:
			return 0
		}
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// topUsersByAggregate is the straightforward ranking: aggregate every event of the window.
func (Store) topUsersByAggregate(ctx context.Context, since time.Time, limit int) ([]domain.TopUser, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.TopUsers(ctx, sqlcgen.TopUsersParams{
		TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, Since: since, RowLimit: int32(limit), //nolint:gosec // small
	})
	if err != nil {
		return nil, fmt.Errorf("top users: %w", err)
	}
	out := make([]domain.TopUser, len(rows))
	for i, r := range rows {
		out[i] = domain.TopUser{User: r.UserRef, MaxScore: int(r.MaxScore), Events: int(r.Events), Flagged: int(r.Flagged), LastSeen: r.LastSeen}
	}
	return out, nil
}
