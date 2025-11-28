package main

import (
	"errors"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"net/http"
	"time"

	"golang.org/x/image/draw"
)

const (
	floofTargetWidth          = 256
	furBrightnessThreshold    = 0.55
	furSaturationThreshold    = 0.25
	textureNormalizationRange = 0.2
	furAreaWeight             = 0.65
	furTextureWeight          = 0.35
)

type FloofMajestyEvaluator struct {
	Store  *FloofMajestyStore
	client *http.Client
}

func NewFloofMajestyEvaluator(store *FloofMajestyStore) *FloofMajestyEvaluator {
	return &FloofMajestyEvaluator{
		Store: store,
		client: &http.Client{
			Timeout: 45 * time.Second,
		},
	}
}

func (e *FloofMajestyEvaluator) Score(imageURL string) (float64, error) {
	if e == nil || e.Store == nil {
		return 0, errors.New("floof evaluator not configured")
	}
	if imageURL == "" {
		return 0, errors.New("image URL cannot be empty")
	}

	if score, ok := e.Store.Get(imageURL); ok {
		return score, nil
	}

	img, err := e.fetchImage(imageURL)
	if err != nil {
		return 0, err
	}

	normalized := resizeForFloof(img)
	score := calculateFloofMajesty(normalized)

	if err := e.Store.Set(imageURL, score); err != nil {
		return 0, err
	}

	return score, nil
}

func (e *FloofMajestyEvaluator) fetchImage(imageURL string) (image.Image, error) {
	resp, err := e.client.Get(imageURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusBadRequest {
		return nil, errors.New("failed to download image")
	}

	img, _, err := image.Decode(resp.Body)
	if err != nil {
		return nil, err
	}

	return img, nil
}

func resizeForFloof(src image.Image) *image.NRGBA {
	bounds := src.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	if width == 0 || height == 0 {
		return image.NewNRGBA(image.Rect(0, 0, 1, 1))
	}

	targetWidth := floofTargetWidth
	if width < targetWidth {
		targetWidth = width
	}
	if targetWidth < 1 {
		targetWidth = 1
	}

	aspect := float64(height) / float64(width)
	targetHeight := int(math.Max(1, math.Round(float64(targetWidth)*aspect)))

	dst := image.NewNRGBA(image.Rect(0, 0, targetWidth, targetHeight))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, bounds, draw.Over, nil)
	return dst
}

func calculateFloofMajesty(img *image.NRGBA) float64 {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	totalPixels := width * height
	if totalPixels == 0 {
		return 0
	}

	grayscale := make([]float64, totalPixels)
	furMask := make([]bool, totalPixels)

	var furPixels int

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			idx := y*width + x
			r, g, b, _ := img.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
			rf := float64(r) / 65535.0
			gf := float64(g) / 65535.0
			bf := float64(b) / 65535.0

			maxv := math.Max(rf, math.Max(gf, bf))
			minv := math.Min(rf, math.Min(gf, bf))
			brightness := maxv
			saturation := 0.0
			if maxv > 0 {
				saturation = (maxv - minv) / maxv
			}

			gray := 0.299*rf + 0.587*gf + 0.114*bf
			grayscale[idx] = gray

			if brightness >= furBrightnessThreshold && saturation <= furSaturationThreshold {
				furMask[idx] = true
				furPixels++
			}
		}
	}

	if furPixels == 0 {
		return 0
	}

	var gradientSum float64
	var gradientCount int

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			idx := y*width + x
			if !furMask[idx] {
				continue
			}
			if x+1 < width {
				neighbourIdx := y*width + (x + 1)
				if furMask[neighbourIdx] {
					gradientSum += math.Abs(grayscale[idx] - grayscale[neighbourIdx])
					gradientCount++
				}
			}
			if y+1 < height {
				neighbourIdx := (y+1)*width + x
				if furMask[neighbourIdx] {
					gradientSum += math.Abs(grayscale[idx] - grayscale[neighbourIdx])
					gradientCount++
				}
			}
		}
	}

	var textureScore float64
	if gradientCount > 0 {
		avgDiff := gradientSum / float64(gradientCount)
		textureScore = clamp(avgDiff/textureNormalizationRange, 0, 1)
	}

	furFraction := float64(furPixels) / float64(totalPixels)
	combined := (furFraction * furAreaWeight) + (textureScore * furTextureWeight)
	return clamp(combined, 0, 1)
}

func clamp(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
