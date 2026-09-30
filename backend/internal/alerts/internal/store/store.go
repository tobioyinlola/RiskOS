// Package store is the only code that touches the alerts module's tables. Every query runs in
// the caller's tenant transaction and names tenant and environment explicitly.
package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/tobioyinlola/RiskOS/backend/internal/alerts/internal/domain"
	"github.com/tobioyinlola/RiskOS/backend/internal/alerts/internal/store/sqlcgen"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/db"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/tenancy"
)

// Store accesses alerts.
type Store struct{}

func scoped(ctx context.Context) (*sqlcgen.Queries, tenancy.Scope, error) {
	tx, err := db.TxFrom(ctx)
	if err != nil {
		return nil, tenancy.Scope{}, err
	}
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return nil, tenancy.Scope{}, err
	}
	return sqlcgen.New(tx), scope, nil
}

// LockSubject serializes alert raising for one (kind, subject) in the environment until the
// transaction ends.
func (Store) LockSubject(ctx context.Context, kind domain.Kind, subjectRef string) error {
	q, scope, err := scoped(ctx)
	if err != nil {
		return err
	}
	key := fmt.Sprintf("alerts|%s|%s|%s|%s", scope.TenantID, scope.EnvironmentID, kind, subjectRef)
	if err := q.LockAlertSubject(ctx, key); err != nil {
		return fmt.Errorf("lock alert subject: %w", err)
	}
	return nil
}

// Alerted reports whether the evaluation already joined an alert of kind.
func (Store) Alerted(ctx context.Context, evaluationID uuid.UUID, kind domain.Kind) (bool, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return false, err
	}
	ok, err := q.EvaluationAlerted(ctx, sqlcgen.EvaluationAlertedParams{
		TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, EvaluationID: evaluationID, AlertKind: string(kind),
	})
	if err != nil {
		return false, fmt.Errorf("check evaluation alerted: %w", err)
	}
	return ok, nil
}

// Grouping is the open alert an evaluation joins.
type Grouping struct {
	ID       uuid.UUID
	Severity string
}

// FindGrouping returns the most recent groupable alert for the subject seen since since, locked
// for update, or nil.
func (Store) FindGrouping(ctx context.Context, kind domain.Kind, subjectRef string, since time.Time) (*Grouping, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return nil, err
	}
	row, err := q.FindGroupingAlert(ctx, sqlcgen.FindGroupingAlertParams{
		TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, Kind: string(kind), SubjectRef: subjectRef, Since: since,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil //nolint:nilnil // no grouping alert is a normal outcome
	}
	if err != nil {
		return nil, fmt.Errorf("find grouping alert: %w", err)
	}
	return &Grouping{ID: row.ID, Severity: row.Severity}, nil
}

// InsertAlert creates an open alert from its first evaluation.
func (Store) InsertAlert(ctx context.Context, a domain.Alert) error {
	q, scope, err := scoped(ctx)
	if err != nil {
		return err
	}
	if err := q.InsertAlert(ctx, sqlcgen.InsertAlertParams{
		TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, ID: a.ID, Kind: string(a.Kind),
		SubjectType: a.SubjectType, SubjectRef: a.SubjectRef, Title: a.Title, Severity: a.Severity,
		MaxScore: int16(a.MaxScore), SeenAt: a.FirstSeenAt, Now: a.CreatedAt, //nolint:gosec // score is 0-100
	}); err != nil {
		return fmt.Errorf("insert alert: %w", err)
	}
	return nil
}

// AddToAlert updates a grouping alert's aggregates with one more evaluation.
func (Store) AddToAlert(ctx context.Context, id uuid.UUID, severity, title string, score int, seenAt, now time.Time) error {
	q, scope, err := scoped(ctx)
	if err != nil {
		return err
	}
	if err := q.AddToAlert(ctx, sqlcgen.AddToAlertParams{
		TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, ID: id, Severity: severity, Title: title,
		Score: int16(score), SeenAt: seenAt, Now: now, //nolint:gosec // score is 0-100
	}); err != nil {
		return fmt.Errorf("add to alert: %w", err)
	}
	return nil
}

// LinkEvaluation records that the evaluation joined the alert.
func (Store) LinkEvaluation(ctx context.Context, alertID uuid.UUID, kind domain.Kind, e domain.Evaluation) error {
	q, scope, err := scoped(ctx)
	if err != nil {
		return err
	}
	signals, err := json.Marshal(e.Signals)
	if err != nil {
		return fmt.Errorf("encode signals: %w", err)
	}
	if err := q.InsertAlertEvaluation(ctx, sqlcgen.InsertAlertEvaluationParams{
		TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, AlertID: alertID, AlertKind: string(kind),
		EvaluationID: e.EvaluationID, EventID: e.EventID, EventType: e.EventType,
		Score: int16(e.Score), Level: e.Level, Decision: e.Decision, //nolint:gosec // score is 0-100
		Signals: signals, DegradedSignals: e.Degraded, EvaluatedAt: e.EvaluatedAt,
	}); err != nil {
		return fmt.Errorf("link evaluation to alert: %w", err)
	}
	return nil
}

// Get returns an alert; forUpdate locks it for the rest of the transaction.
func (Store) Get(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.Alert, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return domain.Alert{}, err
	}
	var row sqlcgen.Alert
	if forUpdate {
		row, err = q.GetAlertForUpdate(ctx, sqlcgen.GetAlertForUpdateParams{TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, ID: id})
	} else {
		row, err = q.GetAlert(ctx, sqlcgen.GetAlertParams{TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, ID: id})
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Alert{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Alert{}, fmt.Errorf("get alert: %w", err)
	}
	return toDomain(row), nil
}

// List returns a newest-first page of alerts.
func (Store) List(ctx context.Context, f domain.Filter) ([]domain.Alert, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return nil, err
	}
	p := sqlcgen.ListAlertsParams{
		TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, BeforeID: f.Before, RowLimit: int32(f.Limit), //nolint:gosec // bounded page size
	}
	if f.Status != nil {
		p.Status = new(string(*f.Status))
	}
	if f.Kind != nil {
		p.Kind = new(string(*f.Kind))
	}
	rows, err := q.ListAlerts(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("list alerts: %w", err)
	}
	return mapRows(rows), nil
}

// ListByCase returns the alerts attached to a case.
func (Store) ListByCase(ctx context.Context, caseID uuid.UUID) ([]domain.Alert, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListAlertsByCase(ctx, sqlcgen.ListAlertsByCaseParams{TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, CaseID: &caseID})
	if err != nil {
		return nil, fmt.Errorf("list case alerts: %w", err)
	}
	return mapRows(rows), nil
}

// Evaluations returns the newest evaluations of an alert.
func (Store) Evaluations(ctx context.Context, alertID uuid.UUID) ([]domain.Evaluation, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListAlertEvaluations(ctx, sqlcgen.ListAlertEvaluationsParams{
		TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, AlertID: alertID, RowLimit: domain.MaxEvaluationsShown,
	})
	if err != nil {
		return nil, fmt.Errorf("list alert evaluations: %w", err)
	}
	out := make([]domain.Evaluation, len(rows))
	for i, r := range rows {
		out[i] = domain.Evaluation{
			EvaluationID: r.EvaluationID, EventID: r.EventID, EventType: r.EventType, Score: int(r.Score),
			Level: r.Level, Decision: r.Decision, Degraded: r.DegradedSignals, EvaluatedAt: r.EvaluatedAt,
		}
		if err := json.Unmarshal(r.Signals, &out[i].Signals); err != nil {
			return nil, fmt.Errorf("decode signals: %w", err)
		}
	}
	return out, nil
}

// SetTriage updates status and assignee.
func (Store) SetTriage(ctx context.Context, id uuid.UUID, status domain.Status, assignee *string, now time.Time) error {
	q, scope, err := scoped(ctx)
	if err != nil {
		return err
	}
	if err := q.UpdateAlertTriage(ctx, sqlcgen.UpdateAlertTriageParams{
		TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, ID: id, Status: string(status), Assignee: assignee, Now: now,
	}); err != nil {
		return fmt.Errorf("update alert: %w", err)
	}
	return nil
}

// Escalate attaches an alert to a case.
func (Store) Escalate(ctx context.Context, id, caseID uuid.UUID, now time.Time) error {
	q, scope, err := scoped(ctx)
	if err != nil {
		return err
	}
	if err := q.EscalateAlert(ctx, sqlcgen.EscalateAlertParams{
		TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, ID: id, CaseID: &caseID, Now: now,
	}); err != nil {
		return fmt.Errorf("escalate alert: %w", err)
	}
	return nil
}

// CloseForCase sets the escalated alerts of a case to status and returns their IDs.
func (Store) CloseForCase(ctx context.Context, caseID uuid.UUID, status domain.Status, now time.Time) ([]uuid.UUID, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return nil, err
	}
	closed, err := q.CloseCaseAlerts(ctx, sqlcgen.CloseCaseAlertsParams{
		TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, CaseID: &caseID, Status: string(status), Now: now,
	})
	if err != nil {
		return nil, fmt.Errorf("close case alerts: %w", err)
	}
	return closed, nil
}

// AddActivity appends to an alert's timeline.
func (Store) AddActivity(ctx context.Context, alertID uuid.UUID, action, actor string, detail map[string]any, at time.Time) error {
	q, scope, err := scoped(ctx)
	if err != nil {
		return err
	}
	if detail == nil {
		detail = map[string]any{}
	}
	enc, err := json.Marshal(detail)
	if err != nil {
		return fmt.Errorf("encode activity: %w", err)
	}
	if err := q.InsertAlertActivity(ctx, sqlcgen.InsertAlertActivityParams{
		TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, ID: ids.NewV7(), AlertID: alertID,
		Action: action, Actor: actor, Detail: enc, CreatedAt: at,
	}); err != nil {
		return fmt.Errorf("insert alert activity: %w", err)
	}
	return nil
}

// Activity returns an alert's timeline, oldest first.
func (Store) Activity(ctx context.Context, alertID uuid.UUID) ([]domain.Activity, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListAlertActivity(ctx, sqlcgen.ListAlertActivityParams{TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, AlertID: alertID})
	if err != nil {
		return nil, fmt.Errorf("list alert activity: %w", err)
	}
	out := make([]domain.Activity, len(rows))
	for i, r := range rows {
		out[i] = domain.Activity{ID: r.ID, Action: r.Action, Actor: r.Actor, CreatedAt: r.CreatedAt}
		if err := json.Unmarshal(r.Detail, &out[i].Detail); err != nil {
			return nil, fmt.Errorf("decode activity: %w", err)
		}
	}
	return out, nil
}

func mapRows(rows []sqlcgen.Alert) []domain.Alert {
	out := make([]domain.Alert, len(rows))
	for i, r := range rows {
		out[i] = toDomain(r)
	}
	return out
}

func toDomain(r sqlcgen.Alert) domain.Alert {
	return domain.Alert{
		ID: r.ID, Kind: domain.Kind(r.Kind), SubjectType: r.SubjectType, SubjectRef: r.SubjectRef, Title: r.Title,
		Severity: r.Severity, Status: domain.Status(r.Status), Assignee: r.Assignee, CaseID: r.CaseID,
		MaxScore: int(r.MaxScore), EvaluationCount: int(r.EvaluationCount), FirstSeenAt: r.FirstSeenAt,
		LastSeenAt: r.LastSeenAt, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}
