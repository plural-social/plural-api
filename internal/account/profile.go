package account

import (
	"time"

	"github.com/google/uuid"
)

const (
	DisplayNameMaxLength = 50
	BioMaxLength         = 300

	ErrTypeInvalidAvatarMedia = "invalidAvatarMedia"
)

// UserSummary is the short form of a user used inside posts, comments and lists.
type UserSummary struct {
	UUID        uuid.UUID `json:"uuid"`
	Username    string    `json:"username"`
	DisplayName string    `json:"displayName"`
	AvatarURL   string    `json:"avatarUrl,omitzero"`
	// AvatarKey is the storage key of the avatar thumbnail. The store fills it;
	// handlers turn it into AvatarURL (see app.userSummary).
	AvatarKey string `json:"-"`
}

// Profile is the public view of a user. UserSummary is embedded so its fields
// appear at the top level of the JSON.
type Profile struct {
	UserSummary
	Bio            string    `json:"bio"`
	FollowerCount  int       `json:"followerCount"`
	FollowingCount int       `json:"followingCount"`
	PostCount      int       `json:"postCount"`
	IsFollowing    bool      `json:"isFollowing"`
	IsBlocked      bool      `json:"isBlocked"`
	IsSelf         bool      `json:"isSelf"`
	CreatedAt      time.Time `json:"createdAt"`
}

// UpdateMeRequest fields are optional. A nil field is left unchanged.
type UpdateMeRequest struct {
	DisplayName *string `json:"displayName,omitzero"`
	Bio         *string `json:"bio,omitzero"`
}

// ProfileUpdate is the normalized update passed to the store.
type ProfileUpdate struct {
	DisplayName *string
	Bio         *string
}

// DeleteMeRequest asks for the password again: deleting an account cannot be
// undone, and a stolen session token should not be enough to do it.
type DeleteMeRequest struct {
	Password string `json:"password"`
}

type SetAvatarRequest struct {
	MediaUUID string `json:"mediaUuid"`
}
