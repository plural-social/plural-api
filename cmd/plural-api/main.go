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

	"github.com/peterbourgon/ff/v3"
	"go.uber.org/zap"

	"github.com/plural-social/plural-api/internal/postgres"
)

var version = "dev"

type app struct {
	accountStore   accountStore
	logger         *zap.Logger
	corsOrigins    []string
	instanceDomain string
	instanceName   string
	now            func() time.Time

	trustProxyHeaders bool
	sessionLimiter    *ipRateLimiter
	signupLimiter     *ipRateLimiter
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
		trustProxy      = flag.Bool("trust.proxy.headers", false, "Trust X-Forwarded-For for client IPs (only behind a proxy that sets it)")
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

	a := &app{
		accountStore:   store,
		logger:         logger,
		corsOrigins:    splitOrigins(*corsOrigins),
		instanceDomain: *instanceDomain,
		instanceName:   *instanceName,
		now:            time.Now,

		trustProxyHeaders: *trustProxy,
		sessionLimiter:    newIPRateLimiter(6*time.Second, 10),
		signupLimiter:     newIPRateLimiter(12*time.Minute, 5),
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
