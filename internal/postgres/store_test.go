package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"
	"time"

	"github.com/nicheinc/expect"
)

// newTestStore connects to TEST_DATABASE_URL (skips the test when unset),
// runs migrations, and closes the store when the test ends.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	connection := os.Getenv("TEST_DATABASE_URL")
	if connection == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	store, err := NewStore(ctx, connection)
	expect.ErrorNil(t, err)
	t.Cleanup(func() { _ = store.Close() })
	return store
}

// uniqueName returns a username like "t_<8 random hex chars>".
func uniqueName(t *testing.T) string {
	t.Helper()
	raw := make([]byte, 4)
	_, err := rand.Read(raw)
	expect.ErrorNil(t, err)
	return "t_" + hex.EncodeToString(raw)
}
