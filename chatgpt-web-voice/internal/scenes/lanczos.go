package scenes

import (
	"bytes"
	"errors"
	"image"
	"image/draw"
	"image/jpeg"
	"image/png"
	"math"

	// Register the WebP decoder with image.Decode. golang.org/x/image provides
	// no WebP encoder, so WebP inputs are re-encoded as PNG after scaling.
	_ "golang.org/x/image/webp"
)

// encodeNormalizedImage deterministically scales an already-decoded image
// (PNG/JPEG/WebP, aspect ratio and pixel caps validated by the caller) to
// exactly TargetImageWidth x TargetImageHeight using a Lanczos-3 kernel, then
// re-encodes it. It must not decode again: the caller performs the single,
// safety-checked decode.
//
// Encoding preserves the input format for PNG and JPEG; WebP has no encoder in
// the chosen dependency set, so WebP is normalized to PNG (never faked as
// WebP). The returned MIME and the file extension derived from it always match
// the real encoded bytes.
func encodeNormalizedImage(img image.Image, mime string) ([]byte, string, error) {
	resized := lanczosResize(img, TargetImageWidth, TargetImageHeight)

	var buffer bytes.Buffer
	switch mime {
	case "image/png":
		if err := png.Encode(&buffer, resized); err != nil {
			return nil, "", err
		}
		return buffer.Bytes(), "image/png", nil
	case "image/jpeg":
		// Stable high-quality parameters; the same input always yields the
		// same output bytes for the same Go version.
		if err := jpeg.Encode(&buffer, resized, &jpeg.Options{Quality: 90}); err != nil {
			return nil, "", err
		}
		return buffer.Bytes(), "image/jpeg", nil
	case "image/webp":
		// No reliable WebP encoder in the dependency set: normalize to PNG and
		// report PNG so bytes, extension and MIME stay consistent.
		if err := png.Encode(&buffer, resized); err != nil {
			return nil, "", err
		}
		return buffer.Bytes(), "image/png", nil
	default:
		return nil, "", errors.New("unsupported source format")
	}
}

// lanczosResize resizes src to dstW x dstH with a deterministic two-pass
// separable Lanczos-3 kernel operating on non-premultiplied RGBA samples.
// No cropping, no stretching of wrong-aspect inputs (callers validate the
// aspect ratio first), and no orientation changes.
func lanczosResize(src image.Image, dstW, dstH int) *image.NRGBA {
	bounds := src.Bounds()
	srcW, srcH := bounds.Dx(), bounds.Dy()
	if srcW <= 0 || srcH <= 0 || dstW <= 0 || dstH <= 0 {
		return image.NewNRGBA(image.Rect(0, 0, dstW, dstH))
	}

	srcRGBA := image.NewNRGBA(image.Rect(0, 0, srcW, srcH))
	draw.Draw(srcRGBA, srcRGBA.Bounds(), src, bounds.Min, draw.Src)

	// Horizontal pass: scale every row from srcW to dstW columns.
	horizontal := image.NewNRGBA(image.Rect(0, 0, dstW, srcH))
	scaleX := float64(srcW) / float64(dstW)
	for y := 0; y < srcH; y++ {
		row := srcRGBA.Pix[y*srcW*4 : (y+1)*srcW*4]
		outRow := horizontal.Pix[y*dstW*4 : (y+1)*dstW*4]
		for x := 0; x < dstW; x++ {
			r, g, b, a := resample1D(row, srcW, (float64(x)+0.5)*scaleX, scaleX)
			idx := x * 4
			outRow[idx] = clampChannel(r)
			outRow[idx+1] = clampChannel(g)
			outRow[idx+2] = clampChannel(b)
			outRow[idx+3] = clampChannel(a)
		}
	}

	// Vertical pass: scale every column from srcH to dstH rows.
	out := image.NewNRGBA(image.Rect(0, 0, dstW, dstH))
	scaleY := float64(srcH) / float64(dstH)
	for x := 0; x < dstW; x++ {
		for y := 0; y < dstH; y++ {
			var r, g, b, a float64
			center := (float64(y) + 0.5) * scaleY
			radius := int(math.Ceil(3 * scaleY))
			start := int(math.Floor(center)) - radius
			end := int(math.Floor(center)) + radius + 1
			var weightSum float64
			for yi := start; yi <= end; yi++ {
				if yi < 0 || yi >= srcH {
					continue
				}
				w := lanczos3((float64(yi) + 0.5 - center) / scaleY)
				if w == 0 {
					continue
				}
				idx := (yi*dstW + x) * 4
				r += w * float64(horizontal.Pix[idx])
				g += w * float64(horizontal.Pix[idx+1])
				b += w * float64(horizontal.Pix[idx+2])
				a += w * float64(horizontal.Pix[idx+3])
				weightSum += w
			}
			if weightSum == 0 {
				weightSum = 1
			}
			idx := (y*dstW + x) * 4
			out.Pix[idx] = clampChannel(r / weightSum)
			out.Pix[idx+1] = clampChannel(g / weightSum)
			out.Pix[idx+2] = clampChannel(b / weightSum)
			out.Pix[idx+3] = clampChannel(a / weightSum)
		}
	}
	return out
}

// resample1D computes the Lanczos-3 weighted sample of one horizontal RGBA row
// at the given source-space center (already in source pixels), with scale
// defined as sourcePixelsPerOutputPixel.
func resample1D(row []byte, srcWidth int, center float64, scale float64) (r, g, b, a float64) {
	radius := int(math.Ceil(3 * scale))
	start := int(math.Floor(center)) - radius
	end := int(math.Floor(center)) + radius + 1
	var weightSum float64
	for xi := start; xi <= end; xi++ {
		if xi < 0 || xi >= srcWidth {
			continue
		}
		w := lanczos3((float64(xi) + 0.5 - center) / scale)
		if w == 0 {
			continue
		}
		idx := xi * 4
		r += w * float64(row[idx])
		g += w * float64(row[idx+1])
		b += w * float64(row[idx+2])
		a += w * float64(row[idx+3])
		weightSum += w
	}
	if weightSum == 0 {
		weightSum = 1
	}
	return r / weightSum, g / weightSum, b / weightSum, a / weightSum
}

// lanczos3 is the Lanczos kernel with a=3. Deterministic pure math.
func lanczos3(x float64) float64 {
	ax := math.Abs(x)
	if ax >= 3 {
		return 0
	}
	if x == 0 {
		return 1
	}
	px := math.Pi * x
	return 3 * math.Sin(px) * math.Sin(px/3) / (px * px)
}

func clampChannel(value float64) uint8 {
	if value <= 0 {
		return 0
	}
	if value >= 255 {
		return 255
	}
	return uint8(math.Round(value))
}
