package auth

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestCloudflareTurnstileVerification(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != turnstileVerifyURL {
			t.Fatalf("verify url=%q", req.URL.String())
		}
		if err := req.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if req.Form.Get("secret") != "secret-key" || req.Form.Get("response") != "token" || req.Form.Get("remoteip") != "203.0.113.5" {
			t.Fatalf("unexpected verification form: %v", req.Form)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"success":true}`)),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})}
	verifier := NewCloudflareTurnstile("site-key", "secret-key", client)
	ok, err := verifier.Verify(context.Background(), "token", "203.0.113.5")
	if err != nil || !ok {
		t.Fatalf("verify ok=%v err=%v", ok, err)
	}
	if verifier.SiteKey() != "site-key" {
		t.Fatalf("site key=%q", verifier.SiteKey())
	}
}

func TestCloudflareTurnstileRejectsMissingTokenWithoutRequest(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("missing token must not call Cloudflare")
		return nil, nil
	})}
	ok, err := NewCloudflareTurnstile("site", "secret", client).Verify(context.Background(), "", "")
	if err != nil || ok {
		t.Fatalf("missing token ok=%v err=%v", ok, err)
	}
}
