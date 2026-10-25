package postgres

import "fmt"

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
