package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/httpx"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/service"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/store"
)

func taskToAPI(t store.Task, now time.Time) apigen.ComplianceTask {
	out := apigen.ComplianceTask{
		Id: ids.Format(ids.PrefixComplianceTask, t.ID), Kind: t.Kind, Rule: t.RuleID,
		Status: apigen.ComplianceTaskStatus(t.Status), Sensitive: t.Sensitive, DueAt: t.DueAt,
		Overdue:    t.DueAt != nil && now.After(*t.DueAt) && (t.Status == service.TaskOpen || t.Status == service.TaskApproved),
		SubjectRef: t.SubjectRef, AccountRef: t.AccountRef, Report: t.Report, CreatedAt: t.CreatedAt, CreatedBy: t.CreatedBy, UpdatedAt: t.UpdatedAt,
		ApprovedBy: t.ApprovedBy, ApprovedAt: t.ApprovedAt, ClosedBy: t.ClosedBy, ClosedAt: t.ClosedAt,
		CloseNote: t.CloseNote, FilingRef: t.FilingRef,
	}
	if out.Report == nil {
		out.Report = map[string]any{}
	}
	if t.EvaluationID != nil {
		out.EvaluationId = new(ids.Format(ids.PrefixEvaluation, *t.EvaluationID))
	}
	if t.EventID != nil {
		out.EventId = new(ids.Format(ids.PrefixEvent, *t.EventID))
	}
	if t.CaseID != nil {
		out.CaseId = new(ids.Format(ids.PrefixCase, *t.CaseID))
	}
	return out
}

func (h *Handlers) taskID(raw string) (id [16]byte, err error) {
	u, err := ids.Parse(ids.PrefixComplianceTask, raw)
	if err != nil {
		return id, httpx.NotFound(err)
	}
	return u, nil
}

// ListComplianceTasks implements GET /api/v1/compliance/tasks.
func (h *Handlers) ListComplianceTasks(ctx context.Context, req apigen.ListComplianceTasksRequestObject) (apigen.ListComplianceTasksResponseObject, error) {
	status := ""
	if req.Params.Status != nil {
		status = string(*req.Params.Status)
	}
	now := h.clock.Now()
	q, err := h.svc.Queue(ctx, status, now)
	if err != nil {
		return nil, configError(err)
	}
	out := apigen.ListComplianceTasks200JSONResponse{
		Data: make([]apigen.ComplianceTask, len(q.Tasks)), Escalations: make([]apigen.ScreeningEscalation, len(q.Escalations)),
		UnreportedFraud: make([]apigen.UnreportedFraudCase, len(q.Unreported)),
	}
	for i, t := range q.Tasks {
		out.Data[i] = taskToAPI(t, now)
	}
	for i, e := range q.Escalations {
		out.Escalations[i] = apigen.ScreeningEscalation{
			AlertId: ids.Format(ids.PrefixAlert, e.Alert.ID), Rule: apigen.ScreeningEscalationRule(e.Rule), Title: e.Alert.Title,
			SubjectRef: e.Alert.SubjectRef, Severity: apigen.ScreeningEscalationSeverity(e.Alert.Severity),
			CreatedAt: e.Alert.CreatedAt, DueAt: e.DueAt,
		}
	}
	for i, c := range q.Unreported {
		out.UnreportedFraud[i] = apigen.UnreportedFraudCase{
			CaseId: ids.Format(ids.PrefixCase, c.ID), Title: c.Title, ResolvedAt: c.ResolvedAt,
		}
	}
	return out, nil
}

// GetComplianceTask implements GET /api/v1/compliance/tasks/{task_id}.
func (h *Handlers) GetComplianceTask(ctx context.Context, req apigen.GetComplianceTaskRequestObject) (apigen.GetComplianceTaskResponseObject, error) {
	id, err := h.taskID(req.TaskId)
	if err != nil {
		return nil, err
	}
	t, err := h.svc.Task(ctx, id)
	if err != nil {
		return nil, configError(err)
	}
	return apigen.GetComplianceTask200JSONResponse(taskToAPI(t, h.clock.Now())), nil
}

// EditComplianceReport implements PATCH /api/v1/compliance/tasks/{task_id}/report.
func (h *Handlers) EditComplianceReport(ctx context.Context, req apigen.EditComplianceReportRequestObject) (apigen.EditComplianceReportResponseObject, error) {
	id, err := h.taskID(req.TaskId)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, httpx.InvalidRequest("A request body is required.", nil)
	}
	f := service.ReportFields{PredicateOffence: req.Body.PredicateOffence, Narrative: req.Body.Narrative}
	if req.Body.Category != nil {
		f.Category = new(string(*req.Body.Category))
	}
	if req.Body.Indicators != nil {
		f.Indicators = *req.Body.Indicators
	}
	t, err := h.svc.EditReport(ctx, id, f, h.clock.Now())
	if err != nil {
		return nil, configError(err)
	}
	return apigen.EditComplianceReport200JSONResponse(taskToAPI(t, h.clock.Now())), nil
}

// ApproveComplianceReport implements POST /api/v1/compliance/tasks/{task_id}/approve.
func (h *Handlers) ApproveComplianceReport(ctx context.Context, req apigen.ApproveComplianceReportRequestObject) (apigen.ApproveComplianceReportResponseObject, error) {
	id, err := h.taskID(req.TaskId)
	if err != nil {
		return nil, err
	}
	t, err := h.svc.ApproveReport(ctx, id, h.clock.Now())
	if err != nil {
		return nil, configError(err)
	}
	return apigen.ApproveComplianceReport200JSONResponse(taskToAPI(t, h.clock.Now())), nil
}

// FileComplianceReport implements POST /api/v1/compliance/tasks/{task_id}/file.
func (h *Handlers) FileComplianceReport(ctx context.Context, req apigen.FileComplianceReportRequestObject) (apigen.FileComplianceReportResponseObject, error) {
	id, err := h.taskID(req.TaskId)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, httpx.InvalidRequest("A request body is required.", nil)
	}
	t, err := h.svc.FileReport(ctx, id, req.Body.FilingRef, h.clock.Now())
	if err != nil {
		return nil, configError(err)
	}
	return apigen.FileComplianceReport200JSONResponse(taskToAPI(t, h.clock.Now())), nil
}

// CloseComplianceTask implements POST /api/v1/compliance/tasks/{task_id}/close.
func (h *Handlers) CloseComplianceTask(ctx context.Context, req apigen.CloseComplianceTaskRequestObject) (apigen.CloseComplianceTaskResponseObject, error) {
	id, err := h.taskID(req.TaskId)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, httpx.InvalidRequest("A request body is required.", nil)
	}
	note := ""
	if req.Body.Note != nil {
		note = *req.Body.Note
	}
	t, err := h.svc.CloseTask(ctx, id, string(req.Body.Status), note, h.clock.Now())
	if err != nil {
		return nil, configError(err)
	}
	return apigen.CloseComplianceTask200JSONResponse(taskToAPI(t, h.clock.Now())), nil
}

// ExportComplianceReport implements GET /api/v1/compliance/tasks/{task_id}/export.
func (h *Handlers) ExportComplianceReport(ctx context.Context, req apigen.ExportComplianceReportRequestObject) (apigen.ExportComplianceReportResponseObject, error) {
	id, err := h.taskID(req.TaskId)
	if err != nil {
		return nil, err
	}
	t, err := h.svc.ExportReport(ctx, id)
	if err != nil {
		return nil, configError(err)
	}
	return apigen.ExportComplianceReport200JSONResponse(taskToAPI(t, h.clock.Now())), nil
}

// ConfirmSuspicion implements POST /api/v1/compliance/suspicion.
func (h *Handlers) ConfirmSuspicion(ctx context.Context, req apigen.ConfirmSuspicionRequestObject) (apigen.ConfirmSuspicionResponseObject, error) {
	if req.Body == nil {
		return nil, httpx.InvalidRequest("A request body is required.", nil)
	}
	caseID, err := ids.Parse(ids.PrefixCase, req.Body.CaseId)
	if err != nil {
		return nil, httpx.NewError(http.StatusNotFound, httpx.CodeNotFound, "No such case.", err)
	}
	note := ""
	if req.Body.Note != nil {
		note = *req.Body.Note
	}
	t, err := h.svc.ConfirmSuspicion(ctx, caseID, note, h.clock.Now())
	if err != nil {
		return nil, configError(err)
	}
	return apigen.ConfirmSuspicion201JSONResponse(taskToAPI(t, h.clock.Now())), nil
}

// PrepareFraudReport implements POST /api/v1/compliance/fraud-reports.
func (h *Handlers) PrepareFraudReport(ctx context.Context, req apigen.PrepareFraudReportRequestObject) (apigen.PrepareFraudReportResponseObject, error) {
	if req.Body == nil {
		return nil, httpx.InvalidRequest("A request body is required.", nil)
	}
	caseID, err := ids.Parse(ids.PrefixCase, req.Body.CaseId)
	if err != nil {
		return nil, httpx.NewError(http.StatusNotFound, httpx.CodeNotFound, "No such case.", err)
	}
	t, err := h.svc.PrepareFraudReport(ctx, caseID, h.clock.Now())
	if err != nil {
		return nil, configError(err)
	}
	return apigen.PrepareFraudReport201JSONResponse(taskToAPI(t, h.clock.Now())), nil
}
