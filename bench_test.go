package pdf

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/F31/go-pdf/internal/testkit"
)

func benchmarkPDF(pages int) []byte {
	kids := make([]string, 0, pages)
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(3, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"),
	}
	for i := 0; i < pages; i++ {
		pageID := 4 + 2*i
		contentID := pageID + 1
		kids = append(kids, fmt.Sprintf("%d 0 R", pageID))
		objs = append(objs,
			testkit.Raw(pageID, fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /Resources << /Font << /F1 3 0 R >> >> /Contents %d 0 R >>", contentID)),
			testkit.Stream(contentID, "", []byte(fmt.Sprintf("BT /F1 10 Tf 10 50 Td (Page %d server document core benchmark) Tj ET 0 0 1 rg 10 10 100 20 re f", i+1))),
		)
	}
	objs = append(objs, testkit.Raw(2, fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", joinStrings(kids), pages)))
	return testkit.Build(objs, "/Root 1 0 R")
}

func joinStrings(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += " "
		}
		out += p
	}
	return out
}

func BenchmarkOpenReaderAt(b *testing.B) {
	data := benchmarkPDF(50)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d, err := Open(ctx, bytes.NewReader(data), int64(len(data)))
		if err != nil {
			b.Fatal(err)
		}
		_ = d.Close()
	}
}

func BenchmarkExtractText(b *testing.B) {
	data := benchmarkPDF(50)
	for _, workers := range []int{1, 4} {
		b.Run(fmt.Sprintf("workers=%d", workers), func(b *testing.B) {
			d, err := Open(context.Background(), bytes.NewReader(data), int64(len(data)))
			if err != nil {
				b.Fatal(err)
			}
			defer d.Close()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				res, err := d.ExtractText(context.Background(), ExtractOptions{Workers: workers})
				if err != nil || res.Failed != 0 {
					b.Fatalf("extract: %v failed=%d", err, res.Failed)
				}
			}
		})
	}
}

func BenchmarkExtractTextEach(b *testing.B) {
	data := benchmarkPDF(50)
	d, err := Open(context.Background(), bytes.NewReader(data), int64(len(data)))
	if err != nil {
		b.Fatal(err)
	}
	defer d.Close()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := d.ExtractTextEach(context.Background(), ExtractOptions{Workers: 4}, func(PageText) error { return nil }); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPreflight(b *testing.B) {
	data := benchmarkPDF(50)
	for _, workers := range []int{1, 4} {
		b.Run(fmt.Sprintf("workers=%d", workers), func(b *testing.B) {
			d, err := Open(context.Background(), bytes.NewReader(data), int64(len(data)))
			if err != nil {
				b.Fatal(err)
			}
			defer d.Close()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := d.PreflightWithOptions(context.Background(), PreflightOptions{Workers: workers}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// TestServerCoreSLO is opt-in because it reports timing rather than asserting a
// portable threshold. Run with PDF_SLO=1 and record host/Go version.
func TestServerCoreSLO(t *testing.T) {
	if os.Getenv("PDF_SLO") != "1" {
		t.Skip("set PDF_SLO=1 to run the server core timing sample")
	}
	data := benchmarkPDF(50)
	ctx := context.Background()
	const samples = 40
	openTimes := make([]time.Duration, 0, samples)
	for i := 0; i < samples; i++ {
		start := time.Now()
		d, err := Open(ctx, bytes.NewReader(data), int64(len(data)))
		if err != nil {
			t.Fatal(err)
		}
		openTimes = append(openTimes, time.Since(start))
		_ = d.Close()
	}
	d, err := Open(ctx, bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	extractTimes := make([]time.Duration, 0, samples)
	for i := 0; i < samples; i++ {
		start := time.Now()
		res, err := d.ExtractText(ctx, ExtractOptions{Workers: 4})
		if err != nil || res.Failed != 0 {
			t.Fatalf("extract: %v", err)
		}
		extractTimes = append(extractTimes, time.Since(start))
	}
	preflightTimes := make([]time.Duration, 0, samples)
	for i := 0; i < samples; i++ {
		start := time.Now()
		if _, err := d.PreflightWithOptions(ctx, PreflightOptions{Workers: 4}); err != nil {
			t.Fatal(err)
		}
		preflightTimes = append(preflightTimes, time.Since(start))
	}
	t.Logf("open50 p50=%s p95=%s p99=%s", percentile(openTimes, 0.50), percentile(openTimes, 0.95), percentile(openTimes, 0.99))
	t.Logf("extract50 workers=4 p50=%s p95=%s p99=%s", percentile(extractTimes, 0.50), percentile(extractTimes, 0.95), percentile(extractTimes, 0.99))
	t.Logf("preflight50 workers=4 p50=%s p95=%s p99=%s", percentile(preflightTimes, 0.50), percentile(preflightTimes, 0.95), percentile(preflightTimes, 0.99))
}

func percentile(values []time.Duration, p float64) time.Duration {
	if len(values) == 0 {
		return 0
	}
	cp := append([]time.Duration(nil), values...)
	sort.Slice(cp, func(i, j int) bool { return cp[i] < cp[j] })
	idx := int(float64(len(cp)-1) * p)
	return cp[idx]
}
