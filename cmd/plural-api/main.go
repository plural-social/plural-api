package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/peterbourgon/ff/v3"
	"go.uber.org/zap"

	"github.com/plural-social/plural-api/internal/blob"
	"github.com/plural-social/plural-api/internal/media"
	"github.com/plural-social/plural-api/internal/postgres"
)

var version = "dev"

// app holds every dependency the HTTP handlers use. Tests build it with mocks
// and only the fields the code under test needs.
type app struct {
	logger *zap.Logger

	// Stores
	accountStore    accountStore
	mediaStore      mediaStore
	postStore       postStore
	socialStore     socialStore
	moderationStore moderationStore
	blobStore       blobStore

	// Configuration
	corsOrigins       []string
	instanceDomain    string
	instanceName      string
	termsURL          string
	publicURL         string
	trustProxyHeaders bool
	maxImageBytes     int64
	maxVideoBytes     int64
	tmpDir            string // empty means os.TempDir()

	// Replaceable in tests
	now     func() time.Time
	newUUID func() uuid.UUID

	// Per-IP rate limiters. Nil disables the limit.
	sessionLimiter *ipRateLimiter
	signupLimiter  *ipRateLimiter
	uploadLimiter  *ipRateLimiter
	reportLimiter  *ipRateLimiter
}

func main() {
	var (
		port            = flag.Int("port", 4100, "plural-api service port")
		dbConnection    = flag.String("db.connection", "postgres://plural:plural@localhost:5432/plural?sslmode=disable", "Postgres connection string")
		startupTimeout  = flag.Duration("startup.timeout", 30*time.Second, "Startup timeout")
		shutdownTimeout = flag.Duration("shutdown.timeout", 30*time.Second, "Shutdown timeout")
		corsOrigins     = flag.String("cors.origins", "http://localhost:8081", "Comma-separated list of allowed browser origins; * allows any")
		instanceDomain  = flag.String("instance.domain", "localhost", "Public domain of this instance")
		instanceName    = flag.String("instance.name", "Plural", "Display name of this instance")
		termsURL        = flag.String("instance.terms.url", "", "URL of the terms and community guidelines shown at sign-up. Empty hides the terms checkbox")
		trustProxy      = flag.Bool("trust.proxy.headers", false, "Trust X-Forwarded-For and X-Forwarded-Proto (only behind a proxy that sets them)")
		publicURL       = flag.String("public.url", "", "Public base URL for file links, e.g. https://plural.example. Empty uses the request host")
		s3Endpoint      = flag.String("s3.endpoint", "localhost:8333", "S3-compatible storage endpoint (host:port, no scheme)")
		s3AccessKey     = flag.String("s3.access.key", "plural", "S3 access key")
		s3SecretKey     = flag.String("s3.secret.key", "plural", "S3 secret key")
		s3Bucket        = flag.String("s3.bucket", "plural-media", "S3 bucket for media files")
		s3Region        = flag.String("s3.region", "us-east-1", "S3 region")
		s3UseSSL        = flag.Bool("s3.use.ssl", false, "Use HTTPS to reach S3")
		maxVideoBytes   = flag.Int64("media.max.video.bytes", media.VideoMaxBytes, "Largest accepted video upload in bytes")
		tmpDir          = flag.String("media.tmp.dir", "", "Directory for video uploads in progress. Empty uses the system temp dir")
	)

	if err := ff.Parse(flag.CommandLine, os.Args[1:], ff.WithEnvVars()); err != nil {
		log.Fatalf("Error parsing flags: %s", err)
	}

	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatalf("Error creating logger: %s", err)
	}
	logger = logger.With(zap.String("service", "plural-api"))
	defer logger.Sync()

	ctx, cancel := context.WithTimeout(context.Background(), *startupTimeout)
	defer cancel()

	store, err := postgres.NewStore(ctx, *dbConnection)
	if err != nil {
		logger.Fatal("Error initializing store", zap.Error(err))
	}

	blobStore, err := blob.New(ctx, blob.Config{
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

	a := &app{
		logger: logger,

		accountStore:    store,
		mediaStore:      store,
		postStore:       store,
		socialStore:     store,
		moderationStore: store,
		blobStore:       blobStore,

		corsOrigins:       splitOrigins(*corsOrigins),
		instanceDomain:    *instanceDomain,
		instanceName:      *instanceName,
		termsURL:          *termsURL,
		publicURL:         strings.TrimSuffix(*publicURL, "/"),
		trustProxyHeaders: *trustProxy,
		maxImageBytes:     media.ImageMaxBytes,
		maxVideoBytes:     *maxVideoBytes,
		tmpDir:            *tmpDir,

		now:     time.Now,
		newUUID: uuid.New,

		sessionLimiter: newIPRateLimiter(6*time.Second, 10),
		signupLimiter:  newIPRateLimiter(12*time.Minute, 5),
		uploadLimiter:  newIPRateLimiter(10*time.Second, 30),
		reportLimiter:  newIPRateLimiter(6*time.Second, 10),
	}
	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", *port),
		Handler:           a.routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	// On SIGINT/SIGTERM, stop accepting connections and drain in-flight
	// requests. ListenAndServe returns as soon as shutdown starts, so wait on
	// shutdownDone before tearing anything else down.
	signalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	shutdownDone := make(chan struct{})
	context.AfterFunc(signalCtx, func() {
		defer close(shutdownDone)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), *shutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("Error shutting down server", zap.Error(err))
		}
	})

	logger.Info("Starting server", zap.Int("port", *port))
	if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		logger.Fatal("Error running server", zap.Error(err))
	}
	<-shutdownDone

	logger.Info("Closing database connection")
	if err := store.Close(); err != nil {
		logger.Error("Error closing database connection", zap.Error(err))
	}

	logger.Info("Goodbye!")
}

func (a *app) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", a.endpoint(a.getHealthEndpoint))
	a.setUpInstanceRoutes(mux)
	a.setUpAccountRoutes(mux)
	a.setUpSessionRoutes(mux)
	a.setUpMeRoutes(mux)
	a.setUpUserRoutes(mux)
	a.setUpMediaRoutes(mux)
	a.setUpPostRoutes(mux)
	a.setUpFeedRoutes(mux)
	a.setUpLikeRoutes(mux)
	a.setUpCommentRoutes(mux)
	a.setUpModerationRoutes(mux)
	return a.recoverPanics(a.logRequests(a.cors(mux)))
}

func splitOrigins(s string) []string {
	origins := strings.Split(s, ",")
	for i := range origins {
		origins[i] = strings.TrimSpace(origins[i])
	}
	return origins
}

func (a *app) getHealthEndpoint(r *http.Request) (int, any) {
	if err := a.accountStore.Ping(r.Context()); err != nil {
		a.logger.Error("Health check failed", zap.Error(err))
		return http.StatusServiceUnavailable, ErrorResponse{Message: "database unavailable"}
	}
	return http.StatusOK, map[string]string{"status": "ok"}
}
