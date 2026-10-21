package content

import (
	"testing"

	"github.com/F31/go-pdf/internal/syntax"
)

func TestLexerSequence(t *testing.T) {
	l := newLexer([]byte("BT /F1 12 Tf (Hi) Tj ET"), 64)
	var ops []string
	var objs []syntax.Object
	for {
		tok, err := l.next()
		if err == errEOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if tok.kind == tokOperator {
			ops = append(ops, tok.op)
		} else {
			objs = append(objs, tok.obj)
		}
	}
	if len(ops) != 4 {
		t.Fatalf("ops %v", ops)
	}
	if ops[0] != "BT" || ops[1] != "Tf" || ops[2] != "Tj" || ops[3] != "ET" {
		t.Fatalf("ops %v", ops)
	}
	if len(objs) != 3 {
		t.Fatalf("objs %v", objs)
	}
}

func TestLexerArrayDict(t *testing.T) {
	l := newLexer([]byte("[(A) -100 (B)] << /Type /X >>"), 64)
	tok, err := l.next()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := tok.obj.(interface{}); !ok {
		t.Fatal("array")
	}
	tok, err = l.next()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := tok.obj.(interface{}); !ok {
		t.Fatal("dict")
	}
}

func TestInlineImageSkip(t *testing.T) {
	data := []byte("q BI /W 1 /H 1 /BPC 8 /CS /G ID \x00\x01\nEI Q")
	l := newLexer(data, 64)
	tok, err := l.next()
	if err != nil || tok.op != "q" {
		t.Fatalf("q: %v %v", tok, err)
	}
	tok, err = l.next()
	if err != nil || tok.op != "BI" {
		t.Fatalf("BI: %v %v", tok, err)
	}
	l.skipInlineImage()
	tok, err = l.next()
	if err != nil || tok.op != "Q" {
		t.Fatalf("Q after inline: %v %v", tok, err)
	}
}

func FuzzContentLexer(f *testing.F) {
	f.Add([]byte("BT /F1 12 Tf (Hi) Tj ET"))
	f.Add([]byte("[(A) 100 (B)] TJ"))
	f.Add([]byte("q 1 0 0 1 0 0 cm Q"))
	f.Fuzz(func(t *testing.T, data []byte) {
		l := newLexer(data, 32)
		for i := 0; i < 10000; i++ {
			if _, err := l.next(); err != nil {
				return
			}
		}
	})
}
