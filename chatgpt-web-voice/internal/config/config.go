package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

const (
	EnvironmentDevelopment = "development"
	EnvironmentProduction  = "production"

	DevelopmentTurnstileSiteKey   = "1x00000000000000000000AA"
	DevelopmentTurnstileSecretKey = "1x0000000000000000000000000000000AA"

	// Upstream transport modes.
	// Docker image defaults to curl-impersonate; local go run defaults to tls-client.
	TransportTLSClient       = "tls-client"
	TransportCurlImpersonate = "curl-impersonate"
	TransportGo              = "go"

	// Defaults mirror ChatGPT2API-GO account fingerprint + upstream headers
	// (Edge 143 browser persona + curl edge_101).
	//
	// tls-client MappedTLSClients has no edge_101; Chrome_120 is the same
	// fallback their upstreamTLSProfile() uses when edge_101 is missing.
	DefaultTLSProfile = "chrome_120"
	// defaultImpersonate is the curl-impersonate launcher (lwthiker v0.6.1).
	defaultImpersonate = "edge_101"

	defaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Safari/537.36 Edg/143.0.0.0"
	defaultSecCHUA   = `"Microsoft Edge";v="143", "Chromium";v="143", "Not A(Brand";v="24"`
	// Full Client Hints match ChatGPT2API-GO headers() defaults.
	defaultSecCHUAArch            = `"x86"`
	defaultSecCHUABitness         = `"64"`
	defaultSecCHUAFullVersion     = `"143.0.3650.96"`
	defaultSecCHUAFullVersionList = `"Microsoft Edge";v="143.0.3650.96", "Chromium";v="143.0.7499.147", "Not A(Brand";v="24.0.0.0"`
	defaultSecCHUAMobile          = "?0"
	defaultSecCHUAModel           = `""`
	defaultSecCHUAPlatform        = `"Windows"`
	defaultSecCHUAPlatformVersion = `"19.0.0"`
	// ChatGPT web client build identifiers from ChatGPT2API-GO.
	defaultClientVersion     = "prod-be885abbfcfe7b1f511e88b3003d9ee44757fbad"
	defaultClientBuildNumber = "5955942"
)

// SceneTextConfig holds the scene text-orchestration provider settings
// ("另一种可能 · 生活的一帧" candidate moments and SceneBrief). It reads
// VOICE_SCENE_AI_* and only calls /v1/chat/completions. It never touches the
// ChatGPT Web account pool nor the IMAGE_API_* credentials.
type SceneTextConfig struct {
	// BaseURL is the OpenAI-compatible chat-completions base
	// (e.g. https://api.openai.com/v1). A trailing /v1 is normalized away and
	// re-appended, so both "https://example.com" and "https://example.com/v1"
	// produce exactly one /v1/chat/completions request.
	BaseURL string
	// APIKey is the independent text-provider key. Never stored or returned.
	APIKey string
	// Model is the text model used for candidates and SceneBrief.
	Model string
	// RequestTimeout bounds one chat-completion request.
	RequestTimeout int
}

// Configured reports whether text orchestration can be called.
func (c SceneTextConfig) Configured() bool {
	return strings.TrimSpace(c.APIKey) != "" && strings.TrimSpace(c.BaseURL) != ""
}

// SceneImageConfig holds the production OpenAI Images API provider settings
// ("另一种可能 · 生活的一帧" final image). It reads IMAGE_API_* and only calls
// /v1/images/generations with fixed business presets
// (1536x1024, quality=standard, n=1, no response_format). It never reads or
// falls back to VOICE_SCENE_AI_API_KEY or the account pool tokens.
type SceneImageConfig struct {
	// BaseURL is the OpenAI Images API host (e.g. https://api.openai.com).
	// Both "https://example.com" and "https://example.com/v1" are accepted and
	// normalized to exactly one /v1/images/generations request.
	BaseURL string
	// APIKey is the independent image-provider key. Never stored or returned.
	APIKey string
	// Model is the image model (e.g. gpt-image-2).
	Model string
	// RequestTimeout bounds one image-generation request.
	RequestTimeout int
	// MaxImageBytes caps accepted generated image payloads.
	MaxImageBytes int64
}

// Configured reports whether image generation can be called.
func (c SceneImageConfig) Configured() bool {
	return strings.TrimSpace(c.APIKey) != "" && strings.TrimSpace(c.BaseURL) != ""
}

// SceneWorkerConfig holds scene-job scheduling settings that belong to the
// gateway process, not to any vendor credential.
type SceneWorkerConfig struct {
	// GenerationConcurrency bounds parallel generation jobs.
	GenerationConcurrency int
	// RequestTimeout bounds one whole async job (brief composition + image).
	RequestTimeout int
}

// Config holds runtime settings for the voice gateway.
type Config struct {
	Environment         string
	DataDir             string
	StaticDir           string
	DatabaseFile        string
	AuthUsername        string
	AuthPassword        string
	AuthSessionTTL      int
	LoginMaxFailures    int
	LoginLockoutSeconds int
	LoginWindowSeconds  int
	TurnstileSiteKey    string
	TurnstileSecretKey  string
	TrustCloudflareIP   bool
	PublicSessionRate   int
	PublicWriteRate     int
	TokenEncryptionKey  string
	// UpstreamTransport selects the HTTP stack for chatgpt.com:
	// "tls-client" (local default), "curl-impersonate" (Docker default), or "go".
	UpstreamTransport string
	// TLSProfile is the bogdanfinn/tls-client profile key (e.g. chrome_120).
	TLSProfile string
	// Impersonate is the curl-impersonate browser profile (e.g. edge_101).
	Impersonate string
	// CurlImpersonateBin is an optional absolute path to a curl-impersonate binary.
	CurlImpersonateBin string
	// DeviceID / SessionID are process-global oai-* fingerprint values.
	DeviceID               string
	SessionID              string
	SkipSSLVerify          bool
	SessionTTLSeconds      int
	MaxAccountAttempts     int
	DefaultUA              string
	SecCHUA                string
	SecCHUAArch            string
	SecCHUABitness         string
	SecCHUAFullVersion     string
	SecCHUAFullVersionList string
	SecCHUAMobile          string
	SecCHUAModel           string
	SecCHUAPlatform        string
	SecCHUAPlatformVersion string
	ClientVersion          string
	ClientBuildNumber      string
	ListenAddr             string
	TLS                    bool
	TLSCertFile            string
	TLSKeyFile             string
	TLSCertDir             string
	LogFormat              string
	LogLevel               string
	SceneText              SceneTextConfig
	SceneImage             SceneImageConfig
	SceneWorker            SceneWorkerConfig
}

func env(name, def string) string {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return def
	}
	return v
}

func envRaw(name, def string) string {
	v, ok := os.LookupEnv(name)
	if !ok || v == "" {
		return def
	}
	return v
}

func envBool(name string, def bool) bool {
	raw := strings.ToLower(env(name, map[bool]string{true: "1", false: "0"}[def]))
	return raw == "1" || raw == "true" || raw == "yes" || raw == "on"
}

func envInt(name string, def int) int {
	raw := env(name, "")
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	return n
}

func normalizeUpstreamTransport(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", TransportTLSClient, "tls", "tlsclient":
		return TransportTLSClient
	case TransportCurlImpersonate, "curl":
		return TransportCurlImpersonate
	case TransportGo, "stdlib", "nethttp", "net/http":
		return TransportGo
	default:
		return strings.ToLower(strings.TrimSpace(raw))
	}
}

// Load reads configuration from environment variables.
func Load() Config {
	baseDir, err := os.Getwd()
	if err != nil {
		baseDir = "."
	}

	environment := strings.ToLower(env("VOICE_ENV", EnvironmentDevelopment))
	turnstileSiteKey := env("VOICE_TURNSTILE_SITE_KEY", "")
	turnstileSecretKey := envRaw("VOICE_TURNSTILE_SECRET_KEY", "")
	if environment != EnvironmentProduction {
		if strings.TrimSpace(turnstileSiteKey) == "" {
			turnstileSiteKey = DevelopmentTurnstileSiteKey
		}
		if strings.TrimSpace(turnstileSecretKey) == "" {
			turnstileSecretKey = DevelopmentTurnstileSecretKey
		}
	}
	dataDir := env("VOICE_DATA_DIR", filepath.Join(baseDir, "data"))
	staticDir := env("VOICE_STATIC_DIR", filepath.Join(baseDir, "static"))
	databaseFile := env("VOICE_DATABASE_FILE", filepath.Join(dataDir, "voice.db"))
	skipSSLVerify := envBool("VOICE_SKIP_SSL_VERIFY", false)
	if environment == EnvironmentProduction {
		skipSSLVerify = false
	}

	maxAttempts := envInt("VOICE_MAX_ACCOUNT_ATTEMPTS", 4)
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	transport := normalizeUpstreamTransport(env("VOICE_UPSTREAM_TRANSPORT", TransportTLSClient))

	// Scene worker settings stay on the gateway side (not vendor credentials).
	sceneConcurrency := envInt("VOICE_SCENE_GENERATION_CONCURRENCY", 2)
	if sceneConcurrency < 1 {
		sceneConcurrency = 1
	}
	sceneJobTimeout := envInt("VOICE_SCENE_REQUEST_TIMEOUT_SECONDS", 180)
	if sceneJobTimeout < 5 {
		sceneJobTimeout = 5
	}
	// Text orchestration uses the same request timeout as the worker by
	// default; image generation may be given a longer one.
	textRequestTimeout := envInt("VOICE_SCENE_REQUEST_TIMEOUT_SECONDS", 180)
	if textRequestTimeout < 5 {
		textRequestTimeout = 5
	}
	imageRequestTimeout := envInt("IMAGE_REQUEST_TIMEOUT_SECONDS", textRequestTimeout)
	if imageRequestTimeout < 5 {
		imageRequestTimeout = 5
	}
	// IMAGE_MAX_BYTES is the production cap. One documented compatibility read
	// of the legacy VOICE_SCENE_MAX_IMAGE_BYTES is kept for existing
	// deployments; the legacy variable is never advertised again.
	imageMaxBytes := envInt("IMAGE_MAX_BYTES", 0)
	if imageMaxBytes <= 0 {
		imageMaxBytes = envInt("VOICE_SCENE_MAX_IMAGE_BYTES", 32<<20)
	}
	if imageMaxBytes < 1<<20 {
		imageMaxBytes = 32 << 20
	}

	deviceID := env("VOICE_DEVICE_ID", "")
	if deviceID == "" {
		deviceID = uuid.New().String()
	}
	sessionID := env("VOICE_SESSION_ID", "")
	if sessionID == "" {
		sessionID = uuid.New().String()
	}

	return Config{
		Environment:            environment,
		DataDir:                dataDir,
		StaticDir:              staticDir,
		DatabaseFile:           databaseFile,
		AuthUsername:           env("VOICE_AUTH_USERNAME", ""),
		AuthPassword:           envRaw("VOICE_AUTH_PASSWORD", ""),
		AuthSessionTTL:         envInt("VOICE_AUTH_SESSION_TTL_SECONDS", 12*60*60),
		LoginMaxFailures:       envInt("VOICE_LOGIN_MAX_FAILURES", 8),
		LoginWindowSeconds:     envInt("VOICE_LOGIN_WINDOW_SECONDS", 15*60),
		LoginLockoutSeconds:    envInt("VOICE_LOGIN_LOCKOUT_SECONDS", 15*60),
		TurnstileSiteKey:       turnstileSiteKey,
		TurnstileSecretKey:     turnstileSecretKey,
		TrustCloudflareIP:      envBool("VOICE_TRUST_CLOUDFLARE_IP", false),
		PublicSessionRate:      envInt("VOICE_PUBLIC_SESSION_RATE_LIMIT_PER_MINUTE", 300),
		PublicWriteRate:        envInt("VOICE_PUBLIC_WRITE_RATE_LIMIT_PER_MINUTE", 3000),
		TokenEncryptionKey:     envRaw("VOICE_TOKEN_ENCRYPTION_KEY", ""),
		UpstreamTransport:      transport,
		TLSProfile:             env("VOICE_TLS_PROFILE", DefaultTLSProfile),
		Impersonate:            env("VOICE_IMPERSONATE", defaultImpersonate),
		CurlImpersonateBin:     env("VOICE_CURL_IMPERSONATE_BIN", ""),
		DeviceID:               deviceID,
		SessionID:              sessionID,
		SkipSSLVerify:          skipSSLVerify,
		SessionTTLSeconds:      envInt("VOICE_SESSION_TTL_SECONDS", 3*60),
		MaxAccountAttempts:     maxAttempts,
		DefaultUA:              env("VOICE_USER_AGENT", defaultUserAgent),
		SecCHUA:                env("VOICE_SEC_CH_UA", defaultSecCHUA),
		SecCHUAArch:            env("VOICE_SEC_CH_UA_ARCH", defaultSecCHUAArch),
		SecCHUABitness:         env("VOICE_SEC_CH_UA_BITNESS", defaultSecCHUABitness),
		SecCHUAFullVersion:     env("VOICE_SEC_CH_UA_FULL_VERSION", defaultSecCHUAFullVersion),
		SecCHUAFullVersionList: env("VOICE_SEC_CH_UA_FULL_VERSION_LIST", defaultSecCHUAFullVersionList),
		SecCHUAMobile:          env("VOICE_SEC_CH_UA_MOBILE", defaultSecCHUAMobile),
		SecCHUAModel:           env("VOICE_SEC_CH_UA_MODEL", defaultSecCHUAModel),
		SecCHUAPlatform:        env("VOICE_SEC_CH_UA_PLATFORM", defaultSecCHUAPlatform),
		SecCHUAPlatformVersion: env("VOICE_SEC_CH_UA_PLATFORM_VERSION", defaultSecCHUAPlatformVersion),
		ClientVersion:          env("VOICE_CLIENT_VERSION", defaultClientVersion),
		ClientBuildNumber:      env("VOICE_CLIENT_BUILD_NUMBER", defaultClientBuildNumber),
		// Bind all interfaces so Windows / VS Code port-forward can reach WSL.
		ListenAddr:  env("VOICE_LISTEN_ADDR", "0.0.0.0:8090"),
		TLS:         envBool("VOICE_TLS", false),
		TLSCertFile: env("VOICE_TLS_CERT", ""),
		TLSKeyFile:  env("VOICE_TLS_KEY", ""),
		TLSCertDir:  env("VOICE_TLS_CERT_DIR", filepath.Join(dataDir, "certs")),
		LogFormat:   env("VOICE_LOG_FORMAT", "json"),
		LogLevel:    env("VOICE_LOG_LEVEL", "info"),
		SceneText: SceneTextConfig{
			BaseURL:        env("VOICE_SCENE_AI_BASE_URL", "https://api.openai.com/v1"),
			APIKey:         envRaw("VOICE_SCENE_AI_API_KEY", ""),
			Model:          env("VOICE_SCENE_TEXT_MODEL", "gpt-4o-mini"),
			RequestTimeout: textRequestTimeout,
		},
		SceneImage: SceneImageConfig{
			BaseURL:        env("IMAGE_API_BASE_URL", "https://api.openai.com"),
			APIKey:         envRaw("IMAGE_API_KEY", ""),
			Model:          env("IMAGE_MODEL", "gpt-image-2"),
			RequestTimeout: imageRequestTimeout,
			MaxImageBytes:  int64(imageMaxBytes),
		},
		SceneWorker: SceneWorkerConfig{
			GenerationConcurrency: sceneConcurrency,
			RequestTimeout:        sceneJobTimeout,
		},
	}
}

// Validate rejects configurations that would leave protected content exposed.
func (c Config) Validate() error {
	switch strings.ToLower(strings.TrimSpace(c.Environment)) {
	case EnvironmentDevelopment, EnvironmentProduction:
	default:
		return fmt.Errorf("VOICE_ENV must be development or production")
	}
	if strings.TrimSpace(c.AuthUsername) == "" {
		return fmt.Errorf("VOICE_AUTH_USERNAME is required")
	}
	if c.AuthPassword == "" {
		return fmt.Errorf("VOICE_AUTH_PASSWORD is required")
	}
	if strings.TrimSpace(c.TokenEncryptionKey) == "" {
		return fmt.Errorf("VOICE_TOKEN_ENCRYPTION_KEY is required (32-byte hex or base64 key for sealing access tokens)")
	}
	if c.AuthSessionTTL < 1 {
		return fmt.Errorf("VOICE_AUTH_SESSION_TTL_SECONDS must be greater than zero")
	}
	if c.LoginMaxFailures < 1 {
		return fmt.Errorf("VOICE_LOGIN_MAX_FAILURES must be greater than zero")
	}
	if c.LoginWindowSeconds < 1 {
		return fmt.Errorf("VOICE_LOGIN_WINDOW_SECONDS must be greater than zero")
	}
	if c.LoginLockoutSeconds < 1 {
		return fmt.Errorf("VOICE_LOGIN_LOCKOUT_SECONDS must be greater than zero")
	}
	if c.PublicSessionRate < 0 {
		return fmt.Errorf("VOICE_PUBLIC_SESSION_RATE_LIMIT_PER_MINUTE must not be negative")
	}
	if c.PublicWriteRate < 0 {
		return fmt.Errorf("VOICE_PUBLIC_WRITE_RATE_LIMIT_PER_MINUTE must not be negative")
	}
	if strings.EqualFold(strings.TrimSpace(c.Environment), EnvironmentProduction) {
		if strings.TrimSpace(c.TurnstileSiteKey) == "" {
			return fmt.Errorf("VOICE_TURNSTILE_SITE_KEY is required in production")
		}
		if strings.TrimSpace(c.TurnstileSecretKey) == "" {
			return fmt.Errorf("VOICE_TURNSTILE_SECRET_KEY is required in production")
		}
	}
	if strings.TrimSpace(c.DatabaseFile) == "" {
		return fmt.Errorf("VOICE_DATABASE_FILE is required")
	}
	transport := normalizeUpstreamTransport(c.UpstreamTransport)
	switch transport {
	case TransportTLSClient, TransportCurlImpersonate, TransportGo:
	default:
		return fmt.Errorf("VOICE_UPSTREAM_TRANSPORT must be tls-client, curl-impersonate, or go")
	}
	if transport == TransportCurlImpersonate && strings.TrimSpace(c.Impersonate) == "" {
		return fmt.Errorf("VOICE_IMPERSONATE is required when using curl-impersonate transport")
	}
	if transport == TransportTLSClient && strings.TrimSpace(c.TLSProfile) == "" && strings.TrimSpace(c.Impersonate) == "" {
		return fmt.Errorf("VOICE_TLS_PROFILE is required when using tls-client transport")
	}
	if strings.TrimSpace(c.DeviceID) == "" {
		return fmt.Errorf("VOICE_DEVICE_ID must not be empty")
	}
	if strings.TrimSpace(c.SessionID) == "" {
		return fmt.Errorf("VOICE_SESSION_ID must not be empty")
	}
	if strings.EqualFold(strings.TrimSpace(c.Environment), EnvironmentProduction) && c.SkipSSLVerify {
		return fmt.Errorf("production forbids VOICE_SKIP_SSL_VERIFY=true")
	}
	format := strings.ToLower(strings.TrimSpace(c.LogFormat))
	if format != "json" && format != "text" {
		return fmt.Errorf("VOICE_LOG_FORMAT must be json or text")
	}
	level := strings.ToLower(strings.TrimSpace(c.LogLevel))
	if level != "debug" && level != "info" && level != "warn" && level != "error" {
		return fmt.Errorf("VOICE_LOG_LEVEL must be debug, info, warn, or error")
	}
	return nil
}
