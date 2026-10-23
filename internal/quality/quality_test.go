package quality

import (
	"image"
	"image/color"
	"math"
	"testing"
)

func solid(c color.Color) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, c)
		}
	}
	return img
}

func TestCompareIdentical(t *testing.T) {
	img := solid(color.RGBA{R: 100, G: 150, B: 200, A: 255})
	r, err := Compare(img, img)
	if err != nil {
		t.Fatal(err)
	}
	if !math.IsInf(r.PSNR, 1) || math.Abs(r.SSIM-1) > 1e-9 || r.MeanDeltaE != 0 || r.P95DeltaE != 0 {
		t.Fatalf("report = %#v", r)
	}
}

func TestCompareDifference(t *testing.T) {
	a := solid(color.RGBA{R: 255, A: 255})
	b := solid(color.RGBA{G: 255, A: 255})
	r, err := Compare(a, b)
	if err != nil {
		t.Fatal(err)
	}
	if math.IsInf(r.PSNR, 1) || r.PSNR <= 0 || r.SSIM >= 1 || r.MeanDeltaE <= 0 {
		t.Fatalf("report = %#v", r)
	}
}
