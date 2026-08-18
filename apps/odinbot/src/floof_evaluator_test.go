package main

import (
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestFloofEvaluatorPrefersRealPhoto(t *testing.T) {
	pxl, pxlMetrics := scoreFixture(t, "PXL_20251117_195038946.jpg")
	toilet, toiletMetrics := scoreFixture(t, "ToiletOdin.png")

	if pxl <= toilet {
		t.Fatalf("expected PXL photo to score higher than Toilet Odin illustration: pxl=%v (%+v) toilet=%v (%+v)", pxl, pxlMetrics, toilet, toiletMetrics)
	}
}

func scoreFixture(t *testing.T, filename string) (float64, floofMetrics) {
	t.Helper()

	path := filepath.Join("testdata", "floof", filename)
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("failed opening %s: %v", path, err)
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		t.Fatalf("failed decoding %s: %v", path, err)
	}

	normalized := resizeForFloof(img)
	metrics := calculateFloofMetrics(normalized)
	return calculateFloofMajesty(normalized), metrics
}
