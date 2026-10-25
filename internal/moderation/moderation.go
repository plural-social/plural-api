// Package moderation holds blocks, reports and moderator actions.
package moderation

import (
	"time"

	"github.com/google/uuid"

	"github.com/plural-social/plural-api/internal/account"
)

const (
	TargetPost    = "post"
	TargetComment = "comment"
	TargetAccount = "account"

	StatusOpen      = "open"
	StatusResolved  = "resolved"
	StatusDismissed = "dismissed"

	ActionDismiss        = "dismiss"
	ActionRemoveContent  = "removeContent"
	ActionSuspendAccount = "suspendAccount"

	DetailsMaxLength = 1000
	NoteMaxLength    = 1000

	ErrTypeCannotBlockSelf       = "cannotBlockSelf"
	ErrTypeBlocked               = "blocked"
	ErrTypeInvalidReportTarget   = "invalidReportTarget"
	ErrTypeReportAlreadyResolved = "reportAlreadyResolved"
	ErrTypeInvalidAction         = "invalidAction"
)

var (
	Reasons     = []string{"spam", "harassment", "nudity", "violence", "illegal", "other"}
	TargetTypes = []string{TargetPost, TargetComment, TargetAccount}
	Statuses    = []string{StatusOpen, StatusResolved, StatusDismissed}
	Actions     = []string{ActionDismiss, ActionRemoveContent, ActionSuspendAccount}
)

type CreateReportRequest struct {
	TargetType string `json:"targetType"`
	TargetUUID string `json:"targetUuid"`
	Reason     string `json:"reason"`
	Details    string `json:"details"`
}

type NewReport struct {
	ReporterUUID uuid.UUID
	TargetType   string
	TargetUUID   uuid.UUID
	Reason       string
	Details      string
}

// TargetPreview shows moderators what was reported, even if it was since hidden.
type TargetPreview struct {
	Username     string `json:"username"`
	Text         string `json:"text"`
	ThumbnailKey string `json:"-"`
	ThumbnailURL string `json:"thumbnailUrl,omitzero"`
}

// Report matches type Report in 01-architecture.md.
type Report struct {
	UUID           uuid.UUID           `json:"uuid"`
	Reporter       account.UserSummary `json:"reporter"`
	TargetType     string              `json:"targetType"`
	TargetUUID     uuid.UUID           `json:"targetUuid"`
	Reason         string              `json:"reason"`
	Details        string              `json:"details"`
	Status         string              `json:"status"`
	ResolutionNote string              `json:"resolutionNote"`
	CreatedAt      time.Time           `json:"createdAt"`
	ResolvedAt     *time.Time          `json:"resolvedAt,omitzero"`
	TargetPreview  *TargetPreview      `json:"targetPreview,omitzero"`
}

type ResolveReportRequest struct {
	Action string `json:"action"`
	Note   string `json:"note"`
}

type Resolution struct {
	TargetType    string
	TargetUUID    uuid.UUID
	ModeratorUUID uuid.UUID
	Status        string
	Note          string
	// When set, only this report is updated. When nil, every open report on
	// the same target is updated.
	OnlyReportUUID *uuid.UUID
}

// BlockedAccount is an entry in the viewer's block list.
type BlockedAccount struct {
	account.UserSummary
	BlockedAt time.Time
}
