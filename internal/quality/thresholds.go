package quality

import "fmt"

// Thresholds defines minimum acceptable image quality. Zero values disable the
// corresponding check.
type Thresholds struct {
	MinPSNR       float64
	MinSSIM       float64
	MaxMeanDeltaE float64
	MaxP95DeltaE  float64
}

// Check returns a descriptive error when a report violates a threshold.
func (t Thresholds) Check(r Report) error {
	if t.MinPSNR > 0 && r.PSNR < t.MinPSNR {
		return fmt.Errorf("quality: PSNR %.4f < %.4f", r.PSNR, t.MinPSNR)
	}
	if t.MinSSIM > 0 && r.SSIM < t.MinSSIM {
		return fmt.Errorf("quality: SSIM %.6f < %.6f", r.SSIM, t.MinSSIM)
	}
	if t.MaxMeanDeltaE > 0 && r.MeanDeltaE > t.MaxMeanDeltaE {
		return fmt.Errorf("quality: mean DeltaE %.4f > %.4f", r.MeanDeltaE, t.MaxMeanDeltaE)
	}
	if t.MaxP95DeltaE > 0 && r.P95DeltaE > t.MaxP95DeltaE {
		return fmt.Errorf("quality: p95 DeltaE %.4f > %.4f", r.P95DeltaE, t.MaxP95DeltaE)
	}
	return nil
}
