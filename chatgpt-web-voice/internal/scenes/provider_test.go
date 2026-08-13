package scenes

import (
	"bytes"
	"image"
	"image/png"
	"testing"
)

func TestAspectRatioMatches(t *testing.T) {
	cases := []struct {
		name   string
		width  int
		height int
		want   bool
	}{
		{"exact 3:2 target", 1536, 1024, true},
		{"3:2 landscape", 3072, 2048, true},
		{"3:2 within 0.5%", 1401, 934, true},
		{"exact 5:4 relay output", 1402, 1122, true},
		{"5:4 square-ish relay variant", 1024, 819, true},
		{"5:4 landscape", 1250, 1000, true},
		{"16:9 wide", 1792, 1008, false},
		{"panoramic", 1774, 887, false},
		{"portrait 3:2", 1024, 1536, false},
		{"square", 1024, 1024, false},
		{"zero width", 0, 1024, false},
		{"zero height", 1536, 0, false},
		{"negative", -1536, 1024, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := aspectRatioMatches(tc.width, tc.height); got != tc.want {
				t.Fatalf("aspectRatioMatches(%d, %d) = %v, want %v", tc.width, tc.height, got, tc.want)
			}
		})
	}
}

// TestValidateAndNormalizeImage5x4 verifies the full safety pipeline accepts a
// 5:4 relay output (e.g. 1402x1122) and normalizes it to the exact 1536x1024
// target.
func TestValidateAndNormalizeImage5x4(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 1402, 1122))
	var buf bytes.Buffer
	if err := png.Encode(&buf, src); err != nil {
		t.Fatalf("encode source: %v", err)
	}
	result, err := validateAndNormalizeImage(buf.Bytes(), 32<<20)
	if err != nil {
		t.Fatalf("validateAndNormalizeImage: %v", err)
	}
	if result.Width != TargetImageWidth || result.Height != TargetImageHeight {
		t.Fatalf("normalized size = %dx%d, want %dx%d", result.Width, result.Height, TargetImageWidth, TargetImageHeight)
	}
	if result.MIMEType != "image/png" {
		t.Fatalf("MIME = %q, want image/png", result.MIMEType)
	}
}

// TestValidateAndNormalizeImageRejects16x9 verifies 16:9 inputs are still
// rejected outright.
func TestValidateAndNormalizeImageRejects16x9(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 1792, 1008))
	var buf bytes.Buffer
	if err := png.Encode(&buf, src); err != nil {
		t.Fatalf("encode source: %v", err)
	}
	if _, err := validateAndNormalizeImage(buf.Bytes(), 32<<20); err == nil {
		t.Fatal("expected aspect-ratio rejection, got nil")
	}
}
