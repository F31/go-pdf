package pdf

import (
	"context"
	stdimage "image"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/F31/go-pdf/displaylist"
	"github.com/F31/go-pdf/internal/testkit"
)

type countFactory struct {
	enc   displaylist.ImageEncoding
	calls *atomic.Int32
}

func (f countFactory) Encodings() []displaylist.ImageEncoding {
	return []displaylist.ImageEncoding{f.enc}
}

func (f countFactory) NewDecoder(ctx context.Context, info displaylist.ImageInfo) (ExternalDecoder, error) {
	f.calls.Add(1)
	return nil, nil
}

func TestDecoderRegistryOverride(t *testing.T) {
	var a, b atomic.Int32
	base, err := NewDecoderRegistry(countFactory{enc: "JBIG2Decode", calls: &a})
	if err != nil {
		t.Fatal(err)
	}
	derived, err := base.WithOverride("JPXDecode", countFactory{enc: "JPXDecode", calls: &b})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if d, err := base.decoder(ctx, "JPXDecode", displaylist.ImageInfo{}); err != nil || d != nil {
		t.Fatalf("base inherited override: %v %v", d, err)
	}
	if _, err := derived.decoder(ctx, "JPXDecode", displaylist.ImageInfo{}); err != nil {
		t.Fatal(err)
	}
	if b.Load() != 1 {
		t.Fatalf("derived calls %d", b.Load())
	}
	if _, err := base.decoder(ctx, "JBIG2Decode", displaylist.ImageInfo{}); err != nil {
		t.Fatal(err)
	}
	if a.Load() != 1 {
		t.Fatalf("base calls %d", a.Load())
	}
}

func TestDecoderRegistryConcurrentTasks(t *testing.T) {
	var calls atomic.Int32
	reg, err := NewDecoderRegistry(countFactory{enc: "JBIG2Decode", calls: &calls})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = reg.decoder(context.Background(), "JBIG2Decode", displaylist.ImageInfo{})
		}()
	}
	wg.Wait()
	if calls.Load() != 8 {
		t.Fatalf("factory calls %d", calls.Load())
	}
}

type sessionFactory struct{ calls *atomic.Int32 }

func (f sessionFactory) Encodings() []displaylist.ImageEncoding {
	return []displaylist.ImageEncoding{"JBIG2Decode"}
}
func (f sessionFactory) NewDecoder(ctx context.Context, info displaylist.ImageInfo) (ExternalDecoder, error) {
	return sessionDecoder(f), nil
}

type sessionDecoder struct{ calls *atomic.Int32 }

func (d sessionDecoder) Decode(ctx context.Context, raw []byte, info displaylist.ImageInfo) (stdimage.Image, error) {
	d.calls.Add(1)
	return stdimage.NewGray(stdimage.Rect(0, 0, info.Width, info.Height)), nil
}

func TestDecoderSessionsPerPageTask(t *testing.T) {
	var calls atomic.Int32
	reg, err := NewDecoderRegistry(sessionFactory{calls: &calls})
	if err != nil {
		t.Fatal(err)
	}
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << /XObject << /Im1 5 0 R >> >> /Contents 6 0 R >>"),
		testkit.Raw(4, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << /XObject << /Im1 5 0 R >> >> /Contents 7 0 R >>"),
		testkit.Stream(5, "<< /Type /XObject /Subtype /Image /Width 1 /Height 1 /BitsPerComponent 1 /ColorSpace /DeviceGray /Filter /JBIG2Decode >>", []byte{1}),
		testkit.Stream(6, "", []byte("/Im1 Do")),
		testkit.Stream(7, "", []byte("/Im1 Do")),
	}
	d := openTest(t, testkit.Build(objs, "/Root 1 0 R"), WithDecoderRegistry(reg))
	defer d.Close()
	var refs []displaylist.ImageRef
	for i := 0; i < 2; i++ {
		p, _ := d.Page(i)
		dl, err := p.DisplayList(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		refs = append(refs, dl.Items[0].(*displaylist.Image).Data)
	}
	var wg sync.WaitGroup
	for _, ref := range refs {
		wg.Add(1)
		go func(ref displaylist.ImageRef) {
			defer wg.Done()
			if _, err := ref.Decode(context.Background()); err != nil {
				t.Errorf("decode: %v", err)
			}
		}(ref)
	}
	wg.Wait()
	if calls.Load() != 2 {
		t.Fatalf("factory sessions %d", calls.Load())
	}
}
