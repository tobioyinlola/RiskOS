package service

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/tobioyinlola/RiskOS/backend/internal/audit"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/pack"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/store"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/workdays"
)

// Working-day calendars (ADR-0007, phase 5): weekends and the public holidays the institution
// entered for each country it operates in.

// holidaysByJurisdiction returns the tenant's public holidays, by country then date. It must
// run inside the runner.
func (s *Service) holidaysByJurisdiction(ctx context.Context) (map[string]map[string]bool, error) {
	list, err := s.store.Holidays(ctx, "")
	if err != nil {
		return nil, err
	}
	out := map[string]map[string]bool{}
	for _, h := range list {
		if out[h.Jurisdiction] == nil {
			out[h.Jurisdiction] = map[string]bool{}
		}
		out[h.Jurisdiction][h.Day.Format(workdays.DateLayout)] = true
	}
	return out, nil
}

// calendar returns a pack's working-day calendar for the tenant. It must run inside the runner.
func (s *Service) calendar(ctx context.Context, p *pack.Pack) (workdays.Calendar, error) {
	loc, err := time.LoadLocation(p.Locale().Timezone)
	if err != nil {
		return workdays.Calendar{}, fmt.Errorf("pack %s: %w", p.ID, err)
	}
	list, err := s.store.Holidays(ctx, p.Jurisdiction)
	if err != nil {
		return workdays.Calendar{}, err
	}
	days := make(map[string]bool, len(list))
	for _, h := range list {
		days[h.Day.Format(workdays.DateLayout)] = true
	}
	return workdays.Calendar{Location: loc, Holidays: days}, nil
}

// knownJurisdiction checks that RiskOS has a pack for a country.
func (s *Service) knownJurisdiction(ctx context.Context, code string) error {
	list, err := s.store.Jurisdictions(ctx)
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(list, func(j store.Jurisdiction) bool { return j.Code == code }) {
		return fmt.Errorf("%w: jurisdiction %s", ErrNotFound, code)
	}
	return nil
}

// Holidays lists the public holidays the tenant entered for a country ("" for every country).
// Requires compliance:read.
func (s *Service) Holidays(ctx context.Context, jurisdiction string) ([]store.Holiday, error) {
	if _, err := readScope(ctx); err != nil {
		return nil, err
	}
	var out []store.Holiday
	err := s.runner.Run(ctx, func(ctx context.Context) error {
		var err error
		out, err = s.store.Holidays(ctx, jurisdiction)
		return err
	})
	return out, err
}

// AddHoliday records a public holiday in a country: working-day clocks there skip it from the
// next evaluation. Audited. Requires compliance:configure (people only).
func (s *Service) AddHoliday(ctx context.Context, jurisdiction string, day time.Time, name string, now time.Time) (store.Holiday, error) {
	scope, err := person(ctx, authz.ComplianceConfigure)
	if err != nil {
		return store.Holiday{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 100 {
		return store.Holiday{}, fmt.Errorf("%w: a name of 1 to 100 characters is required", ErrInvalidInput)
	}
	// Holidays matter to holds running now or soon: a year back, three ahead. Every pack compile
	// reads them, and a far-off date could only stretch a hold.
	if day.Before(now.AddDate(-1, 0, -1)) || day.After(now.AddDate(3, 0, 0)) {
		return store.Holiday{}, fmt.Errorf("%w: the date must be within a year back and three years ahead", ErrInvalidInput)
	}
	h := store.Holiday{Jurisdiction: jurisdiction, Day: day, Name: name, CreatedAt: now, CreatedBy: scope.Principal.ID}
	err = s.runner.Run(ctx, func(ctx context.Context) error {
		if err := s.knownJurisdiction(ctx, jurisdiction); err != nil {
			return err
		}
		created, err := s.store.InsertHoliday(ctx, h)
		if err != nil {
			return err
		}
		if !created {
			return fmt.Errorf("%w: %s already has a holiday on %s", ErrConflict, jurisdiction, day.Format(workdays.DateLayout))
		}
		return s.recordHoliday(ctx, "compliance_holiday.added", h)
	})
	return h, err
}

// DeleteHoliday removes a public holiday entered by mistake. Audited. Requires
// compliance:configure (people only).
func (s *Service) DeleteHoliday(ctx context.Context, jurisdiction string, day time.Time) error {
	if _, err := person(ctx, authz.ComplianceConfigure); err != nil {
		return err
	}
	return s.runner.Run(ctx, func(ctx context.Context) error {
		deleted, err := s.store.DeleteHoliday(ctx, jurisdiction, day)
		if err != nil {
			return err
		}
		if !deleted {
			return fmt.Errorf("%w: holiday", ErrNotFound)
		}
		return s.recordHoliday(ctx, "compliance_holiday.deleted", store.Holiday{Jurisdiction: jurisdiction, Day: day})
	})
}

func (s *Service) recordHoliday(ctx context.Context, action string, h store.Holiday) error {
	meta := map[string]any{"jurisdiction": h.Jurisdiction, "day": h.Day.Format(workdays.DateLayout)}
	if h.Name != "" {
		meta["name"] = h.Name
	}
	return s.audit.Record(ctx, audit.Entry{
		Action: action, ResourceType: "compliance_holiday", ResourceID: h.Jurisdiction + ":" + h.Day.Format(workdays.DateLayout),
		Outcome: audit.OutcomeSuccess, TenantLevel: true, Metadata: meta,
	})
}

// holidayCheck is the readiness warning of a pack with working-day clocks and no public holiday
// entered for the coming year: its holds would end too early on a holiday.
func holidayCheck(p *pack.Pack, configured map[string]bool, holidays []store.Holiday, now time.Time) *ReadinessItem {
	var rules []string
	for i := range p.Rules {
		def := &p.Rules[i]
		if !configured[def.ID] {
			continue
		}
		if slices.ContainsFunc(def.Parameters, func(prm pack.Parameter) bool { return prm.Type == pack.ParamWorkingDays }) {
			rules = append(rules, def.ID)
		}
	}
	if len(rules) == 0 {
		return nil
	}
	horizon := now.AddDate(1, 0, 0)
	if slices.ContainsFunc(holidays, func(h store.Holiday) bool { return !h.Day.Before(now.AddDate(0, 0, -1)) && h.Day.Before(horizon) }) {
		return nil
	}
	return &ReadinessItem{
		Kind: "holidays", Rule: rules[0],
		Message: fmt.Sprintf("%s count working days: enter %s's public holidays for the coming year, or holds end too early on a holiday",
			strings.Join(rules, ", "), p.Name),
	}
}

// ClockLoad is how many tasks run against one of a country's clocks now.
type ClockLoad struct {
	Jurisdiction string
	Clock        pack.Clock
	store.Load
}

// Load is the live load of every clock of the countries the tenant has activated, in the
// caller's environment: tasks running (open or approved), past their deadline, and due within a
// day; and the running tasks of rules no clock lists. Suspicious transaction reports count only
// for compliance:sensitive (SensitiveHidden says when they were left out).
type Load struct {
	Clocks          []ClockLoad
	Unclocked       []store.Load
	SensitiveHidden bool
}

// ClockLoad counts the compliance work running against each clock. Requires compliance:read.
func (s *Service) ClockLoad(ctx context.Context, now time.Time) (Load, error) {
	scope, err := readScope(ctx)
	if err != nil {
		return Load{}, err
	}
	sensitive := authz.Require(ctx, authz.ComplianceSensitive) == nil
	out := Load{SensitiveHidden: !sensitive}
	err = s.runner.Run(ctx, func(ctx context.Context) error {
		counts, err := s.store.TaskLoad(ctx, now, sensitive)
		if err != nil {
			return err
		}
		acts, err := s.store.Activations(ctx, scope.TenantID)
		if err != nil {
			return err
		}
		codes := map[string]bool{}
		for _, a := range acts {
			codes[a.Jurisdiction] = true
		}
		for _, c := range counts {
			codes[c.Jurisdiction] = true
		}
		clocked := map[string]bool{}
		for _, code := range slices.Sorted(maps.Keys(codes)) {
			lib, err := s.load(ctx, scope, code)
			if errors.Is(err, ErrNotFound) {
				continue
			}
			if err != nil {
				return err
			}
			for _, c := range lib.pack.Clocks {
				row := ClockLoad{Jurisdiction: code, Clock: c, Load: store.Load{Jurisdiction: code}}
				for _, n := range counts {
					if n.Jurisdiction == code && slices.Contains(c.Rules, n.RuleID) {
						row.Running, row.Overdue, row.DueSoon = row.Running+n.Running, row.Overdue+n.Overdue, row.DueSoon+n.DueSoon
						clocked[code+"|"+n.RuleID] = true
					}
				}
				out.Clocks = append(out.Clocks, row)
			}
		}
		for _, n := range counts {
			if !clocked[n.Jurisdiction+"|"+n.RuleID] {
				out.Unclocked = append(out.Unclocked, n)
			}
		}
		return nil
	})
	return out, err
}

// CrossBorderEvent is an event that started a clock in a country other than the one that
// governed it (a Rwandan data subject of a Ghana-booked breach), with the tasks it started.
type CrossBorderEvent struct {
	EventID uuid.UUID
	// BookedIn is the country that governed the event; Countries every country it started a
	// clock in.
	BookedIn  string
	Countries []string
	Tasks     []store.Task
}

// crossBorderLimit bounds the events the cross-border view lists.
const crossBorderLimit = 100

// CrossBorder lists the latest events that started a clock outside their governing country,
// newest first, in the caller's environment. Suspicious transaction reports (and events known
// only through one) only for compliance:sensitive. Requires compliance:read.
func (s *Service) CrossBorder(ctx context.Context) ([]CrossBorderEvent, error) {
	if _, err := readScope(ctx); err != nil {
		return nil, err
	}
	sensitive := authz.Require(ctx, authz.ComplianceSensitive) == nil
	var out []CrossBorderEvent
	err := s.runner.Run(ctx, func(ctx context.Context) error {
		tasks, err := s.store.CrossBorderTasks(ctx, crossBorderLimit, sensitive)
		if err != nil {
			return err
		}
		index := map[uuid.UUID]int{}
		for _, t := range tasks {
			if t.EventID == nil {
				continue
			}
			i, ok := index[*t.EventID]
			if !ok {
				i = len(out)
				index[*t.EventID] = i
				out = append(out, CrossBorderEvent{EventID: *t.EventID})
			}
			e := &out[i]
			if t.EventJurisdiction != nil {
				e.BookedIn = *t.EventJurisdiction
			}
			if !slices.Contains(e.Countries, t.Jurisdiction) {
				e.Countries = append(e.Countries, t.Jurisdiction)
			}
			e.Tasks = append(e.Tasks, t)
		}
		for i := range out {
			if !slices.Contains(out[i].Countries, out[i].BookedIn) && out[i].BookedIn != "" {
				out[i].Countries = append(out[i].Countries, out[i].BookedIn)
			}
			slices.Sort(out[i].Countries)
		}
		return nil
	})
	return out, err
}
