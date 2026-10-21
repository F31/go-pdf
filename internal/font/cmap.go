package font

import (
	"strconv"
	"strings"

	"github.com/F31/go-pdf/internal/errs"
)

type cmapToken struct {
	kind byte // 'h' hex, 'n' number, 'k' keyword, 'a' array, 's' string/name
	hex  []byte
	num  int64
	kw   string
	arr  []cmapToken
	pos  int
}

type cmapLexer struct {
	data []byte
	pos  int
}

func (l *cmapLexer) skip() {
	for l.pos < len(l.data) {
		c := l.data[l.pos]
		if c == 0 || c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == '\f' {
			l.pos++
			continue
		}
		if c == '%' {
			for l.pos < len(l.data) && l.data[l.pos] != '\n' && l.data[l.pos] != '\r' {
				l.pos++
			}
			continue
		}
		break
	}
}

func (l *cmapLexer) next() (cmapToken, bool, error) {
	l.skip()
	if l.pos >= len(l.data) {
		return cmapToken{}, false, nil
	}
	start := l.pos
	c := l.data[l.pos]
	switch {
	case c == '<':
		l.pos++
		var out []byte
		hi, have := 0, false
		for l.pos < len(l.data) && l.data[l.pos] != '>' {
			v, ok := hexVal(l.data[l.pos])
			l.pos++
			if !ok {
				continue
			}
			if !have {
				hi, have = v, true
			} else {
				out = append(out, byte(hi<<4|v))
				have = false
			}
		}
		if l.pos < len(l.data) {
			l.pos++
		}
		if have {
			out = append(out, byte(hi<<4))
		}
		return cmapToken{kind: 'h', hex: out, pos: start}, true, nil
	case c == '[':
		l.pos++
		var arr []cmapToken
		for {
			l.skip()
			if l.pos >= len(l.data) {
				return cmapToken{kind: 'a', arr: arr, pos: start}, true, errs.Wrap(errs.ErrCorrupt, "FNT-CMAP-ARRAY", "parse ToUnicode CMap", int64(start), 0, -1)
			}
			if l.data[l.pos] == ']' {
				l.pos++
				return cmapToken{kind: 'a', arr: arr, pos: start}, true, nil
			}
			t, ok, err := l.next()
			if err != nil || !ok {
				return cmapToken{kind: 'a', arr: arr, pos: start}, true, errs.Wrap(errs.ErrCorrupt, "FNT-CMAP-ARRAY", "parse ToUnicode CMap", int64(start), 0, -1)
			}
			arr = append(arr, t)
		}
	case c == '/':
		l.pos++
		s := l.pos
		for l.pos < len(l.data) && !isDelim(l.data[l.pos]) {
			l.pos++
		}
		return cmapToken{kind: 's', kw: string(l.data[s:l.pos]), pos: start}, true, nil
	case c == '+' || c == '-' || (c >= '0' && c <= '9'):
		l.pos++
		for l.pos < len(l.data) && ((l.data[l.pos] >= '0' && l.data[l.pos] <= '9') || l.data[l.pos] == '.') {
			l.pos++
		}
		n, _ := strconv.ParseInt(string(l.data[start:l.pos]), 10, 64)
		return cmapToken{kind: 'n', num: n, pos: start}, true, nil
	}
	s := l.pos
	for l.pos < len(l.data) && !isDelim(l.data[l.pos]) {
		l.pos++
	}
	kw := string(l.data[s:l.pos])
	if kw == "" {
		l.pos++
		return cmapToken{kind: 'k', kw: string(c), pos: start}, true, nil
	}
	return cmapToken{kind: 'k', kw: kw, pos: start}, true, nil
}

func hexVal(c byte) (int, bool) {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0'), true
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10, true
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10, true
	}
	return 0, false
}

func isDelim(c byte) bool {
	switch c {
	case '(', ')', '<', '>', '[', ']', '{', '}', '/', '%', ' ', '\t', '\r', '\n', '\f', 0:
		return true
	}
	return false
}

type codeSpace struct {
	Bytes int
	Lo    uint32
	Hi    uint32
}

type ToUnicode struct {
	Map     map[uint32]string
	CodeLen int
	Spaces  []codeSpace
}

func ParseToUnicode(data []byte) (*ToUnicode, error) {
	l := &cmapLexer{data: data}
	cm := &ToUnicode{Map: map[uint32]string{}, CodeLen: 2}
	var errsList []error
	for {
		t, ok, err := l.next()
		if err != nil {
			errsList = append(errsList, err)
			break
		}
		if !ok {
			break
		}
		if t.kind != 'k' {
			continue
		}
		switch t.kw {
		case "begincodespacerange":
			if err := cm.parseCodespace(l); err != nil {
				errsList = append(errsList, err)
			}
		case "beginbfchar":
			if err := cm.parseBFChar(l); err != nil {
				errsList = append(errsList, err)
			}
		case "beginbfrange":
			if err := cm.parseBFRange(l); err != nil {
				errsList = append(errsList, err)
			}
		}
	}
	return cm, joinErrors(errsList)
}

func joinErrors(list []error) error {
	if len(list) == 0 {
		return nil
	}
	return list[0]
}

func codeOf(b []byte) uint32 {
	var v uint32
	for _, c := range b {
		v = v<<8 | uint32(c)
	}
	return v
}

func (cm *ToUnicode) parseCodespace(l *cmapLexer) error {
	for {
		t, ok, err := l.next()
		if err != nil || !ok {
			return errs.Wrap(errs.ErrCorrupt, "FNT-CMAP-CODESPACE", "parse codespace", -1, 0, -1)
		}
		if t.kind == 'k' && t.kw == "endcodespacerange" {
			return nil
		}
		if t.kind != 'h' {
			continue
		}
		src := t
		dst, ok, err := l.next()
		if err != nil || !ok || dst.kind != 'h' {
			return errs.Wrap(errs.ErrCorrupt, "FNT-CMAP-CODESPACE", "parse codespace", -1, 0, -1)
		}
		n := len(src.hex)
		if n == 0 || n > 4 || len(dst.hex) != n {
			return errs.Wrap(errs.ErrCorrupt, "FNT-CMAP-CODESPACE", "parse codespace", -1, 0, -1)
		}
		lo, hi := codeOf(src.hex), codeOf(dst.hex)
		if hi < lo {
			continue
		}
		cm.Spaces = append(cm.Spaces, codeSpace{Bytes: n, Lo: lo, Hi: hi})
		if n > cm.CodeLen {
			cm.CodeLen = n
		}
	}
}

func (cm *ToUnicode) parseBFChar(l *cmapLexer) error {
	for {
		t, ok, err := l.next()
		if err != nil || !ok {
			return errs.Wrap(errs.ErrCorrupt, "FNT-CMAP-BFCHAR", "parse bfchar", -1, 0, -1)
		}
		if t.kind == 'k' && t.kw == "endbfchar" {
			return nil
		}
		if t.kind != 'h' {
			continue
		}
		src := t
		dst, ok, err := l.next()
		if err != nil || !ok || dst.kind != 'h' {
			return errs.Wrap(errs.ErrCorrupt, "FNT-CMAP-BFCHAR", "parse bfchar", -1, 0, -1)
		}
		cm.Map[codeOf(src.hex)] = decodeUTF16BE(dst.hex)
	}
}

func (cm *ToUnicode) parseBFRange(l *cmapLexer) error {
	for {
		t, ok, err := l.next()
		if err != nil || !ok {
			return errs.Wrap(errs.ErrCorrupt, "FNT-CMAP-BFRANGE", "parse bfrange", -1, 0, -1)
		}
		if t.kind == 'k' && t.kw == "endbfrange" {
			return nil
		}
		if t.kind != 'h' {
			continue
		}
		hi, ok2, e2 := l.next()
		dst, ok3, e3 := l.next()
		if e2 != nil || e3 != nil || !ok2 || !ok3 || hi.kind != 'h' {
			return errs.Wrap(errs.ErrCorrupt, "FNT-CMAP-BFRANGE", "parse bfrange", -1, 0, -1)
		}
		start, end := codeOf(t.hex), codeOf(hi.hex)
		if end < start {
			continue
		}
		if dst.kind == 'a' {
			for i, d := range dst.arr {
				if d.kind != 'h' {
					continue
				}
				code := start + uint32(i)
				if code > end {
					break
				}
				cm.Map[code] = decodeUTF16BE(d.hex)
			}
			continue
		}
		if dst.kind != 'h' {
			continue
		}
		base := []rune(decodeUTF16BE(dst.hex))
		for code := start; code <= end; code++ {
			if uint64(code-start) > 1<<20 {
				break
			}
			r := append([]rune(nil), base...)
			if len(r) > 0 {
				r[len(r)-1] += rune(code - start)
			}
			cm.Map[code] = string(r)
		}
	}
}

func decodeUTF16BE(b []byte) string {
	if len(b)%2 != 0 {
		b = append(b, 0)
	}
	var sb strings.Builder
	for i := 0; i+1 < len(b); i += 2 {
		r := rune(b[i])<<8 | rune(b[i+1])
		if r >= 0xD800 && r <= 0xDBFF && i+3 < len(b) {
			r2 := rune(b[i+2])<<8 | rune(b[i+3])
			if r2 >= 0xDC00 && r2 <= 0xDFFF {
				r = 0x10000 + (r-0xD800)<<10 + (r2 - 0xDC00)
				i += 2
			}
		}
		sb.WriteRune(r)
	}
	return sb.String()
}
