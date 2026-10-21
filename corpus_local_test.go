package pdf

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/F31/go-pdf/displaylist"
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
	var totalPages, textRuns, glyphs, glyphsOK, paths, dlErrors int
	var fonts, fontsMetrics, fontsAdvances int
	fontSeen := map[*displaylist.FontInfo]bool{}
	extract := os.Getenv("CORPUS_DL") == "1"
	start := time.Now()
	for _, path := range files {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		doc, err := OpenFile(ctx, path)
		if err != nil {
			cancel()
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
			if extract {
				dl, err := p.DisplayList(ctx)
				if err != nil {
					dlErrors++
					t.Logf("DISPLAY %s #%d: %v", filepath.Base(path), i, err)
					continue
				}
				for _, it := range dl.Items {
					switch v := it.(type) {
					case *displaylist.TextRun:
						textRuns++
						if v.Font != nil && !fontSeen[v.Font] {
							fontSeen[v.Font] = true
							fonts++
							if v.Font.Metrics.UnitsPerEm > 0 {
								fontsMetrics++
							}
							if len(v.Font.Metrics.Advances) > 0 {
								fontsAdvances++
							}
						}
						for _, g := range v.Glyphs {
							glyphs++
							if g.UnicodeOK {
								glyphsOK++
							}
						}
					case *displaylist.Path:
						paths++
					}
				}
			}
		}
		cancel()
		_ = doc.Close()
	}
	t.Logf("corpus: files=%d ok=%d encrypted=%d corrupt=%d unsupported=%d budget=%d other=%d pages=%d textruns=%d glyphs=%d glyphsok=%d paths=%d fonts=%d fontsmetrics=%d fontsadvances=%d dlerrors=%d elapsed=%s",
		len(files), ok, encrypted, corrupt, unsupported, budget, other, totalPages, textRuns, glyphs, glyphsOK, paths, fonts, fontsMetrics, fontsAdvances, dlErrors, time.Since(start).Round(time.Millisecond))
}
