package scenes

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/dyhhhhhh/chatgpt-web-voice/internal/config"
)

// Fixed production business presets for the landscape scene canvas. These are
// intentionally not configurable: the current product always renders one
// 3:2 landscape frame at standard quality.
const (
	imageRequestSize    = "1536x1024"
	imageRequestQuality = "standard"
	imageRequestCount   = 1
)

// OpenAIImageProvider is the production OpenAI Images API adapter. It only
// calls POST /v1/images/generations with the fixed presets above and never
// sends response_format or output_format. It uses IMAGE_API_KEY and never
// reads, uses, or falls back to VOICE_SCENE_AI_API_KEY or any account-pool
// token.
//
// Reference-image edits are NOT implemented: the current product has no
// reference input. If a future ImageInput carries one or more reference images
// the adapter must switch to POST /v1/images/edits (multipart/form-data) with
// repeated `image` file fields whose upload order defines reference priority;
// reference images must never be base64-embedded into generations JSON and
// image models must never be called through /v1/chat/completions.
type OpenAIImageProvider struct {
	endpoint      string
	apiKey        string
	model         string
	maxImageBytes int64
	client        *http.Client
	// downloadClient is dedicated to temporary image URLs: https-only, no
	// private/link-local/metadata hosts, per-hop redirect validation, and
	// dial-time IP filtering against DNS rebinding. The generations endpoint
	// itself uses p.client, whose base URL is trusted operator configuration.
	downloadClient *http.Client
	resolver       *net.Resolver
	logger         *slog.Logger
}

// NewOpenAIImageProvider builds the OpenAI Images API adapter.
func NewOpenAIImageProvider(cfg config.SceneImageConfig, logger *slog.Logger) *OpenAIImageProvider {
	timeout := time.Duration(cfg.RequestTimeout) * time.Second
	if timeout <= 0 {
		timeout = 180 * time.Second
	}
	endpoint, err := joinV1Endpoint(cfg.BaseURL, "/images/generations")
	if err != nil {
		endpoint = ""
	}
	return &OpenAIImageProvider{
		endpoint:      endpoint,
		apiKey:        strings.TrimSpace(cfg.APIKey),
		model:         strings.TrimSpace(cfg.Model),
		maxImageBytes: cfg.MaxImageBytes,
		client: &http.Client{
			Timeout: timeout,
		},
		downloadClient: newSecureImageDownloadClient(timeout, nil),
		resolver:       net.DefaultResolver,
		logger:         logger,
	}
}

// Name returns the provider family recorded on scene rows / logs.
func (p *OpenAIImageProvider) Name() string { return ImageProviderName }

// Model returns the configured image model name.
func (p *OpenAIImageProvider) Model() string { return p.model }

// Generate renders one scene image. The JSON body contains exactly
// model/prompt/n/size/quality; every attempt rebuilds the body so a consumed
// reader is never reused. At most one transport retry is allowed for transient
// network errors and 408/409/429/500/502/503/504; deterministic client errors
// and all response-format/validation failures are never retried.
func (p *OpenAIImageProvider) Generate(ctx context.Context, input ImageInput) (ImageResult, error) {
	if p.endpoint == "" {
		return ImageResult{}, &ErrProviderResponse{Message: "image provider is misconfigured"}
	}
	payload := map[string]any{
		"model":   p.model,
		"prompt":  input.Prompt,
		"n":       imageRequestCount,
		"size":    imageRequestSize,
		"quality": imageRequestQuality,
	}

	bodyLimit := p.maxImageBytes * 2
	if bodyLimit < 8<<20 {
		bodyLimit = 8 << 20
	}

	for attempt := 1; attempt <= 2; attempt++ {
		raw, err := json.Marshal(payload)
		if err != nil {
			return ImageResult{}, &ErrProviderResponse{Message: "image request serialization failed"}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(raw))
		if err != nil {
			return ImageResult{}, &ErrProviderResponse{Message: "image request setup failed"}
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, image/*")
		if p.apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+p.apiKey)
		}

		status, body, doErr := doProviderRequest(ctx, p.client, req, bodyLimit)
		if doErr != nil {
			// Transient transport failure: one retry while the context is alive.
			if attempt < 2 && ctx.Err() == nil && retryBackoff(ctx, attempt) {
				continue
			}
			return ImageResult{}, doErr
		}
		if status != http.StatusOK {
			if retryableStatus(status) && attempt < 2 && retryBackoff(ctx, attempt) {
				continue
			}
			return ImageResult{}, &ErrProviderResponse{Message: fmt.Sprintf("image provider returned HTTP %d", status)}
		}

		result, err := p.parseImageResponse(ctx, body)
		if err != nil {
			// Format, aspect-ratio, size and decoding errors never retry.
			return ImageResult{}, err
		}
		return result, nil
	}
	return ImageResult{}, &ErrProviderResponse{Message: "image provider request failed"}
}

// parseImageResponse resolves the generated image with the following priority:
//
//  1. JSON data[0].b64_json
//  2. JSON data[0].b64
//  3. b64_json / b64 carrying a data:image/...;base64,.... data URL
//  4. JSON data[0].url — HTTPS only, downloaded server-side and stored locally;
//     the temporary provider URL is never persisted or returned.
//  5. The raw HTTP response body being the image bytes themselves.
//
// The declared MIME is never trusted: real format and dimensions come from
// magic bytes, and the payload is aspect-checked and Lanczos-normalized.
func (p *OpenAIImageProvider) parseImageResponse(ctx context.Context, body []byte) (ImageResult, error) {
	var envelope struct {
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
		Data []struct {
			B64JSON string `json:"b64_json"`
			B64     string `json:"b64"`
			URL     string `json:"url"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &envelope) == nil {
		if len(envelope.Data) == 0 {
			if envelope.Error != nil {
				return ImageResult{}, &ErrProviderResponse{Message: "image provider request failed"}
			}
			return ImageResult{}, &ErrProviderResponse{Message: "image provider returned no image"}
		}
		item := envelope.Data[0]
		for _, candidate := range []string{item.B64JSON, item.B64} {
			candidate = strings.TrimSpace(candidate)
			if candidate == "" {
				continue
			}
			raw, ok := decodeImageDataURL(candidate)
			if !ok {
				decoded, err := decodeBase64Payload(candidate)
				if err != nil {
					return ImageResult{}, &ErrProviderResponse{Message: "image provider returned invalid base64 data"}
				}
				raw = decoded
			}
			return validateAndNormalizeImage(raw, p.maxImageBytes)
		}
		if url := strings.TrimSpace(item.URL); url != "" {
			return p.downloadImage(ctx, url)
		}
		return ImageResult{}, &ErrProviderResponse{Message: "image provider returned no image"}
	}
	// Raw image bytes response. The Content-Type hint is deliberately not
	// trusted; the magic-byte sniff inside validation decides.
	result, err := validateAndNormalizeImage(body, p.maxImageBytes)
	if err != nil {
		return ImageResult{}, &ErrProviderResponse{Message: "image provider returned an invalid response"}
	}
	return result, nil
}

// downloadImage fetches a temporary HTTPS image URL and immediately stores it
// locally via validation+normalization. The URL is never persisted, returned
// to the public API, or logged. The dedicated secure client enforces the
// https-only / public-host policy on the initial URL and on every redirect
// hop, and re-validates DNS at dial time against rebinding.
func (p *OpenAIImageProvider) downloadImage(ctx context.Context, url string) (ImageResult, error) {
	parsed, err := validateRemoteImageURL(ctx, url, p.resolver)
	if err != nil {
		// Policy violations are reported distinctly; host-resolution failures
		// are plain transport failures. Neither leaks the URL.
		if errors.Is(err, errForbiddenImageURL) || errors.Is(err, errTooManyImageRedirects) {
			return ImageResult{}, &ErrProviderResponse{Message: "image provider returned an unsafe URL"}
		}
		return ImageResult{}, &ErrProviderResponse{Message: "image provider request failed"}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return ImageResult{}, &ErrProviderResponse{Message: "image provider request failed"}
	}
	resp, err := p.downloadClient.Do(req)
	if err != nil {
		// Safety-policy rejections (including redirects to private hosts) are
		// also surfaced here; the message must stay generic.
		if errors.Is(err, errForbiddenImageURL) || errors.Is(err, errTooManyImageRedirects) {
			return ImageResult{}, &ErrProviderResponse{Message: "image provider returned an unsafe URL"}
		}
		return ImageResult{}, &ErrProviderResponse{Message: "image provider request failed"}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
		return ImageResult{}, &ErrProviderResponse{Message: "image provider request failed"}
	}
	payload, err := io.ReadAll(io.LimitReader(resp.Body, p.maxImageBytes+1))
	if err != nil {
		return ImageResult{}, &ErrProviderResponse{Message: "image provider request failed"}
	}
	return validateAndNormalizeImage(payload, p.maxImageBytes)
}
