package postgres

import (
	"errors"
	"fmt"
)

var (
	ErrAccountNotFound = errors.New("account not found")
	ErrSessionNotFound = errors.New("session not found")
	ErrMediaNotFound   = errors.New("media not found")
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
