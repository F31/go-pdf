package pdf

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/F31/go-pdf/displaylist"
	"github.com/F31/go-pdf/internal/testkit"
)

func TestEngineSharedRegistryAndClose(t *testing.T) {
	data := testkit.Build([]testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << /XObject << /Im1 5 0 R >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("/Im1 Do")),
		testkit.Stream(5, "<< /Type /XObject /Subtype /Image /Width 1 /Height 1 /BitsPerComponent 1 /ColorSpace /DeviceGray /Filter /JBIG2Decode >>", []byte{1}),
	}, "/Root 1 0 R")
	reg, err := NewDecoderRegistry(fakeJBIG2Factory{dec: &fakeJBIG2Decoder{}})
	if err != nil {
		t.Fatal(err)
	}
	eng, err := NewEngine(WithDecoderRegistry(reg))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		doc, err := eng.Open(context.Background(), bytes.NewReader(data), int64(len(data)))
		if err != nil {
			t.Fatal(err)
		}
		p, _ := doc.Page(0)
		dl, err := p.DisplayList(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		im := dl.Items[0].(*displaylist.Image)
		if _, err := im.Data.Decode(context.Background()); err != nil {
			t.Fatal(err)
		}
		_ = doc.Close()
	}
	if err := eng.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := eng.Open(context.Background(), bytes.NewReader(data), int64(len(data))); !errors.Is(err, ErrClosed) {
		t.Fatalf("open after close: %v", err)
	}
}
