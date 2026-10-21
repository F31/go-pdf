package content

import (
	"bytes"
	"strconv"

	"github.com/F31/go-pdf/internal/errs"
	"github.com/F31/go-pdf/internal/syntax"
)

type tokenKind int

const (
	tokObject tokenKind = iota
	tokOperator
)

type token struct {
	kind tokenKind
	obj  syntax.Object
	op   string
	pos  int
}

type lexer struct {
	data     []byte
	pos      int
	maxDepth int
	depth    int
}

func newLexer(data []byte, maxDepth int) *lexer {
	if maxDepth <= 0 {
		maxDepth = 64
	}
	return &lexer{data: data, maxDepth: maxDepth}
}

func isSpace(c byte) bool {
	return c == 0 || c == '\t' || c == '\n' || c == '\f' || c == '\r' || c == ' '
}

func isDelim(c byte) bool {
	switch c {
	case '(', ')', '<', '>', '[', ']', '{', '}', '/', '%':
		return true
	}
	return false
}

func (l *lexer) skipSpace() {
	for l.pos < len(l.data) {
		c := l.data[l.pos]
		if isSpace(c) {
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

func (l *lexer) next() (token, error) {
	l.skipSpace()
	if l.pos >= len(l.data) {
		return token{}, errEOF
	}
	start := l.pos
	c := l.data[l.pos]
	switch {
	case c == '/':
		return l.name(start)
	case c == '(':
		return l.literalString(start)
	case c == '<':
		if l.pos+1 < len(l.data) && l.data[l.pos+1] == '<' {
			return l.dict(start)
		}
		return l.hexString(start)
	case c == '[':
		return l.array(start)
	case c == '+' || c == '-' || c == '.' || (c >= '0' && c <= '9'):
		return l.number(start)
	case (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '\'' || c == '"':
		word := l.word(start)
		switch word {
		case "true":
			return token{kind: tokObject, obj: syntax.Bool(true), pos: start}, nil
		case "false":
			return token{kind: tokObject, obj: syntax.Bool(false), pos: start}, nil
		case "null":
			return token{kind: tokObject, obj: syntax.Null{}, pos: start}, nil
		}
		return token{kind: tokOperator, op: word, pos: start}, nil
	}
	return token{}, errAt("CNT-TOKEN", start)
}

var errEOF = errs.Wrap(errs.ErrCorrupt, "CNT-EOF", "unexpected end of content stream", -1, 0, -1)

func errAt(code string, pos int) error {
	return errs.Wrap(errs.ErrCorrupt, code, "parse content stream", int64(pos), 0, -1)
}

func (l *lexer) word(start int) string {
	for l.pos < len(l.data) && !isSpace(l.data[l.pos]) && !isDelim(l.data[l.pos]) {
		l.pos++
	}
	return string(l.data[start:l.pos])
}

func (l *lexer) number(start int) (token, error) {
	if l.data[l.pos] == '+' || l.data[l.pos] == '-' {
		l.pos++
	}
	digits, dots := 0, 0
	for l.pos < len(l.data) {
		c := l.data[l.pos]
		if c >= '0' && c <= '9' {
			digits++
			l.pos++
			continue
		}
		if c == '.' {
			dots++
			l.pos++
			continue
		}
		break
	}
	if digits == 0 || dots > 1 {
		return token{}, errAt("CNT-NUMBER", start)
	}
	v, err := strconv.ParseFloat(string(l.data[start:l.pos]), 64)
	if err != nil {
		return token{}, errAt("CNT-NUMBER", start)
	}
	return token{kind: tokObject, obj: syntax.Real(v), pos: start}, nil
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

func (l *lexer) name(start int) (token, error) {
	l.pos++
	var out []byte
	for l.pos < len(l.data) {
		c := l.data[l.pos]
		if isSpace(c) || isDelim(c) {
			break
		}
		if c == '#' && l.pos+2 < len(l.data) {
			hi, ok1 := hexVal(l.data[l.pos+1])
			lo, ok2 := hexVal(l.data[l.pos+2])
			if ok1 && ok2 {
				out = append(out, byte(hi<<4|lo))
				l.pos += 3
				continue
			}
		}
		out = append(out, c)
		l.pos++
	}
	if out == nil {
		out = append(out, l.data[start+1:l.pos]...)
	}
	return token{kind: tokObject, obj: syntax.Name(out), pos: start}, nil
}

func (l *lexer) literalString(start int) (token, error) {
	l.pos++
	depth := 1
	var out []byte
	for l.pos < len(l.data) {
		c := l.data[l.pos]
		l.pos++
		switch c {
		case '(':
			depth++
			out = append(out, c)
		case ')':
			depth--
			if depth == 0 {
				return token{kind: tokObject, obj: syntax.String(out), pos: start}, nil
			}
			out = append(out, c)
		case '\\':
			if l.pos >= len(l.data) {
				return token{}, errAt("CNT-STRING", start)
			}
			e := l.data[l.pos]
			l.pos++
			switch e {
			case 'n':
				out = append(out, '\n')
			case 'r':
				out = append(out, '\r')
			case 't':
				out = append(out, '\t')
			case 'b':
				out = append(out, '\b')
			case 'f':
				out = append(out, '\f')
			case '(', ')', '\\':
				out = append(out, e)
			case '\r':
				if l.pos < len(l.data) && l.data[l.pos] == '\n' {
					l.pos++
				}
			case '\n':
			default:
				if e >= '0' && e <= '7' {
					v := int(e - '0')
					for i := 0; i < 2 && l.pos < len(l.data); i++ {
						d := l.data[l.pos]
						if d < '0' || d > '7' {
							break
						}
						v = v*8 + int(d-'0')
						l.pos++
					}
					out = append(out, byte(v))
				} else {
					out = append(out, e)
				}
			}
		case '\r':
			if l.pos < len(l.data) && l.data[l.pos] == '\n' {
				l.pos++
			}
			out = append(out, '\n')
		default:
			out = append(out, c)
		}
	}
	return token{}, errAt("CNT-STRING", start)
}

func (l *lexer) hexString(start int) (token, error) {
	l.pos++
	var out []byte
	high, have := 0, false
	for l.pos < len(l.data) {
		c := l.data[l.pos]
		l.pos++
		if c == '>' {
			if have {
				out = append(out, byte(high<<4))
			}
			return token{kind: tokObject, obj: syntax.String(out), pos: start}, nil
		}
		if isSpace(c) {
			continue
		}
		v, ok := hexVal(c)
		if !ok {
			return token{}, errAt("CNT-HEX", start)
		}
		if !have {
			high, have = v, true
		} else {
			out = append(out, byte(high<<4|v))
			have = false
		}
	}
	return token{}, errAt("CNT-HEX", start)
}

func (l *lexer) array(start int) (token, error) {
	l.depth++
	if l.depth > l.maxDepth {
		l.depth--
		return token{}, errs.Wrap(errs.ErrBudget, "BUD-NESTING-DEPTH", "parse content array", int64(start), 0, -1)
	}
	defer func() { l.depth-- }()
	l.pos++
	arr := syntax.Array{}
	for {
		l.skipSpace()
		if l.pos >= len(l.data) {
			return token{}, errAt("CNT-ARRAY", start)
		}
		if l.data[l.pos] == ']' {
			l.pos++
			return token{kind: tokObject, obj: arr, pos: start}, nil
		}
		t, err := l.next()
		if err != nil {
			return token{}, err
		}
		if t.kind != tokObject {
			return token{}, errAt("CNT-ARRAY-OP", t.pos)
		}
		arr = append(arr, t.obj)
	}
}

func (l *lexer) dict(start int) (token, error) {
	l.depth++
	if l.depth > l.maxDepth {
		l.depth--
		return token{}, errs.Wrap(errs.ErrBudget, "BUD-NESTING-DEPTH", "parse content dict", int64(start), 0, -1)
	}
	defer func() { l.depth-- }()
	l.pos += 2
	d := syntax.Dict{}
	for {
		l.skipSpace()
		if l.pos >= len(l.data) {
			return token{}, errAt("CNT-DICT", start)
		}
		if l.pos+1 < len(l.data) && l.data[l.pos] == '>' && l.data[l.pos+1] == '>' {
			l.pos += 2
			return token{kind: tokObject, obj: d, pos: start}, nil
		}
		kt, err := l.next()
		if err != nil {
			return token{}, err
		}
		key, ok := kt.obj.(syntax.Name)
		if kt.kind != tokObject || !ok {
			return token{}, errAt("CNT-DICT-KEY", kt.pos)
		}
		vt, err := l.next()
		if err != nil {
			return token{}, err
		}
		if vt.kind != tokObject {
			return token{}, errAt("CNT-DICT-VALUE", vt.pos)
		}
		d[key] = vt.obj
	}
}

// skipInlineImage is called after the BI operator. It consumes the inline
// dictionary and binary payload through EI.
func (l *lexer) skipInlineImage() {
	idx := bytes.Index(l.data[l.pos:], []byte("ID"))
	for idx >= 0 {
		at := l.pos + idx
		after := at + 2
		beforeOK := at == 0 || isSpace(l.data[at-1])
		afterOK := after < len(l.data) && isSpace(l.data[after])
		if beforeOK && afterOK {
			l.pos = after + 1
			break
		}
		next := bytes.Index(l.data[at+2:], []byte("ID"))
		if next < 0 {
			idx = -1
			break
		}
		idx = at + 2 + next - l.pos
	}
	for {
		idx = bytes.Index(l.data[l.pos:], []byte("EI"))
		if idx < 0 {
			l.pos = len(l.data)
			return
		}
		at := l.pos + idx
		beforeOK := at > 0 && isSpace(l.data[at-1])
		end := at + 2
		afterOK := end == len(l.data) || isSpace(l.data[end]) || isDelim(l.data[end])
		if beforeOK && afterOK {
			l.pos = end
			return
		}
		l.pos = at + 2
	}
}
