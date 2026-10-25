package postgres

import (
	"errors"
	"fmt"
)

var (
	ErrAccountNotFound = errors.New("account not found")
	ErrSessionNotFound = errors.New("session not found")
	ErrMediaNotFound   = errors.New("media not found")
	ErrPostNotFound    = errors.New("post not found")
	ErrCommentNotFound = errors.New("comment not found")

	ErrInvalidAvatarMedia = errors.New("invalid avatar media")

	ErrMediaAlreadyUsed = errors.New("media already used by another post")
)

type UsernameConflictError struct {
	Username string
}

func (e UsernameConflictError) Error() string {
	return fmt.Sprintf("username '%s' is already taken", e.Username)
}

// EmailConflictError deliberately omits the email so it is safe to log.
type EmailConflictError struct{}

func (e EmailConflictError) Error() string {
	return "email is already registered"
}
