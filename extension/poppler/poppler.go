package poppler

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Renderer invokes pdftoppm. It is safe for concurrent use when its exported
// fields are not mutated after construction.
type Renderer struct {
	Binary   string
	DPI      int
	Password string
}

// New returns a renderer using binary (or pdftoppm when empty).
func New(binary string) (*Renderer, error) {
	if binary == "" {
		binary = "pdftoppm"
	}
	if _, err := exec.LookPath(binary); err != nil {
		return nil, fmt.Errorf("poppler: executable %q: %w", binary, err)
	}
	return &Renderer{Binary: binary, DPI: 120}, nil
}

// Version returns the first line of pdftoppm -v output.
func (r *Renderer) Version(ctx context.Context) (string, error) {
	if r == nil {
		return "", errors.New("poppler: nil renderer")
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

// RenderPage renders a 0-based page through pdftoppm. The returned image is
// decoded from a temporary PNG; no Poppler code is loaded into this process.
func (r *Renderer) RenderPage(ctx context.Context, path string, page int) (image.Image, error) {
	if r == nil {
		return nil, errors.New("poppler: nil renderer")
	}
	if path == "" || page < 0 {
		return nil, errors.New("poppler: invalid page request")
	}
	dpi, err := r.dpi()
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "go-pdf-poppler-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	prefix := filepath.Join(dir, "page")
	args := []string{"-f", strconv.Itoa(page + 1), "-l", strconv.Itoa(page + 1), "-singlefile", "-png", "-r", strconv.Itoa(dpi)}
	if r.Password != "" {
		args = append(args, "-upw", r.Password)
	}
	args = append(args, path, prefix)
	cmd := exec.CommandContext(ctx, r.Binary, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("poppler: render page %d: %w: %s", page, err, strings.TrimSpace(string(out)))
	}
	img, err := readPNG(prefix + ".png")
	if err != nil {
		return nil, fmt.Errorf("poppler: decode PNG: %w", err)
	}
	return img, nil
}

// RenderEach renders pages first..last (inclusive, zero-based) with one
// pdftoppm process and invokes fn in page order. Memory use is one decoded page
// at a time. The callback must not retain the image after returning.
func (r *Renderer) RenderEach(ctx context.Context, path string, first, last int, fn func(page int, img image.Image) error) error {
	if r == nil {
		return errors.New("poppler: nil renderer")
	}
	if path == "" || first < 0 || last < first {
		return errors.New("poppler: invalid page range")
	}
	if fn == nil {
		return errors.New("poppler: nil callback")
	}
	dpi, err := r.dpi()
	if err != nil {
		return err
	}
	dir, err := os.MkdirTemp("", "go-pdf-poppler-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	prefix := filepath.Join(dir, "page")
	args := []string{"-f", strconv.Itoa(first + 1), "-l", strconv.Itoa(last + 1), "-png", "-r", strconv.Itoa(dpi)}
	if r.Password != "" {
		args = append(args, "-upw", r.Password)
	}
	args = append(args, path, prefix)
	cmd := exec.CommandContext(ctx, r.Binary, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("poppler: render pages %d-%d: %w: %s", first, last, err, strings.TrimSpace(string(out)))
	}
	files, err := filepath.Glob(prefix + "-*.png")
	if err != nil {
		return err
	}
	byPage := make(map[int]string, len(files))
	for _, name := range files {
		base := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(name), filepath.Base(prefix)+"-"), ".png")
		n, err := strconv.Atoi(base)
		if err != nil {
			continue
		}
		byPage[n] = name
	}
	for page := first; page <= last; page++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		name := byPage[page+1]
		if name == "" {
			return fmt.Errorf("poppler: page %d not produced", page)
		}
		img, err := readPNG(name)
		if err != nil {
			return fmt.Errorf("poppler: page %d: %w", page, err)
		}
		if err := fn(page, img); err != nil {
			return err
		}
	}
	return nil
}

// RenderPages renders an arbitrary page set. Pages are de-duplicated and
// processed in ascending order; contiguous runs share one pdftoppm process.
// Callback images are valid only until the callback returns.
func (r *Renderer) RenderPages(ctx context.Context, path string, pages []int, fn func(page int, img image.Image) error) error {
	if r == nil {
		return errors.New("poppler: nil renderer")
	}
	if fn == nil {
		return errors.New("poppler: nil callback")
	}
	seen := make(map[int]bool, len(pages))
	ordered := make([]int, 0, len(pages))
	for _, page := range pages {
		if page < 0 {
			return errors.New("poppler: invalid page")
		}
		if !seen[page] {
			seen[page] = true
			ordered = append(ordered, page)
		}
	}
	sort.Ints(ordered)
	for i := 0; i < len(ordered); {
		j := i + 1
		for j < len(ordered) && ordered[j] == ordered[j-1]+1 {
			j++
		}
		if err := r.RenderEach(ctx, path, ordered[i], ordered[j-1], fn); err != nil {
			return err
		}
		i = j
	}
	return nil
}

func (r *Renderer) dpi() (int, error) {
	dpi := r.DPI
	if dpi <= 0 {
		dpi = 120
	}
	if dpi > 1200 {
		return 0, errors.New("poppler: DPI too large")
	}
	return dpi, nil
}

func readPNG(name string) (image.Image, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decode PNG: %w", err)
	}
	return img, nil
}
