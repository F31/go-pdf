package displaylist

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// CanonicalJSON serializes a value with Go's deterministic struct/map ordering.
// It is intended for golden hashes and consumer contract tests.
func CanonicalJSON(v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("displaylist: canonical json: %w", err)
	}
	return b, nil
}

// Hash returns the SHA-256 of the canonical JSON encoding.
func Hash(v any) (string, error) {
	b, err := CanonicalJSON(v)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

func HashPage(p *Page) (string, error) { return Hash(p) }
