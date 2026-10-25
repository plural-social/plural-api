package blob

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/nicheinc/expect"
)

// Test_Store needs a running S3-compatible service (make run starts SeaweedFS).
func Test_Store(t *testing.T) {
	endpoint := os.Getenv("TEST_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("TEST_S3_ENDPOINT is not set")
	}
	ctx := context.Background()
	store, err := New(ctx, Config{
		Endpoint: endpoint, AccessKey: "plural", SecretKey: "plural", Bucket: "plural-test", Region: "us-east-1",
	})
	expect.ErrorNil(t, err)

	key := "test/" + uuid.NewString()
	content := []byte("hello blob")
	expect.ErrorNil(t, store.Put(ctx, key, bytes.NewReader(content), int64(len(content)), "text/plain"))

	t.Run("Get", func(t *testing.T) {
		object, err := store.Get(ctx, key)
		expect.ErrorNil(t, err)
		defer object.Close()
		data, err := io.ReadAll(object)
		expect.ErrorNil(t, err)
		expect.Equal(t, data, content)
		expect.Equal(t, object.Size, int64(len(content)))
		expect.Equal(t, object.ContentType, "text/plain")
	})
	t.Run("GetMissing", func(t *testing.T) {
		_, err := store.Get(ctx, "test/missing-"+uuid.NewString())
		expect.ErrorIs(ErrNotFound)(t, err)
	})
	t.Run("Delete", func(t *testing.T) {
		expect.ErrorNil(t, store.Delete(ctx, key))
		_, err := store.Get(ctx, key)
		expect.ErrorIs(ErrNotFound)(t, err)
	})
	t.Run("DeleteMissing", func(t *testing.T) {
		expect.ErrorNil(t, store.Delete(ctx, key))
	})
}
