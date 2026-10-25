package pdf

import (
	"context"
	"encoding/xml"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"unicode"

	pdftext "github.com/F31/go-pdf/text"
)

type popplerWord struct {
	XMin float64 `xml:"xMin,attr"`
	YMin float64 `xml:"yMin,attr"`
	XMax float64 `xml:"xMax,attr"`
	YMax float64 `xml:"yMax,attr"`
	Text string  `xml:",chardata"`
}

type popplerPage struct {
	Width  float64       `xml:"width,attr"`
	Height float64       `xml:"height,attr"`
	Words  []popplerWord `xml:"word"`
}

func TestEnglishCorpusTextPosition(t *testing.T) {
	if os.Getenv("ENGLISH_CORPUS") != "1" {
		t.Skip("set ENGLISH_CORPUS=1 and ENGLISH_CORPUS_DIR to run")
	}
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("pdftotext not installed")
	}
	dir := os.Getenv("ENGLISH_CORPUS_DIR")
	if dir == "" {
		t.Fatal("ENGLISH_CORPUS_DIR is required")
	}
	limit := 3
	if s := os.Getenv("ENGLISH_CORPUS_PAGES"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n <= 0 {
			t.Fatalf("invalid ENGLISH_CORPUS_PAGES %q", s)
		}
		limit = n
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatalf("no PDFs in %s", dir)
	}
	var edge, center []float64
	pages, matched, compared := 0, 0, 0
	for _, path := range files {
		d, err := OpenFile(context.Background(), path)
		if err != nil {
			t.Logf("skip %s: %v", filepath.Base(path), err)
			continue
		}
		for page := 0; page < d.NumPages() && page < limit; page++ {
			p, err := d.Page(page)
			if err != nil {
				continue
			}
			dl, err := p.DisplayList(context.Background())
			if err != nil {
				continue
			}
			core := pdftext.Extract(dl, pdftext.Options{})
			ref, err := popplerWords(context.Background(), path, page)
			if err != nil {
				t.Logf("skip %s p%d: %v", filepath.Base(path), page+1, err)
				continue
			}
			pages++
			coreWords := contentWords(core.Words)
			refWords := make(map[string][]int)
			for i, w := range ref.Words {
				if key := normalizeWord(w.Text); key != "" {
					refWords[key] = append(refWords[key], i)
				}
			}
			for _, cw := range coreWords {
				key := normalizeWord(cw.Text)
				best, bestDist := -1, math.Inf(1)
				for _, idx := range refWords[key] {
					rw := ref.Words[idx]
					dx := (cw.Bounds.X0 + cw.Bounds.X1 - rw.XMin - rw.XMax) / 2
					dy := (cw.Bounds.Y0 + cw.Bounds.Y1 - rw.YMin - rw.YMax) / 2
					d := math.Hypot(dx, dy)
					if d < bestDist {
						best, bestDist = idx, d
					}
				}
				if best < 0 {
					continue
				}
				rw := ref.Words[best]
				e := math.Max(math.Abs(cw.Bounds.X0-rw.XMin), math.Abs(cw.Bounds.Y0-rw.YMin))
				e = math.Max(e, math.Abs(cw.Bounds.X1-rw.XMax))
				e = math.Max(e, math.Abs(cw.Bounds.Y1-rw.YMax))
				dx := (cw.Bounds.X0 + cw.Bounds.X1 - rw.XMin - rw.XMax) / 2
				dy := (cw.Bounds.Y0 + cw.Bounds.Y1 - rw.YMin - rw.YMax) / 2
				edge = append(edge, e)
				center = append(center, math.Hypot(dx, dy))
				matched++
			}
			compared++
		}
		_ = d.Close()
	}
	if matched < 20 {
		t.Fatalf("too few matched words: %d pages=%d", matched, pages)
	}
	sort.Float64s(edge)
	sort.Float64s(center)
	p := func(v []float64, q float64) float64 {
		if len(v) == 0 {
			return 0
		}
		return v[int(float64(len(v)-1)*q)]
	}
	within := 0
	for _, e := range edge {
		if e <= 0.3 {
			within++
		}
	}
	t.Logf("english text position: files=%d pages=%d matched_words=%d", len(files), compared, matched)
	t.Logf("word bbox edge error: p50=%.3fpt p95=%.3fpt p99=%.3fpt within_0.3=%.2f%%", p(edge, .5), p(edge, .95), p(edge, .99), 100*float64(within)/float64(len(edge)))
	t.Logf("word center error: p50=%.3fpt p95=%.3fpt p99=%.3fpt", p(center, .5), p(center, .95), p(center, .99))
	if got := p(edge, .95); got > 0.3 {
		t.Fatalf("word bbox P95 = %.3fpt, over 0.3pt", got)
	}
}

func popplerWords(ctx context.Context, path string, page int) (popplerPage, error) {
	cmd := exec.CommandContext(ctx, "pdftotext", "-f", strconv.Itoa(page+1), "-l", strconv.Itoa(page+1), "-bbox", path, "-")
	out, err := cmd.Output()
	if err != nil {
		return popplerPage{}, err
	}
	dec := xml.NewDecoder(strings.NewReader(string(out)))
	for {
		tok, err := dec.Token()
		if err != nil {
			return popplerPage{}, err
		}
		if start, ok := tok.(xml.StartElement); ok && start.Name.Local == "page" {
			var p popplerPage
			if err := dec.DecodeElement(&p, &start); err != nil {
				return popplerPage{}, err
			}
			return p, nil
		}
	}
}

func contentWords(words []pdftext.Word) []pdftext.Word {
	out := words[:0]
	for _, w := range words {
		if normalizeWord(w.Text) != "" {
			out = append(out, w)
		}
	}
	return out
}

func normalizeWord(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}
