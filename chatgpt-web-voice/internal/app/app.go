package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/dyhhhhhh/chatgpt-web-voice/internal/accounts"
	"github.com/dyhhhhhh/chatgpt-web-voice/internal/api"
	"github.com/dyhhhhhh/chatgpt-web-voice/internal/apikeys"
	"github.com/dyhhhhhh/chatgpt-web-voice/internal/auth"
	"github.com/dyhhhhhh/chatgpt-web-voice/internal/callsessions"
	"github.com/dyhhhhhh/chatgpt-web-voice/internal/config"
	"github.com/dyhhhhhh/chatgpt-web-voice/internal/conversations"
	"github.com/dyhhhhhh/chatgpt-web-voice/internal/logging"
	"github.com/dyhhhhhh/chatgpt-web-voice/internal/recordings"
	"github.com/dyhhhhhh/chatgpt-web-voice/internal/scenes"
	"github.com/dyhhhhhh/chatgpt-web-voice/internal/secretbox"
	"github.com/dyhhhhhh/chatgpt-web-voice/internal/store"
	"github.com/dyhhhhhh/chatgpt-web-voice/internal/voice"
)

// Run loads configuration, wires dependencies, and serves until interrupted.
func Run() error {
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	logger := logging.New(cfg.LogFormat, cfg.LogLevel)
	slog.SetDefault(logger)
	logger.Info("configuration_loaded",
		"environment", cfg.Environment,
		"listen_addr", cfg.ListenAddr,
		"database_file", cfg.DatabaseFile,
		"tls", cfg.TLS,
		"upstream_transport", cfg.UpstreamTransport,
		"upstream_tls_profile", cfg.TLSProfile,
		"upstream_impersonate", cfg.Impersonate,
		"upstream_skip_ssl_verify", cfg.SkipSSLVerify,
		"log_format", cfg.LogFormat,
		"log_level", cfg.LogLevel,
	)

	db, err := store.Open(cfg.DatabaseFile)
	if err != nil {
		return fmt.Errorf("account database open failed: %w", err)
	}
	// closeDB closes the SQLite handle exactly once. It must never run while
	// the scene worker may still be writing; see the deferred close decision
	// and stopSceneWorker below.
	var dbCloseOnce sync.Once
	closeDB := func() {
		dbCloseOnce.Do(func() { _ = db.Close() })
	}

	tokenKey, err := secretbox.ParseKey(cfg.TokenEncryptionKey)
	if err != nil {
		return fmt.Errorf("invalid VOICE_TOKEN_ENCRYPTION_KEY: %w", err)
	}
	tokenBox, err := secretbox.New(tokenKey)
	if err != nil {
		return fmt.Errorf("token encryption setup failed: %w", err)
	}

	accountPool := accounts.NewPoolFromDB(db).WithBox(tokenBox)
	rewritten, err := accountPool.SealStoredTokens()
	if err != nil {
		return fmt.Errorf("seal stored access tokens failed: %w", err)
	}
	if rewritten > 0 {
		logger.Info("access_tokens_sealed", "rewritten_accounts", rewritten)
	}

	conversationStore := conversations.NewStore(db)
	callSessionStore := callsessions.NewStore(db)
	apiKeyStore := apikeys.NewStore(db)
	recordingStore, err := recordings.NewStore(db, filepath.Join(cfg.DataDir, "recordings"))
	if err != nil {
		return fmt.Errorf("recording store setup failed: %w", err)
	}
	if recovered, recoveryErr := recordingStore.RecoverInterrupted(); recoveryErr != nil {
		logger.Warn("recording_orphan_recovery_failed", "error", recoveryErr)
	} else if recovered > 0 {
		logger.Info("recording_orphans_recovered", "count", recovered)
	}
	releasedOrphans, err := callSessionStore.MarkAllActiveReleased()
	if err != nil {
		return fmt.Errorf("release orphan call sessions failed: %w", err)
	}
	if releasedOrphans > 0 {
		logger.Info("call_sessions_released_on_startup", "count", releasedOrphans)
	}
	available, err := accountPool.AvailableCount()
	if err != nil {
		return fmt.Errorf("account database check failed: %w", err)
	}
	logger.Info("account_database_ready", "available_accounts", available)

	// Scene subsystem ("另一种可能 · 生活的一帧"). Text orchestration
	// (VOICE_SCENE_AI_*) and image generation (IMAGE_API_*) are fully separate
	// providers with separate credentials: neither reads nor falls back to the
	// other, and neither ever touches the ChatGPT Web account pool. The service
	// is always wired so listing/viewing/deleting stored scenes keeps working;
	// only draft creation and generation answer 503 when a capability is
	// missing.
	sceneStore := scenes.NewStore(db)
	textProvider := scenes.NewHTTPTextProvider(cfg.SceneText, logger)
	imageProvider := scenes.NewOpenAIImageProvider(cfg.SceneImage, logger)
	service := scenes.NewService(
		sceneStore,
		filepath.Join(cfg.DataDir, "scenes"),
		textProvider,
		imageProvider,
		logger,
		imageProvider.Name(),
		imageProvider.Model(),
		cfg.SceneText.Configured(),
		cfg.SceneImage.Configured(),
	)
	var sceneService scenes.Interface = service

	// Legacy active jobs must be recovered regardless of provider or worker
	// configuration; otherwise they would stay queued/composing/generating
	// forever and the frontend would keep polling them.
	if interrupted, recoveryErr := service.MarkInterruptedOnStartup(); recoveryErr != nil {
		logger.Warn("scene_orphan_recovery_failed", "error", recoveryErr)
	} else if interrupted > 0 {
		logger.Info("scene_orphans_recovered", "count", interrupted)
	}

	// The worker is created only when both providers are configured. This gate
	// deliberately uses ProvidersConfigured(), which does not depend on the
	// worker existing (no circular dependency).
	//
	// sceneWorkerShutdownTimeout bounds the wait for in-flight scene jobs after
	// their contexts are cancelled. Image requests are cancelled immediately on
	// shutdown, so this is normally short; the bound only exists to guarantee
	// the worker never outlives the database handle.
	const sceneWorkerShutdownTimeout = 20 * time.Second

	var sceneWorkerCancel context.CancelFunc
	var sceneWorkerDone <-chan struct{}
	var sceneWorkerStopped bool

	// stopSceneWorker cancels the queue and waits a bounded time for in-flight
	// jobs to finish (their contexts are cancelled by the same signal). It
	// returns whether the worker fully stopped. It is called explicitly on the
	// shutdown path and again via defer on every other exit path; the guard
	// makes repeated calls idempotent.
	stopSceneWorker := func() bool {
		if sceneWorkerStopped {
			return true
		}
		if sceneWorkerCancel == nil {
			sceneWorkerStopped = true
			return true
		}
		sceneWorkerCancel()
		select {
		case <-sceneWorkerDone:
			sceneWorkerStopped = true
			logger.Info("scene_worker_stopped")
		case <-time.After(sceneWorkerShutdownTimeout):
			logger.Warn("scene_worker_shutdown_timeout", "timeout_seconds", int(sceneWorkerShutdownTimeout.Seconds()))
		}
		return sceneWorkerStopped
	}

	// The database may only be closed after the worker is confirmed stopped.
	// If the bounded wait expired, Run still returns (the OS reclaims file
	// descriptors at process exit) but the handle is deliberately left open
	// and a tiny background goroutine closes it once the worker finishes, so
	// the database is never closed while a worker goroutine may still write.
	//
	// Defer order matters: stopSceneWorker (declared after this) runs before
	// this decision, so sceneWorkerStopped is final when this executes.
	defer func() {
		if sceneWorkerDone == nil || sceneWorkerStopped {
			closeDB()
			return
		}
		logger.Warn("scene_worker_db_close_deferred", "reason", "scene worker still running after shutdown timeout")
		go func() {
			<-sceneWorkerDone
			closeDB()
		}()
	}()
	defer stopSceneWorker()

	if service.ProvidersConfigured() {
		worker := scenes.NewWorker(
			service,
			cfg.SceneWorker.GenerationConcurrency,
			scenes.DefaultQueueCapacity,
			time.Duration(cfg.SceneWorker.RequestTimeout)*time.Second,
			logger,
		)
		service.AttachWorker(worker)
		workerCtx, cancelWorker := context.WithCancel(context.Background())
		sceneWorkerCancel = cancelWorker
		sceneWorkerDone = worker.Done()
		go worker.Run(workerCtx)
	}
	logger.Info("scene_subsystem_status",
		"text_configured", cfg.SceneText.Configured(),
		"text_model", cfg.SceneText.Model,
		"image_configured", cfg.SceneImage.Configured(),
		"image_model", cfg.SceneImage.Model,
		"worker_concurrency", cfg.SceneWorker.GenerationConcurrency,
	)

	authManager := auth.NewWithLimits(
		cfg.AuthUsername,
		cfg.AuthPassword,
		time.Duration(cfg.AuthSessionTTL)*time.Second,
		auth.Limits{
			MaxFailures:  cfg.LoginMaxFailures,
			Window:       time.Duration(cfg.LoginWindowSeconds) * time.Second,
			Lockout:      time.Duration(cfg.LoginLockoutSeconds) * time.Second,
			FailureDelay: 200 * time.Millisecond,
		},
		logger,
	).WithSessionStore(auth.NewSessionStore(db)).WithTurnstile(
		auth.NewCloudflareTurnstile(cfg.TurnstileSiteKey, cfg.TurnstileSecretKey, nil),
	).WithTrustedCloudflareIP(cfg.TrustCloudflareIP)
	apiKeyManager := auth.NewAPIKeyManager(apiKeyStore, logger)
	voiceService := voice.New(cfg, accountPool, logger).WithCallSessions(callSessionStore)
	apiServer := api.New(api.Dependencies{
		Voice:         voiceService,
		Accounts:      accountPool,
		Conversations: conversationStore,
		APIKeys:       apiKeyStore,
		CallSessions:  callSessionStore,
		Recordings:    recordingStore,
		Scenes:        sceneService,
	})

	handler := newHandler(cfg, authManager, apiKeyManager, apiServer, logger)
	server := &http.Server{
		Addr:              normalizeListenAddr(cfg.ListenAddr),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    1 << 20,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}

	certFile, keyFile, scheme, err := prepareTLS(cfg, logger)
	if err != nil {
		return fmt.Errorf("tls setup failed: %w", err)
	}
	logListeningAddresses(server.Addr, cfg.StaticDir, scheme, logger)

	serverErr := make(chan error, 1)
	go func() {
		if cfg.TLS {
			serverErr <- server.ListenAndServeTLS(certFile, keyFile)
			return
		}
		serverErr <- server.ListenAndServe()
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(stop)

	select {
	case sig := <-stop:
		logger.Info("shutdown_requested", "signal", sig.String())
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			return fmt.Errorf("shutdown failed: %w", err)
		}
		// Stop the worker (cancel + bounded wait) before Run returns; the
		// database close decision runs via the deferred call afterwards.
		stopSceneWorker()
		logger.Info("shutdown_completed")
		return nil
	case err := <-serverErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("server failed: %w", err)
		}
		return nil
	}
}

func newHandler(cfg config.Config, authManager *auth.Manager, apiKeyManager *auth.APIKeyManager, apiServer *api.Server, logger *slog.Logger) http.Handler {
	public := http.NewServeMux()
	apiServer.RegisterPublic(public)
	registerPublicStaticRoutes(public, cfg.StaticDir)
	admin := http.NewServeMux()
	apiServer.RegisterAdmin(admin)
	registerAdminStaticRoutes(admin, cfg.StaticDir)
	downstream := http.NewServeMux()
	apiServer.RegisterDownstream(downstream)
	publicLimiter := newPublicRateLimiter(cfg)

	root := http.NewServeMux()
	// Shared CSS/assets must be public: login.html loads /static/app.css before auth.
	registerPublicStaticAssets(root, cfg.StaticDir)
	root.Handle("GET /login", authManager.LoginPage(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveFile(w, r, joinStatic(cfg.StaticDir, "login.html"))
	})))
	root.HandleFunc("GET /api/auth/config", authManager.HandleAuthConfig)
	root.HandleFunc("POST /api/auth/login", authManager.HandleLogin)
	root.Handle("POST /api/auth/logout", authManager.Require(http.HandlerFunc(authManager.HandleLogout)))
	root.Handle("GET /api/auth/session", authManager.Public(http.HandlerFunc(authManager.HandleSession)))
	root.Handle("/v1/", apiKeyManager.Require(downstream))
	for _, path := range []string{
		"GET /accounts", "GET /accounts.html",
		"GET /keys", "GET /keys.html",
		"GET /sessions", "GET /sessions.html",
		"GET /records", "GET /records.html",
		"/api/accounts", "/api/accounts/",
		"/api/keys", "/api/keys/",
		"/api/call-sessions", "/api/call-sessions/",
		"/api/admin/recordings", "/api/admin/recordings/",
	} {
		root.Handle(path, authManager.Require(admin))
	}
	root.Handle("/", authManager.Public(publicLimiter.Wrap(public)))

	return logging.HTTPMiddleware(logger, securityHeaders(root))
}
