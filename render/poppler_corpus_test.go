package render

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/F31/go-pdf"
	"github.com/F31/go-pdf/internal/quality"
)

func envInt(name string, def int) int {
	if v := os.Getenv(name); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envFloat(name string, def float64) float64 {
	if v := os.Getenv(name); v != "" {
		if n, err := strconv.ParseFloat(v, 64); err == nil {
			return n
		}
	}
	return def
}

func corpusFileKey(file string) string {
	abs, err := filepath.Abs(file)
	if err != nil {
		abs = file
	}
	st, err := os.Stat(file)
	if err != nil {
		return fmt.Sprintf("%x", sha256.Sum256([]byte(abs)))[:16]
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%d|%d", abs, st.Size(), st.ModTime().UnixNano())))
	return fmt.Sprintf("%x", sum[:8])
}

func cacheRefPath(dir string, dpi, page int) string {
	return filepath.Join(dir, strconv.Itoa(dpi), fmt.Sprintf("%06d.png", page))
}

func loadCachedRef(dir string, dpi, page int) image.Image {
	f, err := os.Open(cacheRefPath(dir, dpi, page))
	if err != nil {
		return nil
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil
	}
	return img
}

func saveCachedRef(dir string, dpi, page int, img image.Image) {
	path := cacheRefPath(dir, dpi, page)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".ref-*.png")
	if err != nil {
		return
	}
	if err := png.Encode(tmp, img); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return
	}
	_ = os.Rename(tmp.Name(), path)
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 300 {
		s = s[:300]
	}
	return s
}

func pct(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	idx := int(float64(len(values)-1) * p)
	return values[idx]
}

type corpusReportRow struct {
	File     string
	Page     int
	Status   string
	PSNR     float64
	SSIM     float64
	MeanDE   float64
	P95DE    float64
	GoError  string
	RefError string
	Width    int
	Height   int
}

func popplerFormat() (flag, ext string) {
	switch strings.ToLower(os.Getenv("POPPLER_CORPUS_FORMAT")) {
	case "ppm", "pnm":
		return "", ".ppm"
	default:
		return "-png", ".png"
	}
}

func popplerSingle(bin, file string, page, dpi int, out string) (image.Image, error) {
	flag, ext := popplerFormat()
	args := []string{"-f", strconv.Itoa(page), "-l", strconv.Itoa(page), "-r", strconv.Itoa(dpi)}
	if flag != "" {
		args = append(args, flag)
	}
	args = append(args, "-singlefile", "-cropbox", "-hide-annotations", file, out)
	cmd := exec.Command(bin, args...)
	if b, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("pdftoppm page %d: %w: %s", page, err, b)
	}
	f, err := os.Open(out + ext)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return decodePopplerImage(f, ext)
}

func decodePopplerImage(r io.Reader, ext string) (image.Image, error) {
	if ext == ".png" {
		return png.Decode(r)
	}
	return decodePPM(r)
}

func ppmToken(r *bufio.Reader) (string, error) {
	for {
		b, err := r.ReadByte()
		if err != nil {
			return "", err
		}
		if b == '#' {
			for b != '\n' {
				b, err = r.ReadByte()
				if err != nil {
					return "", err
				}
			}
			continue
		}
		if b <= ' ' {
			continue
		}
		var sb strings.Builder
		sb.WriteByte(b)
		for {
			b, err = r.ReadByte()
			if err != nil {
				if err == io.EOF {
					return sb.String(), nil
				}
				return "", err
			}
			if b <= ' ' {
				return sb.String(), nil
			}
			sb.WriteByte(b)
		}
	}
}

func decodePPM(r io.Reader) (image.Image, error) {
	br := bufio.NewReader(r)
	magic, err := ppmToken(br)
	if err != nil {
		return nil, err
	}
	if magic != "P6" && magic != "P5" {
		return nil, fmt.Errorf("unsupported PPM magic %q", magic)
	}
	ws, err := ppmToken(br)
	if err != nil {
		return nil, err
	}
	hs, err := ppmToken(br)
	if err != nil {
		return nil, err
	}
	ms, err := ppmToken(br)
	if err != nil {
		return nil, err
	}
	w, err := strconv.Atoi(ws)
	if err != nil {
		return nil, err
	}
	h, err := strconv.Atoi(hs)
	if err != nil {
		return nil, err
	}
	maxv, err := strconv.Atoi(ms)
	if err != nil || maxv <= 0 || maxv > 255 {
		return nil, fmt.Errorf("unsupported PPM maxval %q", ms)
	}
	channels := 3
	if magic == "P5" {
		channels = 1
	}
	data := make([]byte, w*h*channels)
	if _, err := io.ReadFull(br, data); err != nil {
		return nil, err
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < w*h; i++ {
		var c color.RGBA
		if channels == 1 {
			v := uint8(int(data[i]) * 255 / maxv)
			c = color.RGBA{R: v, G: v, B: v, A: 255}
		} else {
			c = color.RGBA{
				R: uint8(int(data[3*i]) * 255 / maxv),
				G: uint8(int(data[3*i+1]) * 255 / maxv),
				B: uint8(int(data[3*i+2]) * 255 / maxv),
				A: 255,
			}
		}
		img.SetRGBA(i%w, i/w, c)
	}
	return img, nil
}

func TestDecodePPM(t *testing.T) {
	img, err := decodePPM(strings.NewReader("P6\n# test\n2 1\n255\n\xff\x00\x00\x00\xff\x00"))
	if err != nil {
		t.Fatal(err)
	}
	if got := img.At(0, 0); got != (color.RGBA{R: 255, A: 255}) {
		t.Fatalf("pixel 0 = %v", got)
	}
	if got := img.At(1, 0); got != (color.RGBA{G: 255, A: 255}) {
		t.Fatalf("pixel 1 = %v", got)
	}
	gray, err := decodePPM(strings.NewReader("P5 2 1 255\n\x00\xff"))
	if err != nil {
		t.Fatal(err)
	}
	if got := gray.At(0, 0); got != (color.RGBA{A: 255}) {
		t.Fatalf("gray 0 = %v", got)
	}
	if got := gray.At(1, 0); got != (color.RGBA{R: 255, G: 255, B: 255, A: 255}) {
		t.Fatalf("gray 1 = %v", got)
	}
}

// TestPopplerCorpusQuality compares the private corpus against pdftoppm. It is
// opt-in because the corpus and Poppler are not CI dependencies.
//
// POPPLER_CORPUS_PAGES=0 renders every page. POPPLER_CORPUS_REPORT writes a
// per-page CSV report; POPPLER_CORPUS_FILE filters by basename substring;
// POPPLER_CORPUS_PATH selects one file; POPPLER_CORPUS_PAGE_START/PAGE_END
// select a page range; POPPLER_CORPUS_FILES limits the number of files;
// POPPLER_CORPUS_CHUNK controls the Poppler batch size;
// POPPLER_CORPUS_FORMAT=ppm avoids PNG encode/decode;
// POPPLER_CORPUS_REF_CACHE caches Poppler references between runs; and
// POPPLER_CORPUS_FONT_DIR pins font directories for reproducible output.
// By default the harness uses render.BuiltinFonts(); set
// POPPLER_CORPUS_SYSTEM_FONTS=1 to opt into host fonts.
func TestPopplerCorpusQuality(t *testing.T) {
	if os.Getenv("POPPLER_CORPUS") == "" {
		t.Skip("set POPPLER_CORPUS=1 to compare the local corpus")
	}
	bin, err := exec.LookPath("pdftoppm")
	if err != nil {
		t.Skip("pdftoppm not installed")
	}
	var files []string
	if path := os.Getenv("POPPLER_CORPUS_PATH"); path != "" {
		files = []string{path}
	} else {
		files, err = filepath.Glob("../docs/pdf/*.pdf")
		if err != nil {
			t.Fatal(err)
		}
		sort.Strings(files)
	}
	if match := os.Getenv("POPPLER_CORPUS_FILE"); match != "" {
		var selected []string
		for _, file := range files {
			if strings.Contains(filepath.Base(file), match) {
				selected = append(selected, file)
			}
		}
		files = selected
	}
	if limit := envInt("POPPLER_CORPUS_FILES", 0); limit > 0 && len(files) > limit {
		files = files[:limit]
	}
	shards := envInt("POPPLER_CORPUS_SHARDS", 1)
	shard := envInt("POPPLER_CORPUS_SHARD", 0)
	if shards > 1 {
		var selected []string
		for i, file := range files {
			if i%shards == shard {
				selected = append(selected, file)
			}
		}
		files = selected
	}
	if len(files) == 0 {
		t.Skip("docs/pdf corpus not present")
	}
	maxPages := envInt("POPPLER_CORPUS_PAGES", 1)
	pageStart := envInt("POPPLER_CORPUS_PAGE_START", 1)
	if pageStart < 1 {
		pageStart = 1
	}
	pageEnd := envInt("POPPLER_CORPUS_PAGE_END", 0)
	dpi := envInt("POPPLER_CORPUS_DPI", 72)
	minPSNR := envFloat("POPPLER_CORPUS_MIN_PSNR", 20)
	minSSIM := envFloat("POPPLER_CORPUS_MIN_SSIM", 0.85)
	chunk := envInt("POPPLER_CORPUS_CHUNK", 8)
	if chunk < 1 {
		chunk = 1
	}
	if chunk > 64 {
		chunk = 64
	}
	gate := os.Getenv("POPPLER_CORPUS_GATE") == "1"
	maxFallback := envFloat("POPPLER_CORPUS_MAX_FALLBACK", 0.2)

	var report *csv.Writer
	var reportFile *os.File
	if path := os.Getenv("POPPLER_CORPUS_REPORT"); path != "" {
		if dir := filepath.Dir(path); dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		reportFile, err = os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		report = csv.NewWriter(reportFile)
		if err := report.Write([]string{"file", "page", "status", "psnr", "ssim", "mean_delta_e", "p95_delta_e", "go_error", "ref_error", "width", "height"}); err != nil {
			t.Fatal(err)
		}
		defer func() {
			report.Flush()
			if err := report.Error(); err != nil {
				t.Errorf("corpus report: %v", err)
			}
			_ = reportFile.Close()
		}()
		t.Logf("corpus report: %s", path)
	}
	writeRow := func(row corpusReportRow) {
		if report == nil {
			return
		}
		err := report.Write([]string{
			row.File, strconv.Itoa(row.Page), row.Status,
			fmt.Sprintf("%.3f", row.PSNR), fmt.Sprintf("%.6f", row.SSIM),
			fmt.Sprintf("%.3f", row.MeanDE), fmt.Sprintf("%.3f", row.P95DE),
			oneLine(row.GoError), oneLine(row.RefError), strconv.Itoa(row.Width), strconv.Itoa(row.Height),
		})
		if err != nil {
			t.Fatalf("corpus report write: %v", err)
		}
	}

	dir := t.TempDir()
	cacheRoot := os.Getenv("POPPLER_CORPUS_REF_CACHE")
	corpusFonts := FontProvider(BuiltinFonts())
	if dir := os.Getenv("POPPLER_CORPUS_FONT_DIR"); dir != "" {
		dirs := filepath.SplitList(dir)
		sys, err := SystemFonts(dirs...)
		if err != nil {
			t.Fatalf("corpus font dir %q: %v", dir, err)
		}
		corpusFonts = ChainFonts(sys, BuiltinFonts())
	} else if os.Getenv("POPPLER_CORPUS_SYSTEM_FONTS") == "1" {
		if sys, err := SystemFonts(); err == nil {
			corpusFonts = ChainFonts(sys, BuiltinFonts())
		}
	}
	var psnrs, ssims []float64
	var total, docErrors, renderErrors, refErrors, dimErrors, fallback int
	for fileIndex, file := range files {
		fileCache := ""
		if cacheRoot != "" {
			fileCache = filepath.Join(cacheRoot, corpusFileKey(file))
		}
		d, err := pdf.OpenFile(context.Background(), file, pdf.WithRepair(true))
		if err != nil {
			docErrors++
			total++
			writeRow(corpusReportRow{File: filepath.Base(file), Status: "doc_error", GoError: err.Error()})
			continue
		}
		n := d.NumPages()
		if pageEnd > 0 && pageEnd < n {
			n = pageEnd
		}
		if maxPages > 0 && n > maxPages {
			n = maxPages
		}
		first := pageStart - 1
		if first > n {
			first = n
		}
		type refChunk struct {
			start, end int
			ext        string
			outputs    []string
			batchOut   []byte
			batchOK    bool
		}
		prepare := func(start, end int) *refChunk {
			rc := &refChunk{start: start, end: end}
			prefix := filepath.Join(dir, fmt.Sprintf("f%03d_%04d", fileIndex, start+1))
			flag, ext := popplerFormat()
			rc.ext = ext
			allCached := fileCache != ""
			if allCached {
				for i := start; i < end; i++ {
					if _, err := os.Stat(cacheRefPath(fileCache, dpi, i+1)); err != nil {
						allCached = false
						break
					}
				}
			}
			if allCached {
				return rc
			}
			args := []string{"-f", strconv.Itoa(start + 1), "-l", strconv.Itoa(end), "-r", strconv.Itoa(dpi)}
			if flag != "" {
				args = append(args, flag)
			}
			args = append(args, "-cropbox", "-hide-annotations", file, prefix)
			cmd := exec.Command(bin, args...)
			var batchErr error
			rc.batchOut, batchErr = cmd.CombinedOutput()
			rc.outputs, _ = filepath.Glob(prefix + "-*" + ext)
			sort.Strings(rc.outputs)
			rc.batchOK = batchErr == nil && len(rc.outputs) == end-start
			return rc
		}
		chunks := make(chan *refChunk, 1)
		go func() {
			defer close(chunks)
			for start := first; start < n; start += chunk {
				end := start + chunk
				if end > n {
					end = n
				}
				chunks <- prepare(start, end)
			}
		}()
		for rc := range chunks {
			for i := rc.start; i < rc.end; i++ {
				total++
				pageNo := i + 1
				row := corpusReportRow{File: filepath.Base(file), Page: pageNo}
				p, err := d.Page(i)
				if err != nil {
					renderErrors++
					fallback++
					row.Status, row.GoError = "render_error", err.Error()
					writeRow(row)
					continue
				}
				got, err := RenderPage(context.Background(), p, PageOptions{
					DPI: dpi, Background: "white", IncludeText: true,
					Fonts: corpusFonts, MaxPixels: 64 << 20,
				})
				if err != nil {
					renderErrors++
					fallback++
					row.Status, row.GoError = "render_error", err.Error()
					writeRow(row)
					t.Logf("render failed for %s page %d: %v", filepath.Base(file), pageNo, err)
					continue
				}
				var ref image.Image
				cacheHit := false
				if fileCache != "" {
					ref = loadCachedRef(fileCache, dpi, pageNo)
					cacheHit = ref != nil
				}
				if ref == nil && rc.batchOK && i-rc.start < len(rc.outputs) {
					f, err := os.Open(rc.outputs[i-rc.start])
					if err == nil {
						ref, err = decodePopplerImage(f, rc.ext)
						_ = f.Close()
					}
					if err != nil {
						ref = nil
					}
				}
				if ref == nil {
					single := filepath.Join(dir, fmt.Sprintf("single_%03d_%04d", fileIndex, pageNo))
					ref, err = popplerSingle(bin, file, pageNo, dpi, single)
					if err == nil {
						_ = os.Remove(single + rc.ext)
					}
				}
				if ref == nil {
					refErrors++
					fallback++
					row.Status, row.RefError = "ref_error", "pdftoppm failed"
					if len(rc.batchOut) > 0 && !rc.batchOK {
						row.RefError = oneLine(string(rc.batchOut))
					}
					writeRow(row)
					t.Logf("pdftoppm failed for %s page %d", filepath.Base(file), pageNo)
					continue
				}
				if fileCache != "" && !cacheHit {
					saveCachedRef(fileCache, dpi, pageNo, ref)
				}
				qr, err := quality.Compare(got, ref)
				if err != nil {
					dimErrors++
					fallback++
					row.Status, row.RefError = "dimension_error", err.Error()
					row.Width, row.Height = got.Bounds().Dx(), got.Bounds().Dy()
					writeRow(row)
					t.Logf("compare failed for %s page %d: %v (go %v poppler %v)", filepath.Base(file), pageNo, err, got.Bounds(), ref.Bounds())
					continue
				}
				psnrs = append(psnrs, qr.PSNR)
				ssims = append(ssims, qr.SSIM)
				row.Width, row.Height = qr.Width, qr.Height
				row.PSNR, row.SSIM, row.MeanDE, row.P95DE = qr.PSNR, qr.SSIM, qr.MeanDeltaE, qr.P95DeltaE
				if qr.PSNR < minPSNR || qr.SSIM < minSSIM {
					fallback++
					row.Status = "fallback"
				} else {
					row.Status = "ok"
				}
				writeRow(row)
				if total%100 == 0 {
					t.Logf("corpus progress: files=%d pages=%d compared=%d fallback=%d", fileIndex+1, total, len(psnrs), fallback)
				}
			}
			for _, out := range rc.outputs {
				_ = os.Remove(out)
			}
		}
		_ = d.Close()
	}
	sort.Float64s(psnrs)
	sort.Float64s(ssims)
	rate := 0.0
	if total > 0 {
		rate = float64(fallback) / float64(total)
	}
	t.Logf("poppler corpus: files=%d pages=%d compared=%d docErrors=%d renderErrors=%d refErrors=%d dimErrors=%d fallback=%d rate=%.3f",
		len(files), total, len(psnrs), docErrors, renderErrors, refErrors, dimErrors, fallback, rate)
	if len(psnrs) > 0 {
		t.Logf("poppler corpus: PSNR p10=%.2f p50=%.2f p90=%.2f; SSIM p10=%.3f p50=%.3f p90=%.3f",
			pct(psnrs, 0.10), pct(psnrs, 0.50), pct(psnrs, 0.90),
			pct(ssims, 0.10), pct(ssims, 0.50), pct(ssims, 0.90))
	}
	if gate && rate > maxFallback {
		t.Fatalf("poppler corpus fallback rate %.3f > %.3f", rate, maxFallback)
	}
}
