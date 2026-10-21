package function

import (
	"math"
	"strconv"

	"github.com/F31/go-pdf/internal/errs"
)

const (
	psNumber = iota
	psBool
	psOp
	psProc
)

type psToken struct {
	kind uint8
	num  float64
	b    bool
	op   string
	proc []psToken
}

func parsePostScript(data []byte) ([]psToken, error) {
	p := &psParser{data: data}
	prog, err := p.proc(false)
	if err != nil {
		return nil, err
	}
	p.skip()
	if p.pos != len(p.data) {
		return nil, psErr("FUNC-PS-PARSE", "unexpected PostScript input")
	}
	if len(prog) == 1 && prog[0].kind == psProc {
		return prog[0].proc, nil
	}
	return prog, nil
}

type psParser struct {
	data []byte
	pos  int
}

func (p *psParser) skip() {
	for p.pos < len(p.data) {
		switch p.data[p.pos] {
		case ' ', '\t', '\r', '\n', '\f', 0:
			p.pos++
		case '%':
			for p.pos < len(p.data) && p.data[p.pos] != '\n' && p.data[p.pos] != '\r' {
				p.pos++
			}
		default:
			return
		}
	}
}

func (p *psParser) proc(nested bool) ([]psToken, error) {
	var out []psToken
	for {
		p.skip()
		if p.pos >= len(p.data) {
			if nested {
				return nil, psErr("FUNC-PS-PARSE", "unterminated PostScript procedure")
			}
			return out, nil
		}
		switch p.data[p.pos] {
		case '{':
			p.pos++
			sub, err := p.proc(true)
			if err != nil {
				return nil, err
			}
			out = append(out, psToken{kind: psProc, proc: sub})
			continue
		case '}':
			if !nested {
				return nil, psErr("FUNC-PS-PARSE", "unexpected closing brace")
			}
			p.pos++
			return out, nil
		}
		start := p.pos
		for p.pos < len(p.data) && !isPSDelim(p.data[p.pos]) {
			p.pos++
		}
		word := string(p.data[start:p.pos])
		if word == "" {
			return nil, psErr("FUNC-PS-PARSE", "invalid PostScript token")
		}
		switch word {
		case "true":
			out = append(out, psToken{kind: psBool, b: true})
		case "false":
			out = append(out, psToken{kind: psBool, b: false})
		default:
			if n, err := strconv.ParseFloat(word, 64); err == nil {
				out = append(out, psToken{kind: psNumber, num: n})
			} else {
				out = append(out, psToken{kind: psOp, op: word})
			}
		}
	}
}

func isPSDelim(c byte) bool {
	switch c {
	case ' ', '\t', '\r', '\n', '\f', 0, '{', '}', '%':
		return true
	}
	return false
}

type psMachine struct {
	stack []any
	steps int64
	depth int
}

func (m *psMachine) step() error {
	m.steps--
	if m.steps < 0 {
		return errs.Wrap(errs.ErrBudget, "BUD-FUNCTION-STEPS", "PostScript function step limit exceeded", -1, 0, -1)
	}
	return nil
}

func (m *psMachine) exec(prog []psToken) error {
	if m.depth > 64 {
		return psErr("FUNC-PS-DEPTH", "PostScript function nesting limit exceeded")
	}
	m.depth++
	defer func() { m.depth-- }()
	for _, t := range prog {
		if err := m.step(); err != nil {
			return err
		}
		switch t.kind {
		case psNumber:
			m.stack = append(m.stack, t.num)
		case psBool:
			m.stack = append(m.stack, t.b)
		case psProc:
			m.stack = append(m.stack, t.proc)
		case psOp:
			if err := m.op(t.op); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *psMachine) op(op string) error {
	switch op {
	case "dup":
		v, err := m.top()
		if err != nil {
			return err
		}
		m.stack = append(m.stack, v)
	case "pop":
		_, err := m.pop()
		return err
	case "exch":
		if len(m.stack) < 2 {
			return psErr("FUNC-PS-STACK", "PostScript stack underflow")
		}
		n := len(m.stack)
		m.stack[n-1], m.stack[n-2] = m.stack[n-2], m.stack[n-1]
	case "copy":
		n, err := m.popInt()
		if err != nil {
			return err
		}
		if n < 0 || n > len(m.stack) {
			return psErr("FUNC-PS-STACK", "PostScript stack underflow")
		}
		m.stack = append(m.stack, append([]any(nil), m.stack[len(m.stack)-n:]...)...)
	case "index":
		n, err := m.popInt()
		if err != nil {
			return err
		}
		if n < 0 || n >= len(m.stack) {
			return psErr("FUNC-PS-STACK", "PostScript stack underflow")
		}
		m.stack = append(m.stack, m.stack[len(m.stack)-1-n])
	case "roll":
		j, err := m.popInt()
		if err != nil {
			return err
		}
		n, err := m.popInt()
		if err != nil {
			return err
		}
		if n < 0 || n > len(m.stack) {
			return psErr("FUNC-PS-STACK", "PostScript stack underflow")
		}
		if n == 0 {
			return nil
		}
		j %= n
		if j < 0 {
			j += n
		}
		base := len(m.stack) - n
		part := append([]any(nil), m.stack[base:]...)
		part = append(part[len(part)-j:], part[:len(part)-j]...)
		copy(m.stack[base:], part)
	case "if":
		proc, err := m.popProc()
		if err != nil {
			return err
		}
		cond, err := m.popBool()
		if err != nil {
			return err
		}
		if cond {
			return m.exec(proc)
		}
	case "ifelse":
		proc2, err := m.popProc()
		if err != nil {
			return err
		}
		proc1, err := m.popProc()
		if err != nil {
			return err
		}
		cond, err := m.popBool()
		if err != nil {
			return err
		}
		if cond {
			return m.exec(proc1)
		}
		return m.exec(proc2)
	case "repeat":
		proc, err := m.popProc()
		if err != nil {
			return err
		}
		n, err := m.popInt()
		if err != nil {
			return err
		}
		if n < 0 {
			return psErr("FUNC-PS-RANGE", "negative repeat count")
		}
		for i := 0; i < n; i++ {
			if err := m.exec(proc); err != nil {
				return err
			}
		}
	case "exec":
		proc, err := m.popProc()
		if err != nil {
			return err
		}
		return m.exec(proc)
	case "abs":
		return m.unary(math.Abs)
	case "neg":
		return m.unary(func(v float64) float64 { return -v })
	case "ceiling":
		return m.unary(math.Ceil)
	case "floor":
		return m.unary(math.Floor)
	case "round":
		return m.unary(math.Round)
	case "truncate":
		return m.unary(math.Trunc)
	case "sqrt":
		return m.unary(math.Sqrt)
	case "sin":
		return m.unary(func(v float64) float64 { return math.Sin(v * math.Pi / 180) })
	case "cos":
		return m.unary(func(v float64) float64 { return math.Cos(v * math.Pi / 180) })
	case "exp":
		return m.binary(func(a, b float64) float64 { return math.Pow(a, b) })
	case "ln":
		return m.unary(math.Log)
	case "log":
		return m.unary(math.Log10)
	case "cvi":
		return m.unary(math.Trunc)
	case "cvr":
		return m.unary(func(v float64) float64 { return v })
	case "add":
		return m.binary(func(a, b float64) float64 { return a + b })
	case "sub":
		return m.binary(func(a, b float64) float64 { return a - b })
	case "mul":
		return m.binary(func(a, b float64) float64 { return a * b })
	case "div":
		return m.binary(func(a, b float64) float64 { return a / b })
	case "idiv":
		return m.binary(func(a, b float64) float64 { return math.Trunc(a / b) })
	case "mod":
		return m.binary(func(a, b float64) float64 { return math.Mod(a, b) })
	case "atan":
		return m.binary(func(a, b float64) float64 { return math.Atan2(a, b) * 180 / math.Pi })
	case "bitshift":
		return m.binary(func(a, b float64) float64 {
			n := int(b)
			if n >= 0 {
				return float64(int64(a) << uint(n))
			}
			return float64(int64(a) >> uint(-n))
		})
	case "eq":
		return m.compare(func(c int) bool { return c == 0 })
	case "ne":
		return m.compare(func(c int) bool { return c != 0 })
	case "gt":
		return m.compare(func(c int) bool { return c > 0 })
	case "ge":
		return m.compare(func(c int) bool { return c >= 0 })
	case "lt":
		return m.compare(func(c int) bool { return c < 0 })
	case "le":
		return m.compare(func(c int) bool { return c <= 0 })
	case "and":
		return m.logic(func(a, b bool) bool { return a && b }, func(a, b int64) int64 { return a & b })
	case "or":
		return m.logic(func(a, b bool) bool { return a || b }, func(a, b int64) int64 { return a | b })
	case "xor":
		return m.logic(func(a, b bool) bool { return a != b }, func(a, b int64) int64 { return a ^ b })
	case "not":
		if len(m.stack) == 0 {
			return psErr("FUNC-PS-STACK", "PostScript stack underflow")
		}
		switch v := m.stack[len(m.stack)-1].(type) {
		case bool:
			m.stack[len(m.stack)-1] = !v
		case float64:
			m.stack[len(m.stack)-1] = float64(^int64(v))
		default:
			return psErr("FUNC-PS-TYPE", "PostScript type error")
		}
	default:
		return psErr("FUNC-PS-OP", "unsupported PostScript operator")
	}
	return nil
}

func (m *psMachine) pop() (any, error) {
	if len(m.stack) == 0 {
		return nil, psErr("FUNC-PS-STACK", "PostScript stack underflow")
	}
	v := m.stack[len(m.stack)-1]
	m.stack = m.stack[:len(m.stack)-1]
	return v, nil
}

func (m *psMachine) top() (any, error) {
	if len(m.stack) == 0 {
		return nil, psErr("FUNC-PS-STACK", "PostScript stack underflow")
	}
	return m.stack[len(m.stack)-1], nil
}

func (m *psMachine) popNum() (float64, error) {
	v, err := m.pop()
	if err != nil {
		return 0, err
	}
	n, ok := v.(float64)
	if !ok {
		return 0, psErr("FUNC-PS-TYPE", "PostScript number expected")
	}
	return n, nil
}

func (m *psMachine) popInt() (int, error) {
	n, err := m.popNum()
	if err != nil {
		return 0, err
	}
	return int(math.Trunc(n)), nil
}

func (m *psMachine) popBool() (bool, error) {
	v, err := m.pop()
	if err != nil {
		return false, err
	}
	b, ok := v.(bool)
	if !ok {
		return false, psErr("FUNC-PS-TYPE", "PostScript boolean expected")
	}
	return b, nil
}

func (m *psMachine) popProc() ([]psToken, error) {
	v, err := m.pop()
	if err != nil {
		return nil, err
	}
	p, ok := v.([]psToken)
	if !ok {
		return nil, psErr("FUNC-PS-TYPE", "PostScript procedure expected")
	}
	return p, nil
}

func (m *psMachine) unary(fn func(float64) float64) error {
	v, err := m.popNum()
	if err != nil {
		return err
	}
	m.stack = append(m.stack, fn(v))
	return nil
}

func (m *psMachine) binary(fn func(float64, float64) float64) error {
	b, err := m.popNum()
	if err != nil {
		return err
	}
	a, err := m.popNum()
	if err != nil {
		return err
	}
	m.stack = append(m.stack, fn(a, b))
	return nil
}

func (m *psMachine) compare(fn func(int) bool) error {
	b, err := m.popNum()
	if err != nil {
		return err
	}
	a, err := m.popNum()
	if err != nil {
		return err
	}
	c := 0
	if a < b {
		c = -1
	} else if a > b {
		c = 1
	}
	m.stack = append(m.stack, fn(c))
	return nil
}

func (m *psMachine) logic(bf func(bool, bool) bool, nf func(int64, int64) int64) error {
	bv, err := m.pop()
	if err != nil {
		return err
	}
	av, err := m.pop()
	if err != nil {
		return err
	}
	if ab, ok := av.(bool); ok {
		bb, ok := bv.(bool)
		if !ok {
			return psErr("FUNC-PS-TYPE", "PostScript type mismatch")
		}
		m.stack = append(m.stack, bf(ab, bb))
		return nil
	}
	an, ok := av.(float64)
	if !ok {
		return psErr("FUNC-PS-TYPE", "PostScript type error")
	}
	bn, ok := bv.(float64)
	if !ok {
		return psErr("FUNC-PS-TYPE", "PostScript type mismatch")
	}
	m.stack = append(m.stack, float64(nf(int64(an), int64(bn))))
	return nil
}

func psErr(code, msg string) error {
	return errs.Wrap(errs.ErrCorrupt, code, msg, -1, 0, -1)
}
