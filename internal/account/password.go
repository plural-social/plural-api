package account

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"

	"golang.org/x/crypto/argon2"
)

// argon2id parameters from RFC 9106 §4 (second recommended option).
const (
	argonTime    = 3
	argonMemory  = 64 * 1024
	argonThreads = 4
	argonKeyLen  = 32
	saltLen      = 16
)

var (
	ErrInvalidPasswordHash = errors.New("invalid password hash")

	b64 = base64.RawStdEncoding
)

// HashPassword returns a PHC-format string:
// $argon2id$v=19$m=<memory>,t=<time>,p=<threads>$<salt>$<key>
func HashPassword(password string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generating salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		b64.EncodeToString(salt), b64.EncodeToString(key),
	), nil
}

// VerifyPassword uses the parameters stored in the hash, so existing hashes
// keep working if the defaults above change.
func VerifyPassword(password, encodedHash string) (bool, error) {
	// "", "argon2id", "v=19", "m=..,t=..,p=..", salt, key
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, ErrInvalidPasswordHash
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false, ErrInvalidPasswordHash
	}

	var (
		memory, time uint32
		threads      uint8
	)
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &time, &threads); err != nil {
		return false, ErrInvalidPasswordHash
	}

	salt, saltErr := b64.DecodeString(parts[4])
	expected, keyErr := b64.DecodeString(parts[5])
	if saltErr != nil || keyErr != nil || len(expected) == 0 {
		return false, ErrInvalidPasswordHash
	}

	actual := argon2.IDKey([]byte(password), salt, time, memory, threads, uint32(len(expected)))
	return subtle.ConstantTimeCompare(actual, expected) == 1, nil
}

// dummyPasswordHash is verified when a login name does not exist, so a missing
// account takes as long to reject as a wrong password.
var dummyPasswordHash = sync.OnceValues(func() (string, error) {
	return HashPassword("plural-timing-equalizer")
})

// VerifyDummyPassword spends the same time as VerifyPassword and returns nothing useful.
func VerifyDummyPassword(password string) {
	hash, err := dummyPasswordHash()
	if err != nil {
		return
	}
	_, _ = VerifyPassword(password, hash)
}
