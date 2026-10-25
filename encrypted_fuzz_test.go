package pdf

import (
	"bytes"
	"context"
	"os"
	"testing"
)

func FuzzEncryptedOpen(f *testing.F) {
	for _, path := range []string{"testdata/encrypted/rc4-128.pdf", "testdata/encrypted/aes-256.pdf"} {
		if data, err := os.ReadFile(path); err == nil {
			f.Add(data)
		}
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		doc, err := Open(context.Background(), bytes.NewReader(data), int64(len(data)), WithPassword("user"))
		if err != nil {
			return
		}
		defer doc.Close()
		if doc.NumPages() == 0 {
			return
		}
		page, err := doc.Page(0)
		if err != nil {
			return
		}
		_, _ = page.DisplayList(context.Background())
	})
}
