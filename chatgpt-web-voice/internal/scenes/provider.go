package scenes

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"image"
	"io"
	"math"
	"net/http"
	"strings"
	"time"
)

// Provider family names recorded on scene rows / logs.
const (
	// TextProviderName identifies the chat-completions text orchestrator.
	TextProviderName = "openai-compatible"
	// ImageProviderName identifies the OpenAI Images API adapter.
	ImageProviderName = "openai-images"
)

// Target canvas of the current landscape product.
const (
	TargetImageWidth  = 1536
	TargetImageHeight = 1024
	// targetAspectRatio = TargetImageWidth / TargetImageHeight (3:2).
	targetAspectRatio = float64(TargetImageWidth) / float64(TargetImageHeight)
	// maxAspectRelativeError allows 0.5% deviation before rejection.
	maxAspectRelativeError = 0.005
)

// Pixel-safety caps for decoded images. Every generated image, including ones
// whose header claims the exact target size, must survive a full decode within
// these bounds; otherwise a decompression bomb could exhaust memory even when
// the compressed payload is small.
const (
	MaxDecodedImageWidth  = 8192
	MaxDecodedImageHeight = 8192
	MaxDecodedImagePixels = 40_000_000
)

// ErrProviderResponse is a sanitized provider error (no keys, no Authorization,
// no full bodies, no prompts, no temp URLs, no local paths).
type ErrProviderResponse struct {
	Message string
}

func (e *ErrProviderResponse) Error() string { return e.Message }

// joinV1Endpoint normalizes a provider base URL that may or may not end with
// "/v1" and appends the given "/v1/..." path exactly once. Both
// "https://example.com" and "https://example.com/v1" resolve to
// "https://example.com/v1/images/generations" — never "/v1/v1/..." or
// "//v1/...".
func joinV1Endpoint(base string, path string) (string, error) {
	base = strings.TrimSpace(base)
	if base == "" {
		return "", &Error{Message: "provider base URL is empty"}
	}
	if !strings.HasPrefix(base, "http://") && !strings.HasPrefix(base, "https://") {
		return "", &Error{Message: "provider base URL must be http(s)"}
	}
	base = strings.TrimRight(base, "/")
	if strings.HasSuffix(base, "/v1") {
		base = strings.TrimRight(strings.TrimSuffix(base, "/v1"), "/")
	}
	path = strings.TrimSpace(path)
	if path == "" || !strings.HasPrefix(path, "/") {
		return "", &Error{Message: "provider endpoint path is invalid"}
	}
	return base + "/v1" + path, nil
}

// retryableStatus reports whether an HTTP status may be transient enough for
// exactly one transport-level retry. Deterministic client/authorization errors
// (400, 401, 403, 404, ...) never retry.
func retryableStatus(status int) bool {
	switch status {
	case http.StatusRequestTimeout, http.StatusConflict, http.StatusTooManyRequests,
		http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

// retryBackoff sleeps a short, cancellable delay before the single retry.
// Returns false when the context is done.
func retryBackoff(ctx context.Context, attempt int) bool {
	delay := time.Duration(500*attempt) * time.Millisecond
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// doProviderRequest performs one provider call, enforces a response body cap,
// and maps transport failures to sanitized errors. The caller owns retry
// decisions; retries must rebuild the request body each attempt.
func doProviderRequest(ctx context.Context, client *http.Client, req *http.Request, bodyLimit int64) (int, []byte, error) {
	resp, err := client.Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return 0, nil, &ErrProviderResponse{Message: "provider request timed out"}
		}
		return 0, nil, &ErrProviderResponse{Message: "provider request failed"}
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, bodyLimit))
	if readErr != nil {
		return 0, nil, &ErrProviderResponse{Message: "provider response read failed"}
	}
	return resp.StatusCode, body, nil
}

// decodeImageDataURL parses "data:image/...;base64,...." payloads. It returns
// ok=false when the value is not a base64 data URL. The declared MIME inside
// the data URL is intentionally ignored; real format is decided by magic bytes.
func decodeImageDataURL(value string) (raw []byte, ok bool) {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "data:") {
		return nil, false
	}
	comma := strings.IndexByte(value, ',')
	if comma < 0 {
		return nil, false
	}
	meta := value[:comma]
	if !strings.Contains(meta, ";base64") {
		return nil, false
	}
	decoded, err := base64.StdEncoding.DecodeString(value[comma+1:])
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(value[comma+1:])
		if err != nil {
			return nil, false
		}
	}
	return decoded, true
}

// decodeBase64Payload decodes a plain base64 value (with or without padding).
func decodeBase64Payload(value string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		raw, err = base64.RawStdEncoding.DecodeString(value)
		if err != nil {
			return nil, err
		}
	}
	return raw, nil
}

// validateAndNormalizeImage accepts PNG/JPEG/WebP payloads and performs the
// full safety pipeline:
//
//  1. non-empty payload and compressed-size cap,
//  2. magic-byte format identification,
//  3. image.DecodeConfig as the authoritative format and dimensions,
//  4. format consistency between magic bytes and DecodeConfig,
//  5. width/height > 0,
//  6. max dimension and pixel-count caps (int64 math, overflow-safe),
//  7. 3:2 aspect-ratio check within 0.5%,
//  8. complete image.Decode,
//  9. decoded bounds must match DecodeConfig,
//  10. exact 1536x1024 keeps the original bytes only after a successful full
//      decode; any other size is Lanczos-normalized from the already-decoded
//      image (no second decode).
//
// Aspect-ratio mismatches fail immediately: no stretch, crop, or letterbox.
func validateAndNormalizeImage(payload []byte, maxBytes int64) (ImageResult, error) {
	if len(payload) == 0 {
		return ImageResult{}, &ErrProviderResponse{Message: "image provider returned empty data"}
	}
	if int64(len(payload)) > maxBytes {
		return ImageResult{}, &ErrProviderResponse{Message: "generated image exceeds the size limit"}
	}
	mime, _, _, err := sniffImage(payload)
	if err != nil {
		return ImageResult{}, &ErrProviderResponse{Message: "generated image has an unsupported format"}
	}

	// DecodeConfig is the authoritative format/dimension source; the magic
	// bytes above are only the fast format gate.
	config, format, err := image.DecodeConfig(bytes.NewReader(payload))
	if err != nil {
		return ImageResult{}, &ErrProviderResponse{Message: "image provider returned an invalid image"}
	}
	if formatMIME(format) != mime {
		return ImageResult{}, &ErrProviderResponse{Message: "generated image has an unsupported format"}
	}
	width, height := config.Width, config.Height
	if width <= 0 || height <= 0 {
		return ImageResult{}, &ErrProviderResponse{Message: "image provider returned an invalid image"}
	}
	if width > MaxDecodedImageWidth || height > MaxDecodedImageHeight {
		return ImageResult{}, &ErrProviderResponse{Message: "image provider returned an invalid image"}
	}
	// int64 math keeps the product overflow-safe on 32-bit platforms.
	pixels := int64(width) * int64(height)
	if pixels > MaxDecodedImagePixels {
		return ImageResult{}, &ErrProviderResponse{Message: "image provider returned an invalid image"}
	}
	if !aspectRatioMatches(width, height) {
		return ImageResult{}, &ErrProviderResponse{
			Message: "generated image aspect ratio does not match 1536x1024",
		}
	}

	// Full decode: a truncated or header-only file (exact size or not) must be
	// rejected, and the decoded bounds must match the config.
	img, _, err := image.Decode(bytes.NewReader(payload))
	if err != nil {
		return ImageResult{}, &ErrProviderResponse{Message: "image provider returned an invalid image"}
	}
	bounds := img.Bounds()
	if bounds.Dx() != width || bounds.Dy() != height {
		return ImageResult{}, &ErrProviderResponse{Message: "image provider returned an invalid image"}
	}

	if width == TargetImageWidth && height == TargetImageHeight {
		// Exact target: keep the original valid bytes and format untouched,
		// but only because the payload already survived a full decode.
		return ImageResult{MIMEType: mime, Bytes: payload, Width: width, Height: height}, nil
	}
	normalized, outMIME, err := encodeNormalizedImage(img, mime)
	if err != nil {
		return ImageResult{}, &ErrProviderResponse{Message: "generated image normalization failed"}
	}
	return ImageResult{
		MIMEType: outMIME,
		Bytes:    normalized,
		Width:    TargetImageWidth,
		Height:   TargetImageHeight,
	}, nil
}

// formatMIME maps the image.DecodeConfig format string back to the MIME type
// used by the magic-byte gate. Unknown formats map to "" so the consistency
// check rejects them.
func formatMIME(format string) string {
	switch format {
	case "png":
		return "image/png"
	case "jpeg":
		return "image/jpeg"
	case "webp":
		return "image/webp"
	default:
		return ""
	}
}

// aspectRatioMatches compares width/height to 1536/1024 with a relative error
// tolerance of 0.5%.
func aspectRatioMatches(width, height int) bool {
	if width <= 0 || height <= 0 {
		return false
	}
	actualRatio := float64(width) / float64(height)
	relativeError := math.Abs(actualRatio-targetAspectRatio) / targetAspectRatio
	return relativeError <= maxAspectRelativeError
}

// sniffImage detects jpeg/png/webp by magic bytes and reads the header
// dimensions without decoding pixel data. It is the fast format gate of
// validateAndNormalizeImage; DecodeConfig remains the authoritative dimension
// source, so the header dims returned here are not cross-checked against it.
func sniffImage(data []byte) (mime string, width, height int, err error) {
	switch {
	case len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF:
		width, height, err = jpegDimensions(data)
		if err != nil {
			return "", 0, 0, err
		}
		return "image/jpeg", width, height, nil
	case len(data) >= 8 && bytes.Equal(data[:8], []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}):
		if len(data) < 24 {
			return "", 0, 0, errors.New("png too small")
		}
		return "image/png", int(binary.BigEndian.Uint32(data[16:20])), int(binary.BigEndian.Uint32(data[20:24])), nil
	case len(data) >= 12 && bytes.Equal(data[:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")):
		width, height, err = webpDimensions(data)
		if err != nil {
			return "", 0, 0, err
		}
		return "image/webp", width, height, nil
	default:
		return "", 0, 0, errors.New("unsupported image format")
	}
}

// uint24LE reads a 24-bit little-endian unsigned integer from at least three
// bytes. The caller must ensure the slice is long enough; a short slice is a
// truncated-header error.
func uint24LE(data []byte) (uint32, error) {
	if len(data) < 3 {
		return 0, errors.New("truncated 24-bit field")
	}
	return uint32(data[0]) |
		uint32(data[1])<<8 |
		uint32(data[2])<<16, nil
}

// jpegDimensions walks JPEG segments to the first SOF marker.
func jpegDimensions(data []byte) (int, int, error) {
	offset := 2
	for offset+9 < len(data) {
		if data[offset] != 0xFF {
			offset++
			continue
		}
		marker := data[offset+1]
		offset += 2
		if marker == 0xD8 || (marker >= 0xD0 && marker <= 0xD7) || marker == 0x01 {
			continue
		}
		if marker == 0xD9 || marker == 0xDA {
			return 0, 0, errors.New("jpeg SOF not found")
		}
		if offset+2 > len(data) {
			return 0, 0, errors.New("jpeg segment truncated")
		}
		length := int(binary.BigEndian.Uint16(data[offset : offset+2]))
		if length < 2 || offset+length > len(data) {
			return 0, 0, errors.New("jpeg segment invalid")
		}
		switch marker {
		case 0xC0, 0xC1, 0xC2, 0xC3, 0xC5, 0xC6, 0xC7, 0xC9, 0xCA, 0xCB, 0xCD, 0xCE, 0xCF:
			if length >= 7 {
				return int(binary.BigEndian.Uint16(data[offset+5 : offset+7])), int(binary.BigEndian.Uint16(data[offset+3 : offset+5])), nil
			}
			return 0, 0, errors.New("jpeg SOF truncated")
		}
		offset += length
	}
	return 0, 0, errors.New("jpeg dimensions not found")
}

// webpDimensions reads dimensions from VP8 / VP8L / VP8X chunks.
// RIFF chunk boundaries and odd-sized-chunk padding are respected; truncated
// chunks are rejected.
func webpDimensions(data []byte) (int, int, error) {
	offset := 12
	for offset+8 <= len(data) {
		chunk := string(data[offset : offset+4])
		size := int(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
		payload := offset + 8
		if payload+size > len(data) {
			return 0, 0, errors.New("webp chunk truncated")
		}
		switch chunk {
		case "VP8 ":
			// VP8 frame header: 3-byte frame tag, then width/height as
			// 14-bit little-endian values starting at payload+6.
			if size >= 10 {
				width := int(binary.LittleEndian.Uint16(data[payload+6:payload+8])) & 0x3FFF
				height := int(binary.LittleEndian.Uint16(data[payload+8:payload+10])) & 0x3FFF
				if width <= 0 || height <= 0 {
					return 0, 0, errors.New("webp VP8 dimensions invalid")
				}
				return width, height, nil
			}
		case "VP8L":
			// Lossless header: signature byte, then 14-bit width-1 and
			// 14-bit height-1 packed into 4 little-endian bytes.
			if size >= 5 {
				if data[payload] != 0x2F {
					return 0, 0, errors.New("webp VP8L signature invalid")
				}
				b := data[payload+1:]
				bits := uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
				width := int(bits&0x3FFF) + 1
				height := int((bits>>14)&0x3FFF) + 1
				if width <= 0 || height <= 0 {
					return 0, 0, errors.New("webp VP8L dimensions invalid")
				}
				return width, height, nil
			}
		case "VP8X":
			// Extended header: byte 0 = feature flags, bytes 1-3 reserved,
			// bytes 4-6 = 24-bit little-endian canvas width minus one,
			// bytes 7-9 = 24-bit little-endian canvas height minus one.
			// The flags and the dimensions must never share one uint32 read.
			if size >= 10 {
				widthMinusOne, err := uint24LE(data[payload+4 : payload+7])
				if err != nil {
					return 0, 0, err
				}
				heightMinusOne, err := uint24LE(data[payload+7 : payload+10])
				if err != nil {
					return 0, 0, err
				}
				width := int(widthMinusOne) + 1
				height := int(heightMinusOne) + 1
				if width <= 0 || height <= 0 {
					return 0, 0, errors.New("webp VP8X dimensions invalid")
				}
				return width, height, nil
			}
		}
		offset = payload + size + (size & 1)
	}
	return 0, 0, errors.New("webp dimensions not found")
}
