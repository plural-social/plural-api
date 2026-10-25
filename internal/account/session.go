package account

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/google/uuid"
)

const (
	SessionLifetime = 90 * 24 * time.Hour

	RoleUser = "user"
)

const (
	ErrTypeUnauthenticated    = "unauthenticated"
	ErrTypeInvalidCredentials = "invalidCredentials"
	ErrTypeRateLimited        = "rateLimited"
)

// NewSessionToken returns a random token for the client and the SHA-256 hash
// to store. The token is 32 bytes from crypto/rand, base64url without padding.
func NewSessionToken() (token string, tokenHash []byte, err error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, fmt.Errorf("generating session token: %w", err)
	}
	token = base64.RawURLEncoding.EncodeToString(raw)
	return token, HashSessionToken(token), nil
}

// HashSessionToken returns the SHA-256 of the token string.
func HashSessionToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// Viewer is the authenticated account making a request.
type Viewer struct {
	UUID     uuid.UUID
	Username string
	Role     string // always RoleUser until F13
}

type NewSession struct {
	TokenHash   []byte
	AccountUUID uuid.UUID
	UserAgent   string
	ExpiresAt   time.Time
}

type Credentials struct {
	AccountUUID  uuid.UUID
	PasswordHash string
}

type CreateSessionRequest struct {
	Login    string `json:"login"` // username or email
	Password string `json:"password"`
}

type SessionResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expiresAt"`
	Me        Me        `json:"me"`
}
