package displaylist

import "testing"

func TestSignatureFloatStability(t *testing.T) {
	a := sigWriter{h: fnvOffset}
	b := sigWriter{h: fnvOffset}
	a.f(1)
	b.f(1 + 1e-15)
	if a.h != b.h {
		t.Fatalf("near-equal floats produced different signatures: %d != %d", a.h, b.h)
	}
	c := sigWriter{h: fnvOffset}
	c.f(1.001)
	if a.h == c.h {
		t.Fatal("distinct floats produced the same signature")
	}
}
