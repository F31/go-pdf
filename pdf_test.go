package pdf

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"

	"github.com/F31/go-pdf/displaylist"
	"github.com/F31/go-pdf/internal/testkit"
)

func openTest(t *testing.T, data []byte, opts ...Option) *Document {
	t.Helper()
	d, err := Open(context.Background(), bytes.NewReader(data), int64(len(data)), opts...)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return d
}

func TestPublicOpenAndPage(t *testing.T) {
	data := testkit.Basic("0 0 200 100")
	d := openTest(t, data)
	defer d.Close()
	if d.NumPages() != 1 {
		t.Fatalf("pages %d", d.NumPages())
	}
	p, err := d.Page(0)
	if err != nil {
		t.Fatal(err)
	}
	w, h := p.Size()
	if w != 200 || h != 100 {
		t.Fatalf("size %v x %v", w, h)
	}
	if p.Index() != 0 {
		t.Fatalf("index %d", p.Index())
	}
	dl, err := p.DisplayList(context.Background())
	if err != nil {
		t.Fatalf("display list: %v", err)
	}
	if dl.Width != 200 || dl.Height != 100 {
		t.Fatalf("display list size %v x %v", dl.Width, dl.Height)
	}
	if len(dl.Items) != 0 {
		t.Fatalf("expected no items for empty page, got %d", len(dl.Items))
	}
	if err := p.Walk(context.Background(), func(displaylist.Item) error { return nil }); err != nil {
		t.Fatalf("walk: %v", err)
	}
}

func TestPublicClose(t *testing.T) {
	d := openTest(t, testkit.Basic("0 0 10 10"))
	_ = d.Close()
	if _, err := d.Page(0); !errors.Is(err, ErrClosed) {
		t.Fatalf("got %v", err)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestBudgetOption(t *testing.T) {
	data := testkit.Basic("0 0 10 10")
	b := DefaultBudget
	b.MaxFileSize = 10
	_, err := Open(context.Background(), bytes.NewReader(data), int64(len(data)), WithBudget(&b))
	if !errors.Is(err, ErrBudget) {
		t.Fatalf("got %v", err)
	}
}

func TestEncryptedPublic(t *testing.T) {
	_, err := OpenFile(context.Background(), "testdata/encrypted/rc4-128.pdf")
	if !errors.Is(err, ErrPassword) {
		t.Fatalf("got %v", err)
	}
}

func TestErrorType(t *testing.T) {
	_, err := Open(context.Background(), bytes.NewReader([]byte("not a pdf")), 9)
	if !errors.Is(err, ErrCorrupt) {
		t.Fatalf("got %v", err)
	}
	var pe *Error
	if !errors.As(err, &pe) {
		t.Fatalf("not *pdf.Error: %T", err)
	}
	if pe.Op == "" {
		t.Fatal("missing op")
	}
}

func TestDisplayListVersion(t *testing.T) {
	if displaylist.Version.Major != 1 {
		t.Fatalf("version %v", displaylist.Version)
	}
	if !displaylist.Compatible(1) || displaylist.Compatible(2) {
		t.Fatal("compatibility check")
	}
}

func TestConcurrentPageAccess(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R 4 0 R 5 0 R 6 0 R] /Count 4 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] >>"),
		testkit.Raw(4, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] >>"),
		testkit.Raw(5, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 300] >>"),
		testkit.Raw(6, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 400 400] >>"),
	}
	d := openTest(t, testkit.Build(objs, "/Root 1 0 R"))
	defer d.Close()
	done := make(chan error, 8)
	for i := 0; i < 8; i++ {
		go func(n int) {
			for j := 0; j < 50; j++ {
				p, err := d.Page(n % 4)
				if err != nil {
					done <- err
					return
				}
				p.Size()
			}
			done <- nil
		}(i)
	}
	for i := 0; i < 8; i++ {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
}

func TestErrorsDoNotLeakText(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 10 10] /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("(TOPSECRET) Tj")),
	}
	data := testkit.CorruptStartXref(testkit.Build(objs, "/Root 1 0 R"))
	_, err := Open(context.Background(), bytes.NewReader(data), int64(len(data)))
	if err == nil {
		t.Fatal("expected error")
	}
	if contains(err.Error(), "TOPSECRET") {
		t.Fatal("error leaked document text")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestResourceUsage(t *testing.T) {
	d := openTest(t, testkit.Basic("0 0 10 10"))
	u := d.ResourceUsage()
	if u.LiveBytes <= 0 || u.PeakBytes < u.LiveBytes || u.MaxLiveBytes <= 0 {
		t.Fatalf("usage %#v", u)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	u = d.ResourceUsage()
	if u.LiveBytes != 0 || u.PeakBytes <= 0 {
		t.Fatalf("after close %#v", u)
	}
}

func TestLiveMemoryBudget(t *testing.T) {
	data := testkit.Basic("0 0 10 10")
	b := DefaultBudget
	b.MaxLiveBytes = 1
	_, err := Open(context.Background(), bytes.NewReader(data), int64(len(data)), WithBudget(&b))
	if !errors.Is(err, ErrBudget) {
		t.Fatalf("got %v", err)
	}
}

func TestPreflight(t *testing.T) {
	d := openTest(t, testkit.Basic("0 0 10 10"))
	rep, err := d.Preflight(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rep.Pages != 1 || rep.Scanned != 1 {
		t.Fatalf("pages %#v", rep)
	}
	if rep.Domains[DomainExtract].Status != CapabilitySupported {
		t.Fatalf("extract %#v", rep.Domains[DomainExtract])
	}
	if rep.Domains[DomainRender].Status != CapabilityApproximate {
		t.Fatalf("render %#v", rep.Domains[DomainRender])
	}
	if rep.Domains[DomainEdit].Status != CapabilityUnsupported {
		t.Fatalf("edit %#v", rep.Domains[DomainEdit])
	}
}

func TestPageMemoryLease(t *testing.T) {
	d := openTest(t, testkit.Basic("0 0 10 10"))
	p, _ := d.Page(0)
	before := d.ResourceUsage().LiveBytes
	release, err := p.AcquireMemory(128)
	if err != nil {
		t.Fatal(err)
	}
	if d.ResourceUsage().LiveBytes != before+128 {
		t.Fatalf("live %#v", d.ResourceUsage())
	}
	release()
	if d.ResourceUsage().LiveBytes != before {
		t.Fatalf("release %#v", d.ResourceUsage())
	}
}

func TestOpenFileMappedAndReleased(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "mapped-*.pdf")
	if err != nil {
		t.Fatal(err)
	}
	data := testkit.Basic("0 0 10 10")
	if _, err := f.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	d, err := OpenFile(context.Background(), f.Name())
	if err != nil {
		t.Fatal(err)
	}
	u := d.ResourceUsage()
	if u.LiveBytes < int64(len(data)) || u.PeakBytes < u.LiveBytes {
		t.Fatalf("usage %#v", u)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	if got := d.ResourceUsage().LiveBytes; got != 0 {
		t.Fatalf("live after close %d", got)
	}
}
