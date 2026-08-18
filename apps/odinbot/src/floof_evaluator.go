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
	furTextureEdgeThreshold   = 0.005
	furAreaWeight             = 0.15
	furTextureWeight          = 0.2
	furCoverageWeight         = 0.65
)

type FloofMajestyEvaluator struct {
	Store  *FloofMajestyStore
	client *http.Client
}

type floofMetrics struct {
	furPixels       int
	totalPixels     int
	furFraction     float64
	textureScore    float64
	textureCoverage float64
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
	return e.scoreInternal(imageURL, false)
}

func (e *FloofMajestyEvaluator) ForceRefresh(imageURL string) (float64, error) {
	return e.scoreInternal(imageURL, true)
}

func (e *FloofMajestyEvaluator) scoreInternal(imageURL string, force bool) (float64, error) {
	if e == nil || e.Store == nil {
		return 0, errors.New("floof evaluator not configured")
	}
	if imageURL == "" {
		return 0, errors.New("image URL cannot be empty")
	}

	if !force {
		if entry, ok := e.Store.Get(imageURL); ok && entry.Version >= FloofScoreVersion {
			return entry.Score, nil
		}
	}

	img, err := e.fetchImage(imageURL)
	if err != nil {
		return 0, err
	}

	normalized := resizeForFloof(img)
	score := calculateFloofMajesty(normalized)

	if err := e.Store.Set(imageURL, score, FloofScoreVersion); err != nil {
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
	metrics := calculateFloofMetrics(img)
	combined := (metrics.furFraction * furAreaWeight) + (metrics.textureScore * furTextureWeight) + (metrics.textureCoverage * furCoverageWeight)
	return clamp(combined, 0, 1)
}

func calculateFloofMetrics(img *image.NRGBA) floofMetrics {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	totalPixels := width * height
	metrics := floofMetrics{totalPixels: totalPixels}
	if totalPixels == 0 {
		return metrics
	}

	grayscale := make([]float64, totalPixels)
	furMask := make([]bool, totalPixels)

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
				metrics.furPixels++
			}
		}
	}

	if metrics.furPixels == 0 {
		return metrics
	}

	var gradientSum float64
	var gradientCount int
	var activeEdges int

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			idx := y*width + x
			if !furMask[idx] {
				continue
			}
			if x+1 < width {
				neighbourIdx := y*width + (x + 1)
				if furMask[neighbourIdx] {
					diff := math.Abs(grayscale[idx] - grayscale[neighbourIdx])
					gradientSum += diff
					gradientCount++
					if diff >= furTextureEdgeThreshold {
						activeEdges++
					}
				}
			}
			if y+1 < height {
				neighbourIdx := (y+1)*width + x
				if furMask[neighbourIdx] {
					diff := math.Abs(grayscale[idx] - grayscale[neighbourIdx])
					gradientSum += diff
					gradientCount++
					if diff >= furTextureEdgeThreshold {
						activeEdges++
					}
				}
			}
		}
	}

	if gradientCount > 0 {
		avgDiff := gradientSum / float64(gradientCount)
		metrics.textureScore = clamp(avgDiff/textureNormalizationRange, 0, 1)
	}
	maxPossibleEdges := metrics.furPixels * 2
	if maxPossibleEdges > 0 {
		metrics.textureCoverage = clamp(float64(activeEdges)/float64(maxPossibleEdges), 0, 1)
	}
	metrics.furFraction = float64(metrics.furPixels) / float64(totalPixels)
	return metrics
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
