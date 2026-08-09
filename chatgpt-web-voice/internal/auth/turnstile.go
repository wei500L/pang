package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const turnstileVerifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"

// TurnstileVerifier is injectable so authentication tests never need network
// access and deployments can fail closed when Cloudflare is unavailable.
type TurnstileVerifier interface {
	SiteKey() string
	Verify(ctx context.Context, token, remoteIP string) (bool, error)
}

type CloudflareTurnstile struct {
	siteKey   string
	secretKey string
	client    *http.Client
}

func NewCloudflareTurnstile(siteKey, secretKey string, client *http.Client) *CloudflareTurnstile {
	if client == nil {
		client = &http.Client{Timeout: 8 * time.Second}
	}
	return &CloudflareTurnstile{
		siteKey:   strings.TrimSpace(siteKey),
		secretKey: strings.TrimSpace(secretKey),
		client:    client,
	}
}

func (v *CloudflareTurnstile) SiteKey() string {
	if v == nil {
		return ""
	}
	return v.siteKey
}

func (v *CloudflareTurnstile) Verify(ctx context.Context, token, remoteIP string) (bool, error) {
	if v == nil || v.secretKey == "" {
		return false, fmt.Errorf("turnstile secret is not configured")
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return false, nil
	}
	form := url.Values{
		"secret":   {v.secretKey},
		"response": {token},
	}
	if remoteIP = strings.TrimSpace(remoteIP); remoteIP != "" && remoteIP != "unknown" {
		form.Set("remoteip", remoteIP)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, turnstileVerifyURL, strings.NewReader(form.Encode()))
	if err != nil {
		return false, fmt.Errorf("create turnstile request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := v.client.Do(req)
	if err != nil {
		return false, fmt.Errorf("verify turnstile: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("turnstile returned status %d", resp.StatusCode)
	}
	var result struct {
		Success bool `json:"success"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&result); err != nil {
		return false, fmt.Errorf("decode turnstile response: %w", err)
	}
	return result.Success, nil
}
