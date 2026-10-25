package account

import "time"

// Me is the logged-in user's own account. JSON matches type Me in 01-architecture.md.
type Me struct {
	UserSummary
	Email     string    `json:"email"`
	Bio       string    `json:"bio"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"createdAt"`
}
