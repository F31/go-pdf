package cairo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/F31/go-pdf/extension"
	"github.com/F31/go-pdf/internal/procutil"
)

// Renderer invokes pdftocairo. It is safe for concurrent use when its exported
// fields are not mutated after construction.
type Renderer struct {
	Binary   string
	Password string
	// MaxOutputBytes limits the produced SVG; zero uses 64 MiB.
	MaxOutputBytes int64
}

// New returns a renderer using binary (or pdftocairo when empty).
func New(binary string) (*Renderer, error) {
	if binary == "" {
		binary = "pdftocairo"
	}
	if _, err := exec.LookPath(binary); err != nil {
		return nil, fmt.Errorf("cairo: executable %q: %w", binary, err)
	}
	return &Renderer{Binary: binary}, nil
}

// Version returns the first line of pdftocairo -v output.
func (r *Renderer) Version(ctx context.Context) (string, error) {
	if r == nil {
		return "", errors.New("cairo: nil renderer")
	}
	out, err := procutil.Run(ctx, r.Binary, "-v")
	text := strings.TrimSpace(out)
	if text == "" && err != nil {
		return "", err
	}
	return procutil.FirstLine(text), nil
}

// RenderSVG renders a 0-based page to SVG bytes through pdftocairo. The SVG is
// produced by a separate process and is not loaded into the Core.
func (r *Renderer) RenderSVG(ctx context.Context, path string, page int) ([]byte, error) {
	if r == nil {
		return nil, errors.New("cairo: nil renderer")
	}
	if path == "" || page < 0 {
		return nil, errors.New("cairo: invalid page request")
	}
	dir, err := os.MkdirTemp("", "go-pdf-cairo-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	prefix := filepath.Join(dir, "page")
	args := []string{"-svg", "-f", fmt.Sprint(page + 1), "-l", fmt.Sprint(page + 1)}
	if r.Password != "" {
		args = append(args, "-upw", r.Password)
	}
	args = append(args, path, prefix)
	out, err := procutil.Run(ctx, r.Binary, args...)
	if err != nil {
		return nil, fmt.Errorf("cairo: render page %d: %w: %s", page, err, strings.TrimSpace(out))
	}
	limit := r.MaxOutputBytes
	if limit <= 0 {
		limit = 64 << 20
	}
	svg, err := procutil.ReadFile(prefix, limit)
	if err != nil {
		return nil, fmt.Errorf("cairo: SVG output: %w", err)
	}
	if len(svg) == 0 || !strings.Contains(string(svg[:min(len(svg), 256)]), "<svg") {
		return nil, errors.New("cairo: output is not SVG")
	}
	return svg, nil
}

// ProviderInfo reports the external pdftocairo binary metadata.
func (r *Renderer) ProviderInfo(ctx context.Context) extension.ProviderInfo {
	version, _ := r.Version(ctx)
	if version == "" {
		version = "unknown"
	}
	return extension.ProviderInfo{
		Name: "poppler-cairo", Version: version, License: "GPL-2.0-or-later",
		Determinism: extension.PlatformDependent,
	}
}

// Features reports the optional SVG path.
func (r *Renderer) Features() []extension.Feature {
	return []extension.Feature{{
		Name: "svg", Status: extension.StatusExternal,
		Reason: "rendered by the pdftocairo process",
	}}
}
