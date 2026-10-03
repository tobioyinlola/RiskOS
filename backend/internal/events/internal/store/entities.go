package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

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

// Timeline counts events per bucket since since.
func (Store) Timeline(ctx context.Context, since time.Time, bucket time.Duration) ([]domain.Point, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.EventTimeline(ctx, sqlcgen.EventTimelineParams{
		Step: pgtype.Interval{Microseconds: bucket.Microseconds(), Valid: true}, Origin: since, TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, Since: since,
	})
	if err != nil {
		return nil, fmt.Errorf("event timeline: %w", err)
	}
	out := make([]domain.Point, len(rows))
	for i, r := range rows {
		out[i] = domain.Point{
			At: r.Bucket, Events: int(r.Events), MeanScore: int(r.MeanScore),
			Minimal: int(r.Minimal), Low: int(r.Low), Elevated: int(r.Elevated), High: int(r.High), Critical: int(r.Critical),
		}
		if err := json.Unmarshal(r.Decisions, &out[i].Decisions); err != nil {
			return nil, fmt.Errorf("decode decision counts: %w", err)
		}
	}
	return out, nil
}

// TopUsers ranks users by their highest score since since.
func (Store) TopUsers(ctx context.Context, since time.Time, limit int) ([]domain.TopUser, error) {
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
