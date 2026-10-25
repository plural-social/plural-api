package account

import (
	"testing"

	"github.com/nicheinc/expect"
)

func Test_NewSessionToken(t *testing.T) {
	t.Run("Unique", func(t *testing.T) {
		first, _, err := NewSessionToken()
		expect.ErrorNil(t, err)
		second, _, err := NewSessionToken()
		expect.ErrorNil(t, err)
		expect.Equal(t, first == second, false)
	})
	t.Run("HashMatches", func(t *testing.T) {
		token, hash, err := NewSessionToken()
		expect.ErrorNil(t, err)
		expect.Equal(t, hash, HashSessionToken(token))
	})
	t.Run("Length", func(t *testing.T) {
		token, _, err := NewSessionToken()
		expect.ErrorNil(t, err)
		expect.Equal(t, len(token), 43)
	})
}
