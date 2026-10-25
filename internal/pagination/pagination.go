package pagination

import (
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	DefaultLimit = 10
	MaxLimit     = 30

	ErrTypeInvalidPagination = "invalidPagination"
)

var (
	ErrInvalidCursor = errors.New("invalid cursor")
	ErrInvalidLimit  = errors.New("invalid limit")
)

// Cursor marks the last item of a page. The next page starts strictly after it
// in (created_at DESC, uuid DESC) order.
type Cursor struct {
	CreatedAt time.Time
	UUID      uuid.UUID
}

// IsZero reports whether the cursor is empty (first page).
func (c Cursor) IsZero() bool {
	return c.CreatedAt.IsZero() && c.UUID == uuid.Nil
}

// Encode returns base64url (no padding) of "<RFC3339Nano UTC>|<uuid>".
func Encode(c Cursor) string {
	raw := c.CreatedAt.UTC().Format(time.RFC3339Nano) + "|" + c.UUID.String()
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// Decode reverses Encode. An empty string returns a zero Cursor and nil error.
func Decode(s string) (Cursor, error) {
	if s == "" {
		return Cursor{}, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return Cursor{}, ErrInvalidCursor
	}
	createdAt, id, found := strings.Cut(string(raw), "|")
	if !found {
		return Cursor{}, ErrInvalidCursor
	}
	parsedTime, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return Cursor{}, ErrInvalidCursor
	}
	parsedUUID, err := uuid.Parse(id)
	if err != nil {
		return Cursor{}, ErrInvalidCursor
	}
	return Cursor{CreatedAt: parsedTime.UTC(), UUID: parsedUUID}, nil
}

// ParseLimit parses the ?limit= value. Empty returns DefaultLimit.
func ParseLimit(s string) (int, error) {
	if s == "" {
		return DefaultLimit, nil
	}
	limit, err := strconv.Atoi(s)
	if err != nil || limit < 1 || limit > MaxLimit {
		return 0, ErrInvalidLimit
	}
	return limit, nil
}

// Page is the JSON shape of every list response.
type Page[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"nextCursor,omitzero"`
}

// NewPage builds a page from up to limit+1 rows. If there are more than limit
// rows, it drops the extra row and sets NextCursor from the last kept row.
func NewPage[T any](rows []T, limit int, cursorOf func(T) Cursor) Page[T] {
	if len(rows) <= limit {
		return Page[T]{Items: append([]T{}, rows...)}
	}
	items := append([]T{}, rows[:limit]...)
	return Page[T]{Items: items, NextCursor: Encode(cursorOf(items[limit-1]))}
}
