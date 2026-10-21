package syntax

import (
	"context"
	"errors"
	"testing"

	"github.com/F31/go-pdf/internal/budget"
	"github.com/F31/go-pdf/internal/diag"
	"github.com/F31/go-pdf/internal/errs"
	"github.com/F31/go-pdf/internal/testkit"
)

func openBytes(t *testing.T, data []byte, repair bool) *File {
	t.Helper()
	b := budget.DefaultBudget
	f, err := Open(context.Background(), data, Options{
		Repair: repair, Tracker: budget.NewTracker(&b), Diag: diag.New(1000),
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return f
}

func TestClassicXrefAndPages(t *testing.T) {
	f := openBytes(t, testkit.Basic("0 0 200 100"), false)
	defer f.Close()
	root, err := f.ResolveRoot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if typ, _ := root.NameValue("Type"); typ != "Catalog" {
		t.Fatalf("root type %q", typ)
	}
	pagesRef, ok := root.RefValue("Pages")
	if !ok {
		t.Fatal("no pages ref")
	}
	pages, err := f.ResolveRef(context.Background(), pagesRef)
	if err != nil {
		t.Fatal(err)
	}
	d := pages.(Dict)
	kids, _ := d.ArrayValue("Kids")
	if len(kids) != 1 {
		t.Fatalf("kids %d", len(kids))
	}
	page, err := f.Resolve(context.Background(), kids[0])
	if err != nil {
		t.Fatal(err)
	}
	if typ, _ := page.(Dict).NameValue("Type"); typ != "Page" {
		t.Fatalf("page type %q", typ)
	}
}

func TestIncrementalUpdateNewestWins(t *testing.T) {
	base := testkit.Basic("0 0 200 100")
	updated := testkit.IncrementalUpdate(base, testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 400] >>"))
	f := openBytes(t, updated, false)
	defer f.Close()
	o, err := f.ResolveRef(context.Background(), Ref{Num: 3})
	if err != nil {
		t.Fatal(err)
	}
	box, _ := o.(Dict).ArrayValue("MediaBox")
	n, _ := AsNumber(box[2])
	if n != 300 {
		t.Fatalf("media box width %v", n)
	}
}

func TestXRefStream(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
	}
	objs = append(objs, testkit.ObjectStream(4, map[int]string{3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] >>"}))
	data := testkit.BuildXRefStream(objs, map[int][2]int{3: {4, 0}}, "/Root 1 0 R")
	f := openBytes(t, data, false)
	defer f.Close()
	o, err := f.ResolveRef(context.Background(), Ref{Num: 3})
	if err != nil {
		t.Fatalf("resolve compressed: %v", err)
	}
	if _, ok := o.(Dict); !ok {
		t.Fatalf("compressed object type %T", o)
	}
}

func TestRepairMode(t *testing.T) {
	data := testkit.CorruptStartXref(testkit.Basic("0 0 200 100"))
	if _, err := Open(context.Background(), data, Options{Repair: false, Tracker: budget.NewTracker(&budget.DefaultBudget), Diag: diag.New(100)}); !errors.Is(err, errs.ErrCorrupt) {
		t.Fatalf("strict error = %v", err)
	}
	f := openBytes(t, data, true)
	defer f.Close()
	if !f.Repaired() {
		t.Fatal("expected Repaired")
	}
	if _, err := f.ResolveRoot(context.Background()); err != nil {
		t.Fatalf("root after repair: %v", err)
	}
}

func TestMissingObjectIsNull(t *testing.T) {
	f := openBytes(t, testkit.Basic("0 0 200 100"), false)
	defer f.Close()
	o, err := f.ResolveRef(context.Background(), Ref{Num: 99})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := o.(Null); !ok {
		t.Fatalf("got %T", o)
	}
}

func TestClosedFile(t *testing.T) {
	f := openBytes(t, testkit.Basic("0 0 200 100"), false)
	_ = f.Close()
	if _, err := f.ResolveRef(context.Background(), Ref{Num: 1}); !errors.Is(err, errs.ErrClosed) {
		t.Fatalf("got %v", err)
	}
}

func BenchmarkParseObject(b *testing.B) {
	data := []byte("<< /Type /Page /MediaBox [0 0 612 792] /Resources << /Font << /F1 4 0 R >> >> >>")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		p := newParser(data, 0, false, 64)
		if _, err := p.parseObject(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkResolveObject(b *testing.B) {
	data := testkit.Basic("0 0 612 792")
	bd := budget.DefaultBudget
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		f, err := Open(context.Background(), data, Options{Tracker: budget.NewTracker(&bd), Diag: diag.New(10)})
		if err != nil {
			b.Fatal(err)
		}
		if _, err := f.ResolveRef(context.Background(), Ref{Num: 3}); err != nil {
			b.Fatal(err)
		}
		_ = f.Close()
	}
}

func FuzzOpenFile(f *testing.F) {
	f.Add(testkit.Basic("0 0 200 100"))
	f.Add([]byte("%PDF-1.7\n"))
	f.Add([]byte("not a pdf"))
	f.Fuzz(func(t *testing.T, data []byte) {
		b := budget.DefaultBudget
		file, err := Open(context.Background(), data, Options{Repair: true, Tracker: budget.NewTracker(&b), Diag: diag.New(100)})
		if err == nil {
			_, _ = file.ResolveRoot(context.Background())
			_ = file.Close()
		}
	})
}

func BenchmarkOpenClassicXref(b *testing.B) {
	data := testkit.Basic("0 0 612 792")
	bd := budget.DefaultBudget
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		f, err := Open(context.Background(), data, Options{Tracker: budget.NewTracker(&bd), Diag: diag.New(10)})
		if err != nil {
			b.Fatal(err)
		}
		_ = f.Close()
	}
}
