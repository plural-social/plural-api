package main

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/plural-social/plural-api/internal/account"
	"github.com/plural-social/plural-api/internal/moderation"
	"github.com/plural-social/plural-api/internal/pagination"
	"github.com/plural-social/plural-api/internal/postgres"
)

func (a *app) setUpModerationRoutes(mux *http.ServeMux) {
	mux.HandleFunc("PUT /users/{username}/block", a.endpoint(a.requireViewer(a.putBlockEndpoint)))
	mux.HandleFunc("DELETE /users/{username}/block", a.endpoint(a.requireViewer(a.deleteBlockEndpoint)))
	mux.HandleFunc("GET /me/blocks", a.endpoint(a.requireViewer(a.getBlocksEndpoint)))
	mux.HandleFunc("POST /reports", a.endpoint(a.rateLimited(a.reportLimiter, a.requireViewer(a.postReportEndpoint))))
	mux.HandleFunc("GET /admin/reports", a.endpoint(a.requireModerator(a.getAdminReportsEndpoint)))
	mux.HandleFunc("POST /admin/reports/{uuid}/resolve", a.endpoint(a.requireModerator(a.resolveReportEndpoint)))
	mux.HandleFunc("POST /admin/accounts/{uuid}/unsuspend", a.endpoint(a.requireAdmin(a.unsuspendEndpoint)))
}

// validationError is the 400 response for field failures.
func validationError(failures map[string]any) ErrorResponse {
	return ErrorResponse{Message: "invalid request body", Type: account.ErrTypeValidation, Details: failures}
}

func (a *app) putBlockEndpoint(r *http.Request, viewer account.Viewer) (int, any) {
	blockedUUID, status, body := a.resolveUsername(r)
	if status != 0 {
		return status, body
	}
	if blockedUUID == viewer.UUID {
		return http.StatusUnprocessableEntity, ErrorResponse{Message: "you can't block yourself", Type: moderation.ErrTypeCannotBlockSelf}
	}
	if err := a.moderationStore.BlockAccount(r.Context(), viewer.UUID, blockedUUID); err != nil {
		return a.handleError(err)
	}
	return http.StatusNoContent, nil
}

func (a *app) deleteBlockEndpoint(r *http.Request, viewer account.Viewer) (int, any) {
	blockedUUID, status, body := a.resolveUsername(r)
	if status != 0 {
		return status, body
	}
	if err := a.moderationStore.UnblockAccount(r.Context(), viewer.UUID, blockedUUID); err != nil {
		return a.handleError(err)
	}
	return http.StatusNoContent, nil
}

func (a *app) getBlocksEndpoint(r *http.Request, viewer account.Viewer) (int, any) {
	cursor, limit, errResponse := pageParams(r)
	if errResponse != nil {
		return http.StatusBadRequest, *errResponse
	}
	rows, err := a.moderationStore.ListBlocks(r.Context(), viewer.UUID, cursor, limit)
	if err != nil {
		return a.handleError(err)
	}
	page := pagination.NewPage(rows, limit, func(b moderation.BlockedAccount) pagination.Cursor {
		return pagination.Cursor{CreatedAt: b.BlockedAt, UUID: b.UUID}
	})
	items := make([]account.UserSummary, len(page.Items))
	for i, b := range page.Items {
		items[i] = a.userSummary(r, b.UserSummary)
	}
	return http.StatusOK, pagination.Page[account.UserSummary]{Items: items, NextCursor: page.NextCursor}
}

func (a *app) postReportEndpoint(r *http.Request, viewer account.Viewer) (int, any) {
	ctx := r.Context()

	request, err := decodeJSONBody[moderation.CreateReportRequest](r)
	if err != nil {
		return http.StatusBadRequest, ErrorResponse{Message: err.Error()}
	}
	request.Details = strings.TrimSpace(request.Details)
	targetUUID, failures := validateCreateReportRequest(request)
	if len(failures) > 0 {
		return http.StatusBadRequest, validationError(failures)
	}

	reportable, err := a.isReportable(ctx, viewer, request.TargetType, targetUUID)
	if err != nil {
		return a.handleError(err)
	}
	if !reportable {
		return http.StatusUnprocessableEntity, ErrorResponse{Message: "this can't be reported", Type: moderation.ErrTypeInvalidReportTarget}
	}

	reportUUID, err := a.moderationStore.InsertReport(ctx, moderation.NewReport{
		ReporterUUID: viewer.UUID,
		TargetType:   request.TargetType,
		TargetUUID:   targetUUID,
		Reason:       request.Reason,
		Details:      request.Details,
	})
	if err != nil {
		return a.handleError(err)
	}
	report, err := a.moderationStore.GetReport(ctx, reportUUID)
	if err != nil {
		return a.handleError(err)
	}
	// The preview is for moderators only.
	report.TargetPreview = nil
	report.Reporter = a.userSummary(r, report.Reporter)
	return http.StatusCreated, report
}

func validateCreateReportRequest(request moderation.CreateReportRequest) (uuid.UUID, map[string]any) {
	failures := map[string]any{}
	if !slices.Contains(moderation.TargetTypes, request.TargetType) {
		failures["targetType"] = "must be post, comment or account"
	}
	targetUUID, err := uuid.Parse(request.TargetUUID)
	if err != nil {
		failures["targetUuid"] = "must be a valid UUID"
	}
	if !slices.Contains(moderation.Reasons, request.Reason) {
		failures["reason"] = "must be one of " + strings.Join(moderation.Reasons, ", ")
	}
	if utf8.RuneCountInString(request.Details) > moderation.DetailsMaxLength {
		failures["details"] = "must be at most 1000 characters"
	}
	return targetUUID, failures
}

// isReportable reports whether the target exists, the viewer can see it, and
// it is not the viewer's own. Missing targets are not errors, just false.
func (a *app) isReportable(ctx context.Context, viewer account.Viewer, targetType string, targetUUID uuid.UUID) (bool, error) {
	var (
		owner uuid.UUID
		err   error
	)
	switch targetType {
	case moderation.TargetPost:
		p, getErr := a.postStore.GetPost(ctx, targetUUID, &viewer.UUID)
		owner, err = p.Author.UUID, getErr
	case moderation.TargetComment:
		comment, getErr := a.socialStore.GetComment(ctx, targetUUID, &viewer.UUID)
		owner, err = comment.Author.UUID, getErr
	case moderation.TargetAccount:
		summary, getErr := a.accountStore.GetUserSummary(ctx, targetUUID)
		owner, err = summary.UUID, getErr
	}
	if errors.Is(err, postgres.ErrPostNotFound) || errors.Is(err, postgres.ErrCommentNotFound) || errors.Is(err, postgres.ErrAccountNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return owner != viewer.UUID, nil
}

func (a *app) getAdminReportsEndpoint(r *http.Request, _ account.Viewer) (int, any) {
	status := r.URL.Query().Get("status")
	if status == "" {
		status = moderation.StatusOpen
	}
	if !slices.Contains(moderation.Statuses, status) {
		return http.StatusBadRequest, validationError(map[string]any{"status": "must be open, resolved or dismissed"})
	}
	cursor, limit, errResponse := pageParams(r)
	if errResponse != nil {
		return http.StatusBadRequest, *errResponse
	}

	rows, err := a.moderationStore.ListReports(r.Context(), status, cursor, limit)
	if err != nil {
		return a.handleError(err)
	}
	page := pagination.NewPage(rows, limit, func(report moderation.Report) pagination.Cursor {
		return pagination.Cursor{CreatedAt: report.CreatedAt, UUID: report.UUID}
	})
	for i, report := range page.Items {
		page.Items[i] = a.reportResponse(r, report)
	}
	return http.StatusOK, page
}

// reportResponse builds the reporter avatar and preview thumbnail URLs.
func (a *app) reportResponse(r *http.Request, report moderation.Report) moderation.Report {
	report.Reporter = a.userSummary(r, report.Reporter)
	if report.TargetPreview != nil {
		preview := *report.TargetPreview
		preview.ThumbnailURL = a.fileURL(r, preview.ThumbnailKey)
		report.TargetPreview = &preview
	}
	return report
}

func (a *app) resolveReportEndpoint(r *http.Request, viewer account.Viewer) (int, any) {
	ctx := r.Context()

	reportUUID, ok := parseUUIDPath(r, "uuid")
	if !ok {
		return http.StatusNotFound, errNotFound
	}
	request, err := decodeJSONBody[moderation.ResolveReportRequest](r)
	if err != nil {
		return http.StatusBadRequest, ErrorResponse{Message: err.Error()}
	}
	request.Note = strings.TrimSpace(request.Note)
	if failures := validateResolveReportRequest(request); len(failures) > 0 {
		return http.StatusBadRequest, validationError(failures)
	}

	report, err := a.moderationStore.GetReport(ctx, reportUUID)
	if err != nil {
		return a.handleError(err)
	}
	if report.Status != moderation.StatusOpen {
		return http.StatusConflict, ErrorResponse{Message: "this report is already closed", Type: moderation.ErrTypeReportAlreadyResolved}
	}

	resolution := moderation.Resolution{
		TargetType:    report.TargetType,
		TargetUUID:    report.TargetUUID,
		ModeratorUUID: viewer.UUID,
		Status:        moderation.StatusResolved,
		Note:          request.Note,
	}
	switch request.Action {
	case moderation.ActionDismiss:
		// Dismissing closes this report only; others on the same target stay open.
		resolution.Status = moderation.StatusDismissed
		resolution.OnlyReportUUID = &report.UUID
	case moderation.ActionRemoveContent:
		if status, body := a.removeContent(ctx, report); status != 0 {
			return status, body
		}
	case moderation.ActionSuspendAccount:
		if status, body := a.suspendTargetAccount(ctx, viewer, report); status != 0 {
			return status, body
		}
	}

	if err := a.moderationStore.ResolveReports(ctx, resolution); err != nil {
		return a.handleError(err)
	}
	resolved, err := a.moderationStore.GetReport(ctx, reportUUID)
	if err != nil {
		return a.handleError(err)
	}
	return http.StatusOK, a.reportResponse(r, resolved)
}

func validateResolveReportRequest(request moderation.ResolveReportRequest) map[string]any {
	failures := map[string]any{}
	if !slices.Contains(moderation.Actions, request.Action) {
		failures["action"] = "must be dismiss, removeContent or suspendAccount"
	}
	if utf8.RuneCountInString(request.Note) > moderation.NoteMaxLength {
		failures["note"] = "must be at most 1000 characters"
	}
	return failures
}

// removeContent hides the reported post or comment. Content that is already
// gone counts as removed. A non-zero status is an error response to return.
func (a *app) removeContent(ctx context.Context, report moderation.Report) (int, any) {
	var err error
	switch report.TargetType {
	case moderation.TargetPost:
		m, removeErr := a.moderationStore.RemovePost(ctx, report.TargetUUID)
		if removeErr == nil {
			a.deleteMediaFiles(ctx, m)
		}
		err = removeErr
	case moderation.TargetComment:
		err = a.moderationStore.SoftDeleteComment(ctx, report.TargetUUID)
	default:
		return http.StatusUnprocessableEntity, ErrorResponse{Message: "use suspendAccount for accounts", Type: moderation.ErrTypeInvalidAction}
	}
	if err != nil && !errors.Is(err, postgres.ErrPostNotFound) && !errors.Is(err, postgres.ErrCommentNotFound) {
		return a.handleError(err)
	}
	return 0, nil
}

// suspendTargetAccount suspends the reported account, or the author of the
// reported post or comment. Nobody suspends an admin through the API, and only
// admins suspend moderators. A non-zero status is an error response to return.
func (a *app) suspendTargetAccount(ctx context.Context, viewer account.Viewer, report moderation.Report) (int, any) {
	accountUUID := report.TargetUUID
	if report.TargetType != moderation.TargetAccount {
		author, err := a.moderationStore.GetContentAuthor(ctx, report.TargetType, report.TargetUUID)
		if err != nil {
			return a.handleError(err)
		}
		accountUUID = author
	}

	role, err := a.accountStore.GetAccountRole(ctx, accountUUID)
	if err != nil {
		return a.handleError(err)
	}
	if role == account.RoleAdmin || (role == account.RoleModerator && viewer.Role != account.RoleAdmin) {
		return http.StatusForbidden, errForbidden
	}

	if err := a.moderationStore.SuspendAccount(ctx, accountUUID); err != nil {
		return a.handleError(err)
	}
	return 0, nil
}

func (a *app) unsuspendEndpoint(r *http.Request, _ account.Viewer) (int, any) {
	accountUUID, ok := parseUUIDPath(r, "uuid")
	if !ok {
		return http.StatusNotFound, errNotFound
	}
	if err := a.moderationStore.UnsuspendAccount(r.Context(), accountUUID); err != nil {
		return a.handleError(err)
	}
	return http.StatusNoContent, nil
}
