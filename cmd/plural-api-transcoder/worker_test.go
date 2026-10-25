package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nicheinc/expect"
	"go.uber.org/zap"

	"github.com/plural-social/plural-api/internal/blob"
	"github.com/plural-social/plural-api/internal/media"
)

var (
	testNow       = time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	testJobUUID   = uuid.MustParse("11111111-1111-4111-8111-111111111111")
	testMediaUUID = uuid.MustParse("22222222-2222-4222-8222-222222222222")
	testOriginal  = media.OriginalKey(testMediaUUID)
	inputProbe    = media.ProbeResult{DurationMS: 3000, Width: 1080, Height: 1920, HasAudio: true}
	outputProbe   = media.ProbeResult{DurationMS: 3010, Width: 720, Height: 1280, HasAudio: true}
)

// readSeekCloser wraps a bytes.Reader as a blob object body.
type readSeekCloser struct{ *bytes.Reader }

func (readSeekCloser) Close() error { return nil }

// outcome records what the worker told the store and blob store.
type outcome struct {
	retryRunAfter *time.Time
	failReason    string
	completed     *media.VideoResult
	puts          map[string]string // key → content type
	deleted       []string
}

func Test_worker_process(t *testing.T) {
	type testCase struct {
		attempts int
		// Each hook replaces one step; nil uses the happy path.
		getMedia  func() (media.Media, error)
		getBlob   func() (*blob.Object, error)
		probe     func(path string) (media.ProbeResult, error)
		transcode func() error
		put       func(key string) error
		complete  func() error
		expected  func(t *testing.T, got outcome)
	}
	run := func(name string, testCase testCase) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			t.Helper()
			got := outcome{puts: map[string]string{}}
			store := &jobStoreMock{
				T: t,
				GetMediaStub: func(context.Context, uuid.UUID) (media.Media, error) {
					if testCase.getMedia != nil {
						return testCase.getMedia()
					}
					return media.Media{UUID: testMediaUUID, Kind: media.KindVideo, Status: media.StatusProcessing, OriginalKey: testOriginal}, nil
				},
				RetryMediaJobStub: func(_ context.Context, jobUUID uuid.UUID, _ string, runAfter time.Time) error {
					expect.Equal(t, jobUUID, testJobUUID)
					got.retryRunAfter = &runAfter
					return nil
				},
				FailVideoMediaStub: func(_ context.Context, jobUUID, mediaUUID uuid.UUID, _ string, reason string) error {
					expect.Equal(t, jobUUID, testJobUUID)
					expect.Equal(t, mediaUUID, testMediaUUID)
					got.failReason = reason
					return nil
				},
				CompleteVideoMediaStub: func(_ context.Context, jobUUID, mediaUUID uuid.UUID, result media.VideoResult) error {
					if testCase.complete != nil {
						if err := testCase.complete(); err != nil {
							return err
						}
					}
					got.completed = &result
					return nil
				},
			}
			blobs := &blobStoreMock{
				T: t,
				GetStub: func(_ context.Context, key string) (*blob.Object, error) {
					expect.Equal(t, key, testOriginal)
					if testCase.getBlob != nil {
						return testCase.getBlob()
					}
					return &blob.Object{ReadSeekCloser: readSeekCloser{bytes.NewReader([]byte("video bytes"))}}, nil
				},
				PutStub: func(_ context.Context, key string, r io.Reader, _ int64, contentType string) error {
					if testCase.put != nil {
						if err := testCase.put(key); err != nil {
							return err
						}
					}
					got.puts[key] = contentType
					return nil
				},
				DeleteStub: func(_ context.Context, key string) error {
					got.deleted = append(got.deleted, key)
					return nil
				},
			}
			tools := &videoToolsMock{
				T: t,
				ProbeStub: func(_ context.Context, path string) (media.ProbeResult, error) {
					if testCase.probe != nil {
						return testCase.probe(path)
					}
					if bytesEqualFile(path, "video bytes") {
						return inputProbe, nil
					}
					return outputProbe, nil
				},
				TranscodeStub: func(_ context.Context, _, outPath string) error {
					if testCase.transcode != nil {
						if err := testCase.transcode(); err != nil {
							return err
						}
					}
					return os.WriteFile(outPath, []byte("transcoded"), 0o600)
				},
				PosterStub: func(_ context.Context, _ string, durationMS int, outPath string) error {
					expect.Equal(t, durationMS, outputProbe.DurationMS)
					return os.WriteFile(outPath, []byte("jpeg"), 0o600)
				},
			}
			w := &worker{
				store: store, blobs: blobs, tools: tools, logger: zap.NewNop(),
				tmpDir: t.TempDir(), maxAttempts: 3, now: func() time.Time { return testNow },
			}
			attempts := testCase.attempts
			if attempts == 0 {
				attempts = 1
			}
			w.process(context.Background(), media.Job{UUID: testJobUUID, MediaUUID: testMediaUUID, Attempts: attempts})
			testCase.expected(t, got)
		})
	}

	retriedAfter := func(d time.Duration) func(t *testing.T, got outcome) {
		return func(t *testing.T, got outcome) {
			expect.Equal(t, got.retryRunAfter, new(testNow.Add(d)))
			expect.Equal(t, got.failReason, "")
			expect.Equal(t, got.completed, nil)
		}
	}

	run("GetMediaError", testCase{
		getMedia: func() (media.Media, error) { return media.Media{}, expect.ErrTest },
		expected: retriedAfter(30 * time.Second),
	})
	run("DownloadError", testCase{
		getBlob:  func() (*blob.Object, error) { return nil, expect.ErrTest },
		expected: retriedAfter(30 * time.Second),
	})
	run("InvalidVideo", testCase{
		probe: func(string) (media.ProbeResult, error) {
			return media.ProbeResult{}, media.InvalidVideoError{Reason: "this file isn't a video we can read"}
		},
		expected: func(t *testing.T, got outcome) {
			expect.Equal(t, got.failReason, "this file isn't a video we can read")
			expect.Equal(t, got.retryRunAfter, nil)
			expect.Equal(t, got.deleted, []string{testOriginal})
		},
	})
	run("TooLongVideo", testCase{
		probe: func(string) (media.ProbeResult, error) {
			return media.ProbeResult{DurationMS: 240_000, Width: 720, Height: 1280}, nil
		},
		expected: func(t *testing.T, got outcome) {
			expect.Equal(t, got.failReason, "video must be 3 minutes or shorter")
			expect.Equal(t, got.retryRunAfter, nil)
		},
	})
	run("TranscodeErrorLastAttempt", testCase{
		attempts:  3,
		transcode: func() error { return expect.ErrTest },
		expected: func(t *testing.T, got outcome) {
			expect.Equal(t, got.failReason, genericFailure)
			expect.Equal(t, got.retryRunAfter, nil)
			expect.Equal(t, got.deleted, []string{testOriginal})
		},
	})
	run("TranscodeErrorEarlyAttempt", testCase{
		attempts:  2,
		transcode: func() error { return expect.ErrTest },
		expected:  retriedAfter(60 * time.Second),
	})
	run("PutError", testCase{
		put:      func(string) error { return expect.ErrTest },
		expected: retriedAfter(30 * time.Second),
	})
	run("CompleteError", testCase{
		complete: func() error { return expect.ErrTest },
		expected: retriedAfter(30 * time.Second),
	})
	run("Success", testCase{
		expected: func(t *testing.T, got outcome) {
			expect.Equal(t, got.puts, map[string]string{
				media.VideoKey(testMediaUUID):  "video/mp4",
				media.PosterKey(testMediaUUID): "image/jpeg",
			})
			expect.Equal(t, got.completed, &media.VideoResult{
				DisplayKey:   media.VideoKey(testMediaUUID),
				ThumbnailKey: media.PosterKey(testMediaUUID),
				Width:        720,
				Height:       1280,
				DurationMS:   3010,
				SizeBytes:    int64(len("transcoded")),
			})
			expect.Equal(t, got.deleted, []string{testOriginal})
			expect.Equal(t, got.retryRunAfter, nil)
		},
	})
}

// bytesEqualFile reports whether the file at path holds exactly content.
// It tells the downloaded input apart from the transcoded output.
func bytesEqualFile(path, content string) bool {
	data, err := os.ReadFile(path)
	return err == nil && string(data) == content
}
