// plural-api-transcoder turns uploaded videos into playable MP4s. It polls the
// media_job table, transcodes one video at a time with ffmpeg, and stores the
// result next to the API's other media. Run as many copies as needed: jobs are
// claimed with row locks, so each video is processed once.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/peterbourgon/ff/v3"
	"go.uber.org/zap"

	"github.com/plural-social/plural-api/internal/blob"
	"github.com/plural-social/plural-api/internal/media"
	"github.com/plural-social/plural-api/internal/postgres"
)

func main() {
	var (
		dbConnection   = flag.String("db.connection", "postgres://plural:plural@localhost:5432/plural?sslmode=disable", "Postgres connection string")
		startupTimeout = flag.Duration("startup.timeout", 30*time.Second, "Startup timeout")
		s3Endpoint     = flag.String("s3.endpoint", "localhost:8333", "S3-compatible storage endpoint (host:port, no scheme)")
		s3AccessKey    = flag.String("s3.access.key", "plural", "S3 access key")
		s3SecretKey    = flag.String("s3.secret.key", "plural", "S3 secret key")
		s3Bucket       = flag.String("s3.bucket", "plural-media", "S3 bucket for media files")
		s3Region       = flag.String("s3.region", "us-east-1", "S3 region")
		s3UseSSL       = flag.Bool("s3.use.ssl", false, "Use HTTPS to reach S3")
		pollInterval   = flag.Duration("poll.interval", 2*time.Second, "Wait between polls when no job is ready")
		jobTimeout     = flag.Duration("job.timeout", 10*time.Minute, "Longest time one job may run")
		staleAfter     = flag.Duration("stale.after", 15*time.Minute, "Reclaim running jobs locked longer than this (a crashed worker). Must exceed job.timeout")
		maxAttempts    = flag.Int("max.attempts", 3, "Attempts before a video is marked failed")
		ffmpegPath     = flag.String("ffmpeg.path", "ffmpeg", "ffmpeg binary")
		ffprobePath    = flag.String("ffprobe.path", "ffprobe", "ffprobe binary")
		tmpDir         = flag.String("tmp.dir", "", "Directory for work files. Empty uses the system temp dir")
	)
	if err := ff.Parse(flag.CommandLine, os.Args[1:], ff.WithEnvVars()); err != nil {
		log.Fatalf("Error parsing flags: %s", err)
	}

	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatalf("Error creating logger: %s", err)
	}
	logger = logger.With(zap.String("service", "plural-api-transcoder"))
	defer logger.Sync()

	// A job that outlives staleAfter would be claimed by a second worker while
	// the first is still running it.
	if *staleAfter <= *jobTimeout {
		logger.Fatal("stale.after must be larger than job.timeout")
	}
	for _, tool := range []string{*ffmpegPath, *ffprobePath} {
		if err := exec.Command(tool, "-version").Run(); err != nil {
			logger.Fatal("Error running video tool", zap.String("tool", tool), zap.Error(err))
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), *startupTimeout)
	defer cancel()
	store, err := postgres.NewStore(ctx, *dbConnection)
	if err != nil {
		logger.Fatal("Error initializing store", zap.Error(err))
	}
	defer store.Close()
	blobs, err := blob.New(ctx, blob.Config{
		Endpoint:  *s3Endpoint,
		AccessKey: *s3AccessKey,
		SecretKey: *s3SecretKey,
		Bucket:    *s3Bucket,
		Region:    *s3Region,
		UseSSL:    *s3UseSSL,
	})
	if err != nil {
		logger.Fatal("Error initializing blob store", zap.Error(err))
	}

	w := &worker{
		store:       store,
		blobs:       blobs,
		tools:       media.Tools{FFmpegPath: *ffmpegPath, FFprobePath: *ffprobePath},
		logger:      logger,
		tmpDir:      *tmpDir,
		maxAttempts: *maxAttempts,
		now:         time.Now,
	}

	signalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger.Info("Starting transcoder")
	for signalCtx.Err() == nil {
		job, ok, err := store.ClaimMediaJob(signalCtx, *staleAfter)
		switch {
		case err != nil && signalCtx.Err() == nil:
			logger.Error("Error claiming job", zap.Error(err))
			sleep(signalCtx, *pollInterval)
		case !ok:
			sleep(signalCtx, *pollInterval)
		default:
			// The job context is not tied to the signal, so a shutdown waits for
			// the current job to finish instead of abandoning it.
			jobCtx, cancelJob := context.WithTimeout(context.Background(), *jobTimeout)
			w.process(jobCtx, job)
			cancelJob()
		}
	}
	logger.Info("Goodbye!")
}

// sleep waits for d or until ctx ends.
func sleep(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}
