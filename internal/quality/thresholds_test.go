package quality

import "testing"

func TestThresholds(t *testing.T) {
	r := Report{PSNR: 40, SSIM: 0.99, MeanDeltaE: 1, P95DeltaE: 2}
	if err := (Thresholds{MinPSNR: 35, MinSSIM: 0.98, MaxMeanDeltaE: 2, MaxP95DeltaE: 3}).Check(r); err != nil {
		t.Fatal(err)
	}
	if err := (Thresholds{MinSSIM: 0.999}).Check(r); err == nil {
		t.Fatal("expected SSIM failure")
	}
}
