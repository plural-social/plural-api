// Package account holds the account domain types. When the monolith is split,
// it becomes the root package of the account service.
package account

import (
	"time"

	"github.com/google/uuid"
)

// Stable, machine-readable error types returned in the "type" field of error
// responses.
const (
	ErrTypeValidation       = "validation"
	ErrTypeUsernameConflict = "usernameConflict"
	ErrTypeEmailConflict    = "emailConflict"
)

type CreateAccountRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

// NewAccount is what gets persisted: a validated request with the password
// already hashed.
type NewAccount struct {
	Username     string
	Email        string
	PasswordHash string
}

type Account struct {
	UUID      uuid.UUID `json:"uuid"`
	Username  string    `json:"username"`
	CreatedAt time.Time `json:"createdAt"`
}
