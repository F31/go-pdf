package cairo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Renderer invokes pdftocairo. It is safe for concurrent use when its exported
// fields are not mutated after construction.
type Renderer struct {
	Binary   string
	Password string
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
	cmd := exec.CommandContext(ctx, r.Binary, "-v")
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if text == "" && err != nil {
		return "", err
	}
	if i := strings.IndexByte(text, '\n'); i >= 0 {
		text = text[:i]
	}
	return text, nil
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
	cmd := exec.CommandContext(ctx, r.Binary, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("cairo: render page %d: %w: %s", page, err, strings.TrimSpace(string(out)))
	}
	svg, err := os.ReadFile(prefix)
	if err != nil {
		return nil, err
	}
	if len(svg) == 0 || !strings.Contains(string(svg[:min(len(svg), 256)]), "<svg") {
		return nil, errors.New("cairo: output is not SVG")
	}
	return svg, nil
}
