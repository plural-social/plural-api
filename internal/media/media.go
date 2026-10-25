// Package media holds the media domain: uploaded images and videos, their
// storage keys and their processing. A media item is created by an upload and
// later attached to a post or used as an avatar.
package media

import (
	"time"

	"github.com/google/uuid"
)

const (
	KindImage = "image"
	KindVideo = "video"

	StatusProcessing = "processing"
	StatusReady      = "ready"
	StatusFailed     = "failed"

	ImageMaxBytes = 20 << 20

	VideoMaxBytes      = 300 << 20
	VideoMinDurationMS = 1_000
	VideoMaxDurationMS = 180_000
	// videoDurationToleranceMS lets a "3:00" clip that is really 180.4 s through.
	videoDurationToleranceMS = 500
	videoMaxSide             = 7680

	ErrTypeUnsupportedMediaType = "unsupportedMediaType"
	ErrTypeMediaTooLarge        = "mediaTooLarge"
	ErrTypeInvalidMedia         = "invalidMedia"
)

// ImageContentTypes and VideoContentTypes are the accepted upload types.
var (
	ImageContentTypes = []string{"image/jpeg", "image/png", "image/webp"}
	VideoContentTypes = []string{"video/mp4", "video/quicktime", "video/webm"}
)

func DisplayKey(mediaUUID uuid.UUID) string {
	return "media/" + mediaUUID.String() + "/display.jpg"
}

func ThumbnailKey(mediaUUID uuid.UUID) string {
	return "media/" + mediaUUID.String() + "/thumb.jpg"
}

// OriginalKey is the uploaded video before transcoding. It is never served.
func OriginalKey(mediaUUID uuid.UUID) string {
	return "media/" + mediaUUID.String() + "/original"
}

func VideoKey(mediaUUID uuid.UUID) string {
	return "media/" + mediaUUID.String() + "/video.mp4"
}

func PosterKey(mediaUUID uuid.UUID) string {
	return "media/" + mediaUUID.String() + "/poster.jpg"
}

// Job is a claimed media_job row.
type Job struct {
	UUID      uuid.UUID
	MediaUUID uuid.UUID
	Attempts  int // after this claim
}

// VideoResult is what the transcoder stores when a video is ready.
type VideoResult struct {
	DisplayKey   string
	ThumbnailKey string
	Width        int
	Height       int
	DurationMS   int
	SizeBytes    int64
}

// Media is a stored media row.
type Media struct {
	UUID          uuid.UUID
	AccountUUID   uuid.UUID
	Kind          string
	Status        string
	ContentType   string
	DisplayKey    string
	ThumbnailKey  string
	OriginalKey   string
	Width         int
	Height        int
	DurationMS    int
	SizeBytes     int64
	FailureReason string
	CreatedAt     time.Time
}

// NewMedia is the row to insert. UUID is chosen by the caller so storage keys
// can be written before the row exists.
type NewMedia struct {
	UUID          uuid.UUID
	AccountUUID   uuid.UUID
	Kind          string
	Status        string
	ContentType   string
	DisplayKey    string
	ThumbnailKey  string
	OriginalKey   string
	Width         int
	Height        int
	DurationMS    int
	SizeBytes     int64
	FailureReason string
}

// Response is the JSON shape (type Media in 01-architecture.md).
type Response struct {
	UUID          uuid.UUID `json:"uuid"`
	Kind          string    `json:"kind"`
	Status        string    `json:"status"`
	URL           string    `json:"url,omitzero"`
	ThumbnailURL  string    `json:"thumbnailUrl,omitzero"`
	Width         int       `json:"width"`
	Height        int       `json:"height"`
	DurationMS    int       `json:"durationMs"`
	FailureReason string    `json:"failureReason,omitzero"`
}
