package pdf

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestLocalCorpus exercises a private corpus when docs/pdf is present. The
// directory is git-ignored; CI skips this test. It reports a summary rather
// than failing on expected encrypted/corrupt inputs so that the corpus can be
// used to triage coverage.
func TestLocalCorpus(t *testing.T) {
	root := filepath.Join("docs", "pdf")
	if _, err := os.Stat(root); err != nil {
		t.Skip("local corpus not present")
	}
	var files []string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Ext(path), ".pdf") {
			files = append(files, path)
		}
		return nil
	})
	if len(files) == 0 {
		t.Skip("no PDFs in local corpus")
	}
	var ok, encrypted, corrupt, unsupported, budget, other int
	var totalPages int
	start := time.Now()
	for _, path := range files {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		doc, err := OpenFile(ctx, path)
		cancel()
		if err != nil {
			switch {
			case errors.Is(err, ErrPassword):
				encrypted++
			case errors.Is(err, ErrCorrupt):
				corrupt++
				var pe *Error
				if errors.As(err, &pe) {
					t.Logf("CORRUPT %s: code=%s obj=%d page=%d offset=%d err=%v", filepath.Base(path), pe.Code, pe.Obj, pe.Page, pe.Offset, pe.Err)
				} else {
					t.Logf("CORRUPT %s: %v", filepath.Base(path), err)
				}
			case errors.Is(err, ErrUnsupported):
				unsupported++
			case errors.Is(err, ErrBudget):
				budget++
			default:
				other++
				t.Logf("OTHER %s: %v", filepath.Base(path), err)
			}
			continue
		}
		ok++
		totalPages += doc.NumPages()
		for i := 0; i < doc.NumPages(); i++ {
			p, err := doc.Page(i)
			if err != nil {
				t.Logf("PAGE %s #%d: %v", filepath.Base(path), i, err)
				break
			}
			_, _ = p.Size()
			_ = p.Info()
		}
		_ = doc.Close()
	}
	t.Logf("corpus: files=%d ok=%d encrypted=%d corrupt=%d unsupported=%d budget=%d other=%d pages=%d elapsed=%s",
		len(files), ok, encrypted, corrupt, unsupported, budget, other, totalPages, time.Since(start).Round(time.Millisecond))
}
