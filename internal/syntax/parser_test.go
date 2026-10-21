package syntax

import (
	"errors"
	"testing"

	"github.com/F31/go-pdf/internal/errs"
)

func parseOne(t *testing.T, src string) Object {
	t.Helper()
	p := newParser([]byte(src), 0, false, 64)
	o, err := p.parseObject()
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	return o
}

func TestParseScalarsAndContainers(t *testing.T) {
	if n, ok := parseOne(t, "42").(Int); !ok || n != 42 {
		t.Fatalf("int mismatch: %#v", parseOne(t, "42"))
	}
	if r, ok := parseOne(t, "-1.5").(Real); !ok || r != -1.5 {
		t.Fatalf("real mismatch")
	}
	if b, ok := parseOne(t, "true").(Bool); !ok || !bool(b) {
		t.Fatalf("bool mismatch")
	}
	if _, ok := parseOne(t, "null").(Null); !ok {
		t.Fatalf("null mismatch")
	}
	if n, ok := parseOne(t, "/A#20B").(Name); !ok || string(n) != "A B" {
		t.Fatalf("name mismatch: %q", n)
	}
	if s, ok := parseOne(t, `(a(b)c)`).(String); !ok || string(s) != "a(b)c" {
		t.Fatalf("string mismatch: %q", s)
	}
	if s, ok := parseOne(t, `<6162F>`).(String); !ok || string(s) != "\x61\x62\xf0" {
		t.Fatalf("hex mismatch: %x", s)
	}
	if r, ok := parseOne(t, "12 0 R").(Ref); !ok || r.Num != 12 || r.Gen != 0 {
		t.Fatalf("ref mismatch: %#v", r)
	}
	arr := parseOne(t, "[1 2 3]").(Array)
	if len(arr) != 3 {
		t.Fatalf("array len %d", len(arr))
	}
	d := parseOne(t, "<< /A 1 /B (x) >>").(Dict)
	if v, _ := d.IntValue("A"); v != 1 {
		t.Fatalf("dict A %v", d.Get("A"))
	}
}

func TestParseStringEscapes(t *testing.T) {
	cases := map[string]string{
		`(a\nb)`:   "a\nb",
		`(a\101b)`: "aAb",
		`(a\
b)`: "ab",
		`(a\tb)`: "a\tb",
	}
	for src, want := range cases {
		if got := string(parseOne(t, src).(String)); got != want {
			t.Errorf("%q = %q, want %q", src, got, want)
		}
	}
}

func TestMalformedNumberStrictAndLenient(t *testing.T) {
	p := newParser([]byte("1.2.3"), 0, false, 64)
	if _, err := p.parseObject(); !errors.Is(err, errs.ErrCorrupt) {
		t.Fatalf("strict error = %v", err)
	}
	p = newParser([]byte("1.2.3"), 0, true, 64)
	o, err := p.parseObject()
	if err != nil {
		t.Fatalf("lenient: %v", err)
	}
	if _, ok := o.(Real); !ok {
		t.Fatalf("lenient type %T", o)
	}
}

func TestNestingLimit(t *testing.T) {
	src := "["
	for i := 0; i < 10; i++ {
		src += "["
	}
	p := newParser([]byte(src), 0, false, 4)
	if _, err := p.parseObject(); !errors.Is(err, errs.ErrBudget) {
		t.Fatalf("expected budget error, got %v", err)
	}
}

func TestDictionaryDuplicateKeyLastWins(t *testing.T) {
	d := parseOne(t, "<< /A 1 /A 2 >>").(Dict)
	if v, _ := d.IntValue("A"); v != 2 {
		t.Fatalf("A = %d", v)
	}
}

func FuzzParseObject(f *testing.F) {
	for _, s := range []string{"1", "/Name", "(text)", "<< /A [1 2] >>", "12 0 R", "<6162>"} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		p := newParser(data, 0, true, 64)
		_, _ = p.parseObject()
	})
}
