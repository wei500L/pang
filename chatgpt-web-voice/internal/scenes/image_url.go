package scenes

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// errForbiddenImageURL is the internal sentinel for any URL that violates the
// remote-image policy. It is mapped to the stable public message
// "image provider returned an unsafe URL" and never leaks the URL itself.
var errForbiddenImageURL = errors.New("forbidden image URL")

// errTooManyImageRedirects bounds redirect chains of temporary image URLs.
var errTooManyImageRedirects = errors.New("too many image URL redirects")

// errImageHostResolution is returned when a temporary-image host cannot be
// resolved. It is a transport-level failure, not a policy violation.
var errImageHostResolution = errors.New("image host resolution failed")

// maxImageRedirects is the maximum number of redirect hops allowed when
// downloading a temporary provider image URL.
const maxImageRedirects = 3

// validateRemoteImageURL enforces the remote-image download policy on one URL:
//
//   - parsed with net/url (never string prefixes),
//   - scheme must be exactly https,
//   - host must be non-empty,
//   - no userinfo,
//   - every A/AAAA address the host resolves to must be public (no loopback,
//     unspecified, link-local, multicast, private, ULA or metadata ranges).
//
// This is a pre-dial check; the actual dial (see dialRemoteImage) re-validates
// the resolution so a DNS-rebinding window cannot smuggle a private address
// past it. The returned URL carries no userinfo or query secrets of interest,
// but callers must still never log it.
func validateRemoteImageURL(ctx context.Context, raw string, resolver *net.Resolver) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, errForbiddenImageURL
	}
	if parsed.Scheme != "https" {
		return nil, errForbiddenImageURL
	}
	if parsed.Host == "" {
		return nil, errForbiddenImageURL
	}
	if parsed.User != nil {
		return nil, errForbiddenImageURL
	}
	if err := checkResolvedImageHost(ctx, parsed.Hostname(), resolver); err != nil {
		return nil, err
	}
	return parsed, nil
}

// checkResolvedImageHost rejects a host when any of its resolved addresses is
// forbidden. IP literals are checked directly without DNS.
func checkResolvedImageHost(ctx context.Context, host string, resolver *net.Resolver) error {
	host = strings.TrimSpace(host)
	if host == "" {
		return errForbiddenImageURL
	}
	if ip := net.ParseIP(host); ip != nil {
		if isForbiddenIP(ip) {
			return errForbiddenImageURL
		}
		return nil
	}
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	addrs, err := resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return errImageHostResolution
	}
	if len(addrs) == 0 {
		return errImageHostResolution
	}
	for _, addr := range addrs {
		if isForbiddenIP(addr.IP) {
			return errForbiddenImageURL
		}
	}
	return nil
}

// isForbiddenIP reports whether an IP must never be reached by the image
// downloader: loopback, unspecified, link-local unicast/multicast, multicast,
// RFC1918 private, IPv6 unique-local (fc00::/7), link-local v6 (fe80::/10) and
// the link-local IPv4 metadata range 169.254.0.0/16.
func isForbiddenIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsPrivate() {
		return true
	}
	if v4 := ip.To4(); v4 != nil {
		// 169.254.0.0/16 (IsLinkLocalUnicast already covers it; kept explicit).
		return v4[0] == 169 && v4[1] == 254
	}
	if v6 := ip.To16(); v6 != nil {
		// Unique-local fc00::/7.
		if v6[0]&0xfe == 0xfc {
			return true
		}
		// Link-local fe80::/10.
		return v6[0] == 0xfe && v6[1]&0xc0 == 0x80
	}
	return false
}

// dialRemoteImage is the DialContext used for temporary image URLs. It
// re-resolves the hostname at dial time, rejects any forbidden address, and
// dials the first allowed address directly. The TLS ServerName stays the
// original hostname because the request URL keeps it; this closes the
// DNS-rebinding window between the pre-dial validation and the actual connect.
func dialRemoteImage(ctx context.Context, dialer *net.Dialer, network, addr string, resolver *net.Resolver) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, errForbiddenImageURL
	}
	host = strings.Trim(host, "[]")
	if ip := net.ParseIP(host); ip != nil {
		if isForbiddenIP(ip) {
			return nil, errForbiddenImageURL
		}
		return dialer.DialContext(ctx, network, addr)
	}
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	addrs, err := resolver.LookupIPAddr(ctx, host)
	if err != nil || len(addrs) == 0 {
		return nil, errImageHostResolution
	}
	for _, resolved := range addrs {
		if isForbiddenIP(resolved.IP) {
			return nil, errForbiddenImageURL
		}
	}
	return dialer.DialContext(ctx, "tcp", net.JoinHostPort(addrs[0].IP.String(), port))
}

// newSecureImageDownloadClient builds an http.Client restricted to public
// HTTPS image hosts: per-hop redirect validation with a bounded chain, and
// dial-time IP filtering (the pre-dial resolution is re-validated on every
// connect, so a rebinding host cannot reach a private address).
func newSecureImageDownloadClient(timeout time.Duration, resolver *net.Resolver) *http.Client {
	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialRemoteImage(ctx, dialer, network, addr, resolver)
		},
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   15 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	return &http.Client{
		Timeout: timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxImageRedirects {
				return errTooManyImageRedirects
			}
			// Every hop must satisfy the same policy: https, public host.
			if _, err := validateRemoteImageURL(req.Context(), req.URL.String(), resolver); err != nil {
				return err
			}
			return nil
		},
	}
}
