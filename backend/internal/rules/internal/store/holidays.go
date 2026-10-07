package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/store/sqlcgen"
)

// Holiday is a public holiday an institution entered for a country (working-day clocks).
type Holiday struct {
	Jurisdiction string
	// Day is the date at midnight UTC: a calendar date, not an instant.
	Day       time.Time
	Name      string
	CreatedAt time.Time
	CreatedBy string
}

func date(t time.Time) pgtype.Date {
	return pgtype.Date{Time: time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC), Valid: true}
}

// Holidays returns the tenant's public holidays of one country ("" for every country), by date.
func (Store) Holidays(ctx context.Context, jurisdiction string) ([]Holiday, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return nil, err
	}
	p := sqlcgen.ListHolidaysParams{TenantID: scope.TenantID}
	if jurisdiction != "" {
		p.Jurisdiction = &jurisdiction
	}
	rows, err := q.ListHolidays(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("list holidays: %w", err)
	}
	out := make([]Holiday, len(rows))
	for i, r := range rows {
		out[i] = Holiday{Jurisdiction: r.Jurisdiction, Day: r.Day.Time, Name: r.Name, CreatedAt: r.CreatedAt, CreatedBy: r.CreatedBy}
	}
	return out, nil
}

// InsertHoliday stores a holiday; created is false when the country already has one that day.
func (Store) InsertHoliday(ctx context.Context, h Holiday) (created bool, err error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertHoliday(ctx, sqlcgen.InsertHolidayParams{
		TenantID: scope.TenantID, Jurisdiction: h.Jurisdiction, Day: date(h.Day), Name: h.Name,
		CreatedAt: h.CreatedAt, CreatedBy: h.CreatedBy,
	})
	if err != nil {
		return false, fmt.Errorf("insert holiday: %w", err)
	}
	return n == 1, nil
}

// DeleteHoliday removes a country's holiday on a date; false if there was none.
func (Store) DeleteHoliday(ctx context.Context, jurisdiction string, day time.Time) (bool, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return false, err
	}
	n, err := q.DeleteHoliday(ctx, sqlcgen.DeleteHolidayParams{TenantID: scope.TenantID, Jurisdiction: jurisdiction, Day: date(day)})
	if err != nil {
		return false, fmt.Errorf("delete holiday: %w", err)
	}
	return n == 1, nil
}
