package pdf_test

import (
	"bytes"
	"context"
	stdimage "image"
	stdcolor "image/color"
	"testing"

	"github.com/F31/go-pdf"
	"github.com/F31/go-pdf/displaylist"
	"github.com/F31/go-pdf/internal/testkit"
)

type externalJBIG2 struct{}
type externalJBIG2Factory struct{}

func (externalJBIG2Factory) Encodings() []displaylist.ImageEncoding {
	return []displaylist.ImageEncoding{"JBIG2Decode"}
}
func (externalJBIG2Factory) NewDecoder(ctx context.Context, info displaylist.ImageInfo) (pdf.ExternalDecoder, error) {
	return externalJBIG2{}, nil
}

func (externalJBIG2) Decode(ctx context.Context, raw []byte, info displaylist.ImageInfo) (stdimage.Image, error) {
	img := stdimage.NewGray(stdimage.Rect(0, 0, info.Width, info.Height))
	for y := 0; y < info.Height; y++ {
		for x := 0; x < info.Width; x++ {
			img.SetGray(x, y, stdcolor.Gray{Y: 77})
		}
	}
	return img, nil
}

func TestDocumentScopedDecoderRegistry(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << /XObject << /Im1 5 0 R >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("/Im1 Do")),
		testkit.Stream(5, "<< /Type /XObject /Subtype /Image /Width 1 /Height 1 /BitsPerComponent 8 /ColorSpace /DeviceGray /Filter /JBIG2Decode >>", []byte{1}),
	}
	data := testkit.Build(objs, "/Root 1 0 R")
	reg, err := pdf.NewDecoderRegistry(externalJBIG2Factory{})
	if err != nil {
		t.Fatal(err)
	}
	doc, err := pdf.Open(context.Background(), bytes.NewReader(data), int64(len(data)), pdf.WithDecoderRegistry(reg))
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()
	page, err := doc.Page(0)
	if err != nil {
		t.Fatal(err)
	}
	dl, err := page.DisplayList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	im := dl.Items[0].(*displaylist.Image)
	img, err := im.Data.Decode(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if img.(*stdimage.Gray).GrayAt(0, 0).Y != 77 {
		t.Fatalf("decoded pixel %d", img.(*stdimage.Gray).GrayAt(0, 0).Y)
	}
}
