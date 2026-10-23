package crypt

import (
	"encoding/hex"
	"testing"
)

func TestStandardKeyVector(t *testing.T) {
	o, _ := hex.DecodeString("0ba3835f88f90388e74e54584125ce142be0de24c6b0d37746e075b891756671")
	id, _ := hex.DecodeString("3533383931653066306339643034646433646632306431623639313531363736")
	u, _ := hex.DecodeString("ab5770ae4432560b28f9ca45118df08728bf4e5e4e758a4164004e56fffa0108")
	opts := Options{V: 2, R: 3, Length: 128, P: 4294967292, O: o, U: u, ID: id, EncryptMetadata: true}
	key := fileKey(opts, padPassword([]byte("user")), 16)
	if got := hex.EncodeToString(key); got != "14350fc8100b1b6eb48868710a41a238" {
		t.Fatalf("key %s", got)
	}
	if _, ok := validateUser(opts, padPassword([]byte("user")), 16); !ok {
		t.Fatal("user password rejected")
	}
}
