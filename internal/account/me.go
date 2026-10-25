package account

import (
	"time"

	"github.com/google/uuid"
)

// Me is the logged-in user's own account. JSON matches type Me in 01-architecture.md.
type Me struct {
	UUID        uuid.UUID `json:"uuid"`
	Username    string    `json:"username"`
	DisplayName string    `json:"displayName"`
	AvatarURL   string    `json:"avatarUrl,omitzero"`
	Email       string    `json:"email"`
	Bio         string    `json:"bio"`
	Role        string    `json:"role"`
	CreatedAt   time.Time `json:"createdAt"`
}
