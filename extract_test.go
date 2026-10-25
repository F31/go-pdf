package pdf

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/F31/go-pdf/internal/testkit"
)

func batchTextData() []byte {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R 5 0 R] /Count 2 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /Resources << /Font << /F1 6 0 R >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("BT /F1 12 Tf 10 50 Td (One) Tj ET")),
		testkit.Raw(5, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /Resources << /Font << /F1 6 0 R >> >> /Contents 7 0 R >>"),
		testkit.Raw(6, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"),
		testkit.Stream(7, "", []byte("BT /F1 12 Tf 10 50 Td (Two) Tj ET")),
	}
	return testkit.Build(objs, "/Root 1 0 R")
}

func batchTextDoc(t *testing.T) *Document {
	t.Helper()
	return openTest(t, batchTextData())
}

func TestExtractTextBatch(t *testing.T) {
	d := batchTextDoc(t)
	defer d.Close()
	res, err := d.ExtractText(context.Background(), ExtractOptions{Workers: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Pages) != 2 || res.Failed != 0 {
		t.Fatalf("result = %#v", res)
	}
	if !strings.Contains(res.Pages[0].Text.Text, "One") || !strings.Contains(res.Pages[1].Text.Text, "Two") {
		t.Fatalf("text = %q / %q", res.Pages[0].Text.Text, res.Pages[1].Text.Text)
	}
}

func brokenPageData() []byte {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R 5 0 R] /Count 2 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /Resources << /Font << /F1 6 0 R >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("BT /F1 12 Tf 10 50 Td (One) Tj ET")),
		testkit.Raw(5, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /Contents 8 0 R >>"),
		testkit.Raw(6, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"),
		testkit.Raw(8, "<< /Length 4 /Filter /FlateDecode >>\nstream\nxxxx\nendstream"),
	}
	return testkit.Build(objs, "/Root 1 0 R")
}

func TestExtractTextIsolatesPageErrors(t *testing.T) {
	d := openTest(t, brokenPageData())
	defer d.Close()
	res, err := d.ExtractText(context.Background(), ExtractOptions{Workers: 2})
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed != 1 || len(res.FailedPages) != 1 || res.FailedPages[0] != 1 || res.Pages[0].Err != nil || res.Pages[1].Err == nil {
		t.Fatalf("result = %#v", res)
	}
	if len(res.Failures) != 1 || res.Failures[0].Page != 1 || res.Failures[0].Code == "" {
		t.Fatalf("failures = %#v", res.Failures)
	}
	if !strings.Contains(res.Pages[0].Text.Text, "One") {
		t.Fatalf("page 0 text = %q", res.Pages[0].Text.Text)
	}
}

func TestExtractTextMatchesSinglePage(t *testing.T) {
	d := batchTextDoc(t)
	defer d.Close()
	res, err := d.ExtractText(context.Background(), ExtractOptions{Workers: 2})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < d.NumPages(); i++ {
		p, err := d.Page(i)
		if err != nil {
			t.Fatal(err)
		}
		single, err := p.Text(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if res.Pages[i].Err != nil || res.Pages[i].Text.Text != single.Text {
			t.Fatalf("page %d: batch=%q single=%q err=%v", i, res.Pages[i].Text.Text, single.Text, res.Pages[i].Err)
		}
	}
}

func TestExtractTextMatchesSinglePageFailures(t *testing.T) {
	d := openTest(t, brokenPageData())
	defer d.Close()
	res, err := d.ExtractText(context.Background(), ExtractOptions{Workers: 2})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < d.NumPages(); i++ {
		p, err := d.Page(i)
		if err != nil {
			t.Fatal(err)
		}
		single, singleErr := p.Text(context.Background())
		batch := res.Pages[i]
		if (singleErr == nil) != (batch.Err == nil) {
			t.Fatalf("page %d: batchErr=%v singleErr=%v", i, batch.Err, singleErr)
		}
		if singleErr == nil && single.Text != batch.Text.Text {
			t.Fatalf("page %d: batch=%q single=%q", i, batch.Text.Text, single.Text)
		}
	}
}

func TestPreflightFailurePagesMatchExtract(t *testing.T) {
	d := openTest(t, brokenPageData())
	defer d.Close()
	extract, err := d.ExtractText(context.Background(), ExtractOptions{Workers: 2})
	if err != nil {
		t.Fatal(err)
	}
	report, err := d.PreflightWithOptions(context.Background(), PreflightOptions{Workers: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(extract.FailedPages) != len(report.FailedPages) || len(report.FailedPages) != 1 || report.FailedPages[0] != extract.FailedPages[0] {
		t.Fatalf("extract=%v preflight=%v", extract.FailedPages, report.FailedPages)
	}
}

func TestExtractTextEachContextCancellation(t *testing.T) {
	d := batchTextDoc(t)
	defer d.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	summary, err := d.ExtractTextEach(ctx, ExtractOptions{Workers: 2}, func(PageText) error {
		cancel()
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want canceled", err)
	}
	if summary == nil || summary.Processed == 0 {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestExtractWorkersRespectBudget(t *testing.T) {
	b := DefaultBudget
	b.MaxConcurrency = 1
	d := openTest(t, batchTextData(), WithBudget(&b))
	defer d.Close()
	if got := d.extractWorkers(16, 16); got != 1 {
		t.Fatalf("workers = %d", got)
	}
}

func TestExtractTextSelectionOrder(t *testing.T) {
	d := batchTextDoc(t)
	defer d.Close()
	res, err := d.ExtractText(context.Background(), ExtractOptions{Pages: []int{1, 0}, Workers: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Pages) != 2 || res.Pages[0].Index != 1 || res.Pages[1].Index != 0 {
		t.Fatalf("order = %#v", res.Pages)
	}
	if !strings.Contains(res.Pages[0].Text.Text, "Two") || !strings.Contains(res.Pages[1].Text.Text, "One") {
		t.Fatalf("text = %q / %q", res.Pages[0].Text.Text, res.Pages[1].Text.Text)
	}
}

func TestExtractTextValidationAndCancel(t *testing.T) {
	d := batchTextDoc(t)
	defer d.Close()
	if _, err := d.ExtractText(context.Background(), ExtractOptions{Pages: []int{2}}); err == nil {
		t.Fatal("expected page range error")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := d.ExtractText(ctx, ExtractOptions{}); err == nil {
		t.Fatal("expected cancellation error")
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := d.ExtractText(context.Background(), ExtractOptions{}); err == nil {
		t.Fatal("expected closed error")
	}
}

func TestExtractTextEachIsolatesPageErrors(t *testing.T) {
	d := openTest(t, brokenPageData())
	defer d.Close()
	rows := map[int]PageText{}
	summary, err := d.ExtractTextEach(context.Background(), ExtractOptions{Workers: 2}, func(row PageText) error {
		rows[row.Index] = row
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if summary.Processed != 2 || summary.Succeeded != 1 || summary.Failed != 1 ||
		len(summary.FailedPages) != 1 || summary.FailedPages[0] != 1 ||
		len(summary.Failures) != 1 || summary.Failures[0].Code == "" {
		t.Fatalf("summary = %#v", summary)
	}
	if len(rows) != 2 || rows[0].Err != nil || rows[1].Err == nil {
		t.Fatalf("rows = %#v", rows)
	}
	if !strings.Contains(rows[0].Text.Text, "One") {
		t.Fatalf("page 0 text = %q", rows[0].Text.Text)
	}
}

func TestPreflightFailedPages(t *testing.T) {
	d := openTest(t, brokenPageData())
	defer d.Close()
	r, err := d.PreflightWithOptions(context.Background(), PreflightOptions{Workers: 2})
	if err != nil {
		t.Fatal(err)
	}
	if r.Failed != 1 || len(r.FailedPages) != 1 || r.FailedPages[0] != 1 {
		t.Fatalf("failed=%d pages=%v", r.Failed, r.FailedPages)
	}
	if len(r.Failures) != 1 || r.Failures[0].Page != 1 || r.Failures[0].Code == "" {
		t.Fatalf("failures = %#v", r.Failures)
	}
	if r.Scanned != 1 {
		t.Fatalf("scanned = %d", r.Scanned)
	}
}

func TestExtractTextEach(t *testing.T) {
	d := batchTextDoc(t)
	defer d.Close()
	seen := map[int]string{}
	summary, err := d.ExtractTextEach(context.Background(), ExtractOptions{Workers: 2}, func(row PageText) error {
		if row.Err != nil {
			return row.Err
		}
		seen[row.Index] = row.Text.Text
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if summary.Processed != 2 || summary.Succeeded != 2 || summary.Failed != 0 {
		t.Fatalf("summary = %#v", summary)
	}
	if len(seen) != 2 || !strings.Contains(seen[0], "One") || !strings.Contains(seen[1], "Two") {
		t.Fatalf("seen = %#v", seen)
	}
}

func TestExtractTextEachStopsOnCallbackError(t *testing.T) {
	d := batchTextDoc(t)
	defer d.Close()
	want := errors.New("stop")
	summary, err := d.ExtractTextEach(context.Background(), ExtractOptions{Workers: 2}, func(PageText) error {
		return want
	})
	if !errors.Is(err, want) {
		t.Fatalf("err = %v", err)
	}
	if summary == nil || summary.Processed != 1 {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestExtractTextEachRejectsNilCallback(t *testing.T) {
	d := batchTextDoc(t)
	defer d.Close()
	if _, err := d.ExtractTextEach(context.Background(), ExtractOptions{}, nil); err == nil {
		t.Fatal("expected nil callback error")
	}
}

func TestPreflightSelectionAndWorkers(t *testing.T) {
	d := batchTextDoc(t)
	defer d.Close()
	rep, err := d.PreflightWithOptions(context.Background(), PreflightOptions{Pages: []int{1}, Workers: 2})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Pages != 2 || rep.Scanned != 1 {
		t.Fatalf("preflight = %#v", rep)
	}
	if _, err := d.PreflightWithOptions(context.Background(), PreflightOptions{Pages: []int{2}}); err == nil {
		t.Fatal("expected page range error")
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := d.PreflightWithOptions(context.Background(), PreflightOptions{}); err == nil {
		t.Fatal("expected closed error")
	}
}

func TestStructOrderHelper(t *testing.T) {
	tree := []StructElement{
		{ID: 2, Page: 0},
		{ID: 0, Page: -1, Children: []StructElement{{ID: 3, Page: 1}}},
	}
	if got := structOrder(tree, 0); len(got) != 1 || got[0] != 2 {
		t.Fatalf("page 0 order = %v", got)
	}
	if got := structOrder(tree, 1); len(got) != 1 || got[0] != 3 {
		t.Fatalf("page 1 order = %v", got)
	}
}
