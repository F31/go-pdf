package syntax

import (
	"bytes"
	"fmt"
	"strconv"

	"github.com/F31/go-pdf/internal/errs"
)

type parser struct {
	data     []byte
	pos      int
	depth    int
	lenient  bool
	maxDepth int
}

func newParser(data []byte, pos int, lenient bool, maxDepth int) *parser {
	if maxDepth <= 0 {
		maxDepth = 64
	}
	return &parser{data: data, pos: pos, lenient: lenient, maxDepth: maxDepth}
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

func (p *parser) eof() bool { return p.pos >= len(p.data) }

func (p *parser) skipSpace() {
	for p.pos < len(p.data) {
		c := p.data[p.pos]
		if isSpace(c) {
			p.pos++
			continue
		}
		if c == '%' {
			for p.pos < len(p.data) && p.data[p.pos] != '\n' && p.data[p.pos] != '\r' {
				p.pos++
			}
			continue
		}
		break
	}
}

func (p *parser) hasKeyword(kw string) bool {
	if p.pos+len(kw) > len(p.data) {
		return false
	}
	if !bytes.Equal(p.data[p.pos:p.pos+len(kw)], []byte(kw)) {
		return false
	}
	end := p.pos + len(kw)
	return end == len(p.data) || isSpace(p.data[end]) || isDelim(p.data[end])
}

func (p *parser) readKeyword() string {
	start := p.pos
	for p.pos < len(p.data) && !isSpace(p.data[p.pos]) && !isDelim(p.data[p.pos]) {
		p.pos++
	}
	return string(p.data[start:p.pos])
}

func (p *parser) parseObject() (Object, error) {
	p.skipSpace()
	if p.eof() {
		return nil, p.err("SYN-EOF", "unexpected end of file", p.pos)
	}
	c := p.data[p.pos]
	switch {
	case c == '/':
		return p.parseName()
	case c == '(':
		return p.parseLiteralString()
	case c == '<':
		if p.pos+1 < len(p.data) && p.data[p.pos+1] == '<' {
			return p.parseDict()
		}
		return p.parseHexString()
	case c == '[':
		return p.parseArray()
	case c == '+' || c == '-' || c == '.' || (c >= '0' && c <= '9'):
		return p.parseNumberOrRef()
	case isAlpha(c):
		kw := p.readKeyword()
		switch kw {
		case "true":
			return Bool(true), nil
		case "false":
			return Bool(false), nil
		case "null":
			return Null{}, nil
		}
		return nil, p.err("SYN-UNEXPECTED-TOKEN", "unexpected keyword", p.pos-len(kw))
	}
	return nil, p.err("SYN-UNEXPECTED-TOKEN", "unexpected byte", p.pos)
}

func isAlpha(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func (p *parser) err(code, msg string, offset int) error {
	return errs.Wrap(errs.ErrCorrupt, code, msg, int64(offset), 0, -1)
}

func (p *parser) parseName() (Object, error) {
	p.pos++
	start := p.pos
	var out []byte
	for p.pos < len(p.data) {
		c := p.data[p.pos]
		if isSpace(c) || isDelim(c) {
			break
		}
		if c == '#' {
			if p.pos+2 < len(p.data) {
				hi, ok1 := hexVal(p.data[p.pos+1])
				lo, ok2 := hexVal(p.data[p.pos+2])
				if ok1 && ok2 {
					out = append(out, byte(hi<<4|lo))
					p.pos += 3
					continue
				}
			}
			if !p.lenient {
				return nil, p.err("SYN-NAME-ESCAPE", "invalid name escape", p.pos)
			}
		}
		out = append(out, c)
		p.pos++
	}
	if out == nil {
		out = append(out, p.data[start:p.pos]...)
	}
	return Name(out), nil
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

func (p *parser) parseLiteralString() (Object, error) {
	p.pos++
	depth := 1
	var out []byte
	for p.pos < len(p.data) {
		c := p.data[p.pos]
		p.pos++
		switch c {
		case '(':
			depth++
			out = append(out, c)
		case ')':
			depth--
			if depth == 0 {
				return String(out), nil
			}
			out = append(out, c)
		case '\\':
			if p.eof() {
				if p.lenient {
					return String(out), nil
				}
				return nil, p.err("SYN-STRING-ESCAPE", "truncated escape", p.pos)
			}
			e := p.data[p.pos]
			p.pos++
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
				if p.pos < len(p.data) && p.data[p.pos] == '\n' {
					p.pos++
				}
			case '\n':
			default:
				if e >= '0' && e <= '7' {
					v := int(e - '0')
					for i := 0; i < 2 && p.pos < len(p.data); i++ {
						d := p.data[p.pos]
						if d < '0' || d > '7' {
							break
						}
						v = v*8 + int(d-'0')
						p.pos++
					}
					out = append(out, byte(v))
				} else {
					if !p.lenient {
						return nil, p.err("SYN-STRING-ESCAPE", "invalid escape", p.pos-1)
					}
					out = append(out, e)
				}
			}
		case '\r':
			if p.pos < len(p.data) && p.data[p.pos] == '\n' {
				p.pos++
			}
			out = append(out, '\n')
		default:
			out = append(out, c)
		}
	}
	if p.lenient {
		return String(out), nil
	}
	return nil, p.err("SYN-STRING-EOF", "unterminated string", p.pos)
}

func (p *parser) parseHexString() (Object, error) {
	p.pos++
	var out []byte
	var high int
	have := false
	for p.pos < len(p.data) {
		c := p.data[p.pos]
		p.pos++
		if c == '>' {
			if have {
				out = append(out, byte(high<<4))
			}
			return String(out), nil
		}
		if isSpace(c) {
			continue
		}
		v, ok := hexVal(c)
		if !ok {
			if p.lenient {
				continue
			}
			return nil, p.err("SYN-HEX-STRING", "invalid hex digit", p.pos-1)
		}
		if !have {
			high = v
			have = true
		} else {
			out = append(out, byte(high<<4|v))
			have = false
		}
	}
	if p.lenient {
		if have {
			out = append(out, byte(high<<4))
		}
		return String(out), nil
	}
	return nil, p.err("SYN-HEX-EOF", "unterminated hex string", p.pos)
}

func (p *parser) parseArray() (Object, error) {
	p.depth++
	if p.depth > p.maxDepth {
		if p.lenient {
			p.depth--
			for p.pos < len(p.data) && p.data[p.pos] != ']' {
				p.pos++
			}
			if p.pos < len(p.data) {
				p.pos++
			}
			return Array{}, nil
		}
		p.depth--
		return nil, errs.Wrap(errs.ErrBudget, "BUD-NESTING-DEPTH", "parse array", int64(p.pos), 0, -1)
	}
	defer func() { p.depth-- }()
	p.pos++
	arr := Array{}
	for {
		p.skipSpace()
		if p.eof() {
			if p.lenient {
				return arr, nil
			}
			return nil, p.err("SYN-ARRAY-EOF", "unterminated array", p.pos)
		}
		if p.data[p.pos] == ']' {
			p.pos++
			return arr, nil
		}
		obj, err := p.parseObject()
		if err != nil {
			return nil, err
		}
		arr = append(arr, obj)
	}
}

func (p *parser) parseDict() (Object, error) {
	p.depth++
	if p.depth > p.maxDepth {
		if p.lenient {
			p.depth--
			idx := bytes.Index(p.data[p.pos:], []byte(">>"))
			if idx >= 0 {
				p.pos += idx + 2
			} else {
				p.pos = len(p.data)
			}
			return Dict{}, nil
		}
		p.depth--
		return nil, errs.Wrap(errs.ErrBudget, "BUD-NESTING-DEPTH", "parse dict", int64(p.pos), 0, -1)
	}
	defer func() { p.depth-- }()
	p.pos += 2
	d := Dict{}
	for {
		p.skipSpace()
		if p.eof() {
			if p.lenient {
				return d, nil
			}
			return nil, p.err("SYN-DICT-EOF", "unterminated dictionary", p.pos)
		}
		if p.pos+1 < len(p.data) && p.data[p.pos] == '>' && p.data[p.pos+1] == '>' {
			p.pos += 2
			return d, nil
		}
		keyObj, err := p.parseObject()
		if err != nil {
			return nil, err
		}
		key, ok := keyObj.(Name)
		if !ok {
			if !p.lenient {
				return nil, p.err("SYN-DICT-KEY", "dictionary key is not a name", p.pos)
			}
			continue
		}
		p.skipSpace()
		if p.eof() || (p.pos+1 < len(p.data) && p.data[p.pos] == '>' && p.data[p.pos+1] == '>') {
			if p.lenient {
				return d, nil
			}
			return nil, p.err("SYN-DICT-VALUE", "dictionary key without value", p.pos)
		}
		val, err := p.parseObject()
		if err != nil {
			return nil, err
		}
		d[key] = val
	}
}

func (p *parser) parseNumberOrRef() (Object, error) {
	first, _, err := p.parseNumber()
	if err != nil {
		return nil, err
	}
	iv, isInt := first.(Int)
	if !isInt {
		return first, nil
	}
	save := p.pos
	p.skipSpace()
	if p.eof() || !(p.data[p.pos] == '+' || p.data[p.pos] == '-' || (p.data[p.pos] >= '0' && p.data[p.pos] <= '9')) {
		p.pos = save
		return first, nil
	}
	second, _, err := p.parseNumber()
	if err != nil {
		p.pos = save
		return first, nil
	}
	gv, ok := second.(Int)
	if !ok {
		p.pos = save
		return first, nil
	}
	p.skipSpace()
	if p.pos < len(p.data) && p.data[p.pos] == 'R' && (p.pos+1 == len(p.data) || isSpace(p.data[p.pos+1]) || isDelim(p.data[p.pos+1])) {
		p.pos++
		return Ref{Num: int(iv), Gen: int(gv)}, nil
	}
	p.pos = save
	return first, nil
}

func (p *parser) parseNumber() (Object, int, error) {
	start := p.pos
	if p.pos < len(p.data) && (p.data[p.pos] == '+' || p.data[p.pos] == '-') {
		p.pos++
	}
	digits := 0
	dots := 0
	for p.pos < len(p.data) {
		c := p.data[p.pos]
		if c >= '0' && c <= '9' {
			digits++
			p.pos++
			continue
		}
		if c == '.' {
			dots++
			p.pos++
			continue
		}
		break
	}
	tok := p.data[start:p.pos]
	if digits == 0 || dots > 1 || p.pos == start || (p.pos == start+1 && (tok[0] == '+' || tok[0] == '-' || tok[0] == '.')) {
		if p.lenient {
			// Consume the longest malformed run and try a prefix.
			for p.pos < len(p.data) && !isSpace(p.data[p.pos]) && !isDelim(p.data[p.pos]) {
				p.pos++
			}
			tok = p.data[start:p.pos]
			return p.lenientNumber(tok, start)
		}
		return nil, start, p.err("SYN-NUMBER", "malformed number", start)
	}
	if dots == 1 {
		f, err := strconv.ParseFloat(string(tok), 64)
		if err != nil {
			if !p.lenient {
				return nil, start, p.err("SYN-NUMBER", "malformed real", start)
			}
			return Real(0), start, nil
		}
		return Real(f), start, nil
	}
	n, err := strconv.ParseInt(string(tok), 10, 64)
	if err != nil {
		if !p.lenient {
			return nil, start, p.err("SYN-NUMBER", "malformed integer", start)
		}
		return Real(0), start, nil
	}
	return Int(n), start, nil
}

func (p *parser) lenientNumber(tok []byte, start int) (Object, int, error) {
	best := 0
	for i := 0; i <= len(tok); i++ {
		if _, err := strconv.ParseInt(string(tok[:i]), 10, 64); err == nil {
			best = i
		}
		if _, err := strconv.ParseFloat(string(tok[:i]), 64); err == nil && i > best {
			best = i
		}
	}
	if best == 0 {
		return Int(0), start, nil
	}
	if n, err := strconv.ParseInt(string(tok[:best]), 10, 64); err == nil {
		return Int(n), start, nil
	}
	f, _ := strconv.ParseFloat(string(tok[:best]), 64)
	return Real(f), start, nil
}

func (p *parser) expectKeyword(kw string) error {
	p.skipSpace()
	if !p.hasKeyword(kw) {
		return p.err("SYN-KEYWORD", fmt.Sprintf("expected %s", kw), p.pos)
	}
	p.pos += len(kw)
	return nil
}
