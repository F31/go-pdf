package crypt

import (
	"strings"

	"golang.org/x/text/unicode/norm"
)

// saslprep applies the common SASLprep mapping and NFKC normalization used by
// PDF 2.0 AES-256 passwords. A password that fails the prohibited-character
// checks is returned as-is so callers can still try the UTF-8 form.
func saslprep(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == 0x00AD || r == 0x034F || r == 0x1806 || (r >= 0x180B && r <= 0x180D) ||
			(r >= 0x200B && r <= 0x200D) || r == 0x2060 || (r >= 0xFE00 && r <= 0xFE0F) || r == 0xFEFF:
			continue
		case r == 0x00A0 || r == 0x1680 || (r >= 0x2000 && r <= 0x200B) || r == 0x202F || r == 0x205F || r == 0x3000:
			b.WriteByte(' ')
		default:
			b.WriteRune(r)
		}
	}
	out := norm.NFKC.String(b.String())
	for _, r := range out {
		if prohibited(r) {
			return s
		}
	}
	return out
}

func prohibited(r rune) bool {
	return r <= 0x1F || r == 0x7F || (r >= 0x80 && r <= 0x9F) ||
		r == 0x06DD || r == 0x070F || r == 0x180E ||
		(r >= 0x200C && r <= 0x200F) || (r >= 0x202A && r <= 0x202E) ||
		(r >= 0x206A && r <= 0x206F) || r == 0xFEFF ||
		(r >= 0xFFF9 && r <= 0xFFFD) ||
		(r >= 0xD800 && r <= 0xDFFF) ||
		(r >= 0xE000 && r <= 0xF8FF) || (r >= 0xFDD0 && r <= 0xFDEF) ||
		(r >= 0xFFFE && r <= 0xFFFF) ||
		(r >= 0xF0000 && r <= 0xFFFFD) || (r >= 0x100000 && r <= 0x10FFFD) ||
		(r >= 0x2FF0 && r <= 0x2FFB) || r == 0xE0001 || (r >= 0xE0020 && r <= 0xE007F) ||
		(r >= 0x1D173 && r <= 0x1D17A)
}
