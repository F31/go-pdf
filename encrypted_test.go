package pdf

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestEncryptedFixtures(t *testing.T) {
	for _, path := range []string{"testdata/encrypted/rc4-40.pdf", "testdata/encrypted/rc4-128.pdf", "testdata/encrypted/aes-128.pdf", "testdata/encrypted/aes-256-r5.pdf", "testdata/encrypted/aes-256.pdf"} {
		t.Run(path, func(t *testing.T) {
			if _, err := OpenFile(context.Background(), path); !errors.Is(err, ErrPassword) {
				t.Fatalf("missing password: %v", err)
			}
			if _, err := OpenFile(context.Background(), path, WithPassword("wrong")); !errors.Is(err, ErrPassword) {
				t.Fatalf("wrong password: %v", err)
			}
			d, err := OpenFile(context.Background(), path, WithPassword("user"))
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			if d.Info().Title != "Encrypted fixture" {
				t.Fatalf("title %q", d.Info().Title)
			}
			p, err := d.Page(0)
			if err != nil {
				t.Fatal(err)
			}
			tx, err := p.Text(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(tx.Text, "Hello encrypted") {
				t.Fatalf("text %q", tx.Text)
			}
		})
	}
}

func TestEncryptedOwnerPassword(t *testing.T) {
	for _, path := range []string{"testdata/encrypted/aes-128.pdf", "testdata/encrypted/aes-256.pdf"} {
		d, err := OpenFile(context.Background(), path, WithPassword("owner"))
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if d.NumPages() != 1 {
			t.Fatalf("%s pages %d", path, d.NumPages())
		}
		_ = d.Close()
	}
}

func TestEncryptedSASLPrep(t *testing.T) {
	d, err := OpenFile(context.Background(), "testdata/encrypted/aes-256-sasl.pdf", WithPassword("pa\u00a0ss"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if d.NumPages() != 1 {
		t.Fatalf("pages %d", d.NumPages())
	}
}
