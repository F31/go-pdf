package content

import (
	"context"
	"errors"
	"math"

	"github.com/F31/go-pdf/displaylist"
	"github.com/F31/go-pdf/internal/budget"
	"github.com/F31/go-pdf/internal/diag"
	"github.com/F31/go-pdf/internal/errs"
	"github.com/F31/go-pdf/internal/font"
	"github.com/F31/go-pdf/internal/syntax"
)

type PageData struct {
	Index     int
	Content   []byte
	Resources syntax.Dict
	Matrix    [6]float64
	MediaBox  displaylist.Rect
	CropBox   displaylist.Rect
	File      *syntax.File
	Tracker   *budget.Tracker
	Diag      *diag.Collector
}

type Result struct {
	Clips []displaylist.Clip
	Items int
	Ops   int
}

type matrix [6]float64

func (m matrix) apply(x, y float64) (float64, float64) {
	return m[0]*x + m[2]*y + m[4], m[1]*x + m[3]*y + m[5]
}

func (m matrix) scale() float64 {
	d := math.Abs(m[0]*m[3] - m[1]*m[2])
	if d <= 0 {
		return 1
	}
	return math.Sqrt(d)
}

func mmul(a, b matrix) matrix {
	return matrix{
		a[0]*b[0] + a[2]*b[1],
		a[1]*b[0] + a[3]*b[1],
		a[0]*b[2] + a[2]*b[3],
		a[1]*b[2] + a[3]*b[3],
		a[0]*b[4] + a[2]*b[5] + a[4],
		a[1]*b[4] + a[3]*b[5] + a[5],
	}
}

func translate(x, y float64) matrix { return matrix{1, 0, 0, 1, x, y} }

type state struct {
	ctm         matrix
	lineWidth   float64
	cap         int
	join        int
	miter       float64
	dash        []float64
	dashPhase   float64
	fill        displaylist.Paint
	stroke      displaylist.Paint
	fillSpace   string
	strokeSpace string

	font      *font.Font
	fontSize  float64
	charSpace float64
	wordSpace float64
	hscale    float64
	leading   float64
	rise      float64
	render    int
}

func (s state) clone() state {
	s.dash = append([]float64(nil), s.dash...)
	return s
}

type interp struct {
	ctx   context.Context
	pd    PageData
	sink  func(displaylist.Item) error
	lx    *lexer
	fonts map[string]*font.Font

	stack  []syntax.Object
	st     state
	gstack []state

	tm, tlm matrix
	path    []displaylist.Segment
	current displaylist.Point
	haveCur bool
	startPt displaylist.Point

	pendingClip *displaylist.Clip
	curClip     int
	clips       []displaylist.Clip

	z, id       int
	tr          *displaylist.TextRun
	trClip      int
	trBounds    displaylist.Rect
	trHasBounds bool

	ops      int
	unknown  map[string]bool
	suppress bool
	result   Result
}

func Interpret(ctx context.Context, pd PageData, emit func(displaylist.Item) error) (*Result, error) {
	maxDepth := 64
	if pd.Tracker != nil {
		maxDepth = pd.Tracker.Budget().MaxNestingDepth
	}
	ip := &interp{
		ctx: ctx, pd: pd, sink: emit, lx: newLexer(pd.Content, maxDepth),
		curClip: -1, trClip: -1, unknown: map[string]bool{}, clips: make([]displaylist.Clip, 0),
	}
	ip.st = state{
		ctm: matrix(pd.Matrix), lineWidth: 1, cap: 0, join: 0, miter: 10, hscale: 1,
		fill:   displaylist.Paint{Kind: displaylist.PaintSolid, RGBA: [4]float64{0, 0, 0, 1}},
		stroke: displaylist.Paint{Kind: displaylist.PaintSolid, RGBA: [4]float64{0, 0, 0, 1}},
	}
	ip.tm, ip.tlm = matrix{1, 0, 0, 1, 0, 0}, matrix{1, 0, 0, 1, 0, 0}
	if err := ip.run(); err != nil {
		return nil, err
	}
	ip.result.Clips = ip.clips
	ip.result.Items = ip.id
	ip.result.Ops = ip.ops
	return &ip.result, nil
}

func (ip *interp) run() error {
	for {
		if err := ip.ctx.Err(); err != nil {
			return errs.Wrap(errs.ErrCanceled, "CNT-CANCELED", "interpret page", -1, 0, ip.pd.Index)
		}
		tok, err := ip.lx.next()
		if err == errEOF {
			break
		}
		if err != nil {
			if errors.Is(err, errs.ErrBudget) || errors.Is(err, errs.ErrCanceled) {
				return err
			}
			ip.diag(diag.Error, "CNT-TOKEN", err.Error())
			break
		}
		if tok.kind == tokObject {
			if len(ip.stack) > 1024 {
				ip.diag(diag.Warn, "CNT-OPERANDS", "operand stack overflow")
				ip.stack = ip.stack[:0]
			}
			ip.stack = append(ip.stack, tok.obj)
			continue
		}
		ip.ops++
		if ip.pd.Tracker != nil && int64(ip.ops) > ip.pd.Tracker.Budget().MaxOperators {
			return errs.Wrap(errs.ErrBudget, "BUD-OPERATORS", "interpret page", -1, 0, ip.pd.Index)
		}
		if err := ip.dispatch(tok); err != nil {
			return err
		}
		if ip.pd.Tracker != nil && int64(len(ip.path)) > ip.pd.Tracker.Budget().MaxPathSegments {
			return errs.Wrap(errs.ErrBudget, "BUD-PATH-SEGMENTS", "interpret page", -1, 0, ip.pd.Index)
		}
	}
	if err := ip.flushText(); err != nil {
		return err
	}
	if len(ip.gstack) > 0 {
		ip.diag(diag.Warn, "CNT-GSTACK", "unbalanced q/Q at end of page")
	}
	return nil
}

func (ip *interp) diag(level diag.Level, code, msg string) {
	if ip.pd.Diag != nil {
		ip.pd.Diag.Add(diag.Diagnostic{Level: level, Code: code, Msg: msg, Page: ip.pd.Index, Obj: 0, Offset: -1})
	}
}

func (ip *interp) diagOnce(level diag.Level, code, msg string) {
	if ip.unknown[code] {
		return
	}
	ip.unknown[code] = true
	ip.diag(level, code, msg)
}

func (ip *interp) take(n int) ([]syntax.Object, bool) {
	if len(ip.stack) < n {
		ip.diag(diag.Warn, "CNT-OPERANDS", "not enough operands")
		ip.stack = ip.stack[:0]
		return nil, false
	}
	out := append([]syntax.Object(nil), ip.stack[len(ip.stack)-n:]...)
	ip.stack = ip.stack[:len(ip.stack)-n]
	return out, true
}

func (ip *interp) all() []syntax.Object {
	out := ip.stack
	ip.stack = nil
	return out
}

func num(o syntax.Object) (float64, bool) { return syntax.AsNumber(o) }

func name(o syntax.Object) (string, bool) { return syntax.AsName(o) }

func (ip *interp) dispatch(t token) error {
	switch t.op {
	case "q":
		ip.stack = nil
		ip.gstack = append(ip.gstack, ip.st.clone())
	case "Q":
		ip.stack = nil
		if len(ip.gstack) == 0 {
			ip.diag(diag.Warn, "CNT-GSTACK", "unbalanced Q")
			return nil
		}
		if err := ip.flushText(); err != nil {
			return err
		}
		ip.st = ip.gstack[len(ip.gstack)-1]
		ip.gstack = ip.gstack[:len(ip.gstack)-1]
	case "cm":
		ops, ok := ip.take(6)
		if !ok {
			return nil
		}
		if err := ip.flushText(); err != nil {
			return err
		}
		var m matrix
		for i := range m {
			v, ok := num(ops[i])
			if !ok {
				ip.diag(diag.Warn, "CNT-NUMBER", "invalid cm operand")
				return nil
			}
			m[i] = v
		}
		ip.st.ctm = mmul(m, ip.st.ctm)
	case "w":
		if ops, ok := ip.take(1); ok {
			if v, ok := num(ops[0]); ok {
				ip.st.lineWidth = v
			}
		}
	case "J":
		if ops, ok := ip.take(1); ok {
			if v, ok := num(ops[0]); ok {
				ip.st.cap = int(v)
			}
		}
	case "j":
		if ops, ok := ip.take(1); ok {
			if v, ok := num(ops[0]); ok {
				ip.st.join = int(v)
			}
		}
	case "M":
		if ops, ok := ip.take(1); ok {
			if v, ok := num(ops[0]); ok {
				ip.st.miter = v
			}
		}
	case "d":
		ops, ok := ip.take(2)
		if !ok {
			return nil
		}
		arr, ok := ops[0].(syntax.Array)
		if !ok {
			return nil
		}
		dash := make([]float64, 0, len(arr))
		for _, o := range arr {
			if v, ok := num(o); ok {
				dash = append(dash, v)
			}
		}
		phase, _ := num(ops[1])
		ip.st.dash, ip.st.dashPhase = dash, phase
	case "G":
		ip.setGray(true)
	case "g":
		ip.setGray(false)
	case "RG":
		ip.setRGB(true)
	case "rg":
		ip.setRGB(false)
	case "K":
		ip.setCMYK(true)
	case "k":
		ip.setCMYK(false)
	case "CS":
		if ops, ok := ip.take(1); ok {
			if n, ok := name(ops[0]); ok {
				ip.st.strokeSpace = n
			}
		}
	case "cs":
		if ops, ok := ip.take(1); ok {
			if n, ok := name(ops[0]); ok {
				ip.st.fillSpace = n
			}
		}
	case "SC", "SCN":
		ip.setComponents(true)
	case "sc", "scn":
		ip.setComponents(false)
	case "m":
		ip.move()
	case "l":
		ip.line()
	case "c":
		ip.curve()
	case "v":
		ip.curveV()
	case "y":
		ip.curveY()
	case "h":
		ip.closePath()
	case "re":
		ip.rect()
	case "S", "s", "f", "F", "f*", "B", "B*", "b", "b*", "n":
		return ip.paint(t.op)
	case "W", "W*":
		ip.markClip(t.op == "W*")
	case "BT":
		ip.stack = nil
		ip.tm, ip.tlm = matrix{1, 0, 0, 1, 0, 0}, matrix{1, 0, 0, 1, 0, 0}
	case "ET":
		ip.stack = nil
		return ip.flushText()
	case "Tc":
		ip.setNumber(&ip.st.charSpace)
	case "Tw":
		ip.setNumber(&ip.st.wordSpace)
	case "Tz":
		ip.setPercent(&ip.st.hscale)
	case "TL":
		ip.setNumber(&ip.st.leading)
	case "Ts":
		ip.setNumber(&ip.st.rise)
	case "Tr":
		if ops, ok := ip.take(1); ok {
			if v, ok := num(ops[0]); ok {
				ip.st.render = int(v)
			}
		}
	case "Tf":
		return ip.setFont()
	case "Td":
		return ip.td()
	case "TD":
		return ip.tdLeading()
	case "Tm":
		return ip.setTM()
	case "T*":
		return ip.nextLine()
	case "Tj":
		if ops, ok := ip.take(1); ok {
			if s, ok := ops[0].(syntax.String); ok {
				return ip.show(s, 0)
			}
		}
	case "TJ":
		return ip.showTJ()
	case "'":
		if err := ip.nextLine(); err != nil {
			return err
		}
		if ops, ok := ip.take(1); ok {
			if s, ok := ops[0].(syntax.String); ok {
				return ip.show(s, 0)
			}
		}
	case "\"":
		ops, ok := ip.take(3)
		if !ok {
			return nil
		}
		if w, ok := num(ops[0]); ok {
			ip.st.wordSpace = w
		}
		if c, ok := num(ops[1]); ok {
			ip.st.charSpace = c
		}
		if err := ip.nextLine(); err != nil {
			return err
		}
		if s, ok := ops[2].(syntax.String); ok {
			return ip.show(s, 0)
		}
	case "Do":
		ip.stack = nil
		ip.diagOnce(diag.Degraded, "CNT-FORM-DEFERRED", "Form/Image XObject is deferred")
	case "BI":
		ip.stack = nil
		ip.lx.skipInlineImage()
		ip.diagOnce(diag.Degraded, "CNT-INLINE-IMAGE", "inline image skipped")
	case "sh":
		ip.stack = nil
		ip.diagOnce(diag.Degraded, "CNT-SHADING-DEFERRED", "shading deferred")
	case "gs":
		ip.stack = nil
		ip.diagOnce(diag.Degraded, "CNT-GSTATE", "ExtGState deferred")
	case "BX":
		ip.stack = nil
		ip.suppress = true
	case "EX":
		ip.stack = nil
		ip.suppress = false
	case "ri", "i", "MP", "DP", "BMC", "BDC", "EMC", "d0", "d1":
		ip.stack = nil
	default:
		ip.stack = nil
		if !ip.suppress {
			ip.diagOnce(diag.Info, "CNT-UNKNOWN-OP", "unknown operator: "+t.op)
		}
	}
	return nil
}

func (ip *interp) setNumber(dst *float64) {
	if ops, ok := ip.take(1); ok {
		if v, ok := num(ops[0]); ok {
			*dst = v
		}
	}
}

func (ip *interp) setPercent(dst *float64) {
	if ops, ok := ip.take(1); ok {
		if v, ok := num(ops[0]); ok {
			*dst = v / 100
		}
	}
}

func (ip *interp) setGray(stroke bool) {
	ops, ok := ip.take(1)
	if !ok {
		return
	}
	v, ok := num(ops[0])
	if !ok {
		return
	}
	ip.setPaint(stroke, "DeviceGray", []float64{v}, [4]float64{v, v, v, 1}, false)
}

func (ip *interp) setRGB(stroke bool) {
	ops, ok := ip.take(3)
	if !ok {
		return
	}
	var c [3]float64
	for i := range c {
		v, ok := num(ops[i])
		if !ok {
			return
		}
		c[i] = v
	}
	ip.setPaint(stroke, "DeviceRGB", c[:], [4]float64{c[0], c[1], c[2], 1}, false)
}

func (ip *interp) setCMYK(stroke bool) {
	ops, ok := ip.take(4)
	if !ok {
		return
	}
	var c [4]float64
	for i := range c {
		v, ok := num(ops[i])
		if !ok {
			return
		}
		c[i] = v
	}
	k := c[3]
	rgba := [4]float64{(1 - c[0]) * (1 - k), (1 - c[1]) * (1 - k), (1 - c[2]) * (1 - k), 1}
	ip.diagOnce(diag.Info, "CLR-CMYK-APPROX", "CMYK converted with fixed approximation")
	ip.setPaint(stroke, "DeviceCMYK", c[:], rgba, true)
}

func (ip *interp) setComponents(stroke bool) {
	all := ip.all()
	space := ip.st.fillSpace
	if stroke {
		space = ip.st.strokeSpace
	}
	var vals []float64
	for _, o := range all {
		if v, ok := num(o); ok {
			vals = append(vals, v)
		}
	}
	switch {
	case space == "DeviceGray" || (space == "" && len(vals) == 1):
		if len(vals) >= 1 {
			ip.setPaint(stroke, "DeviceGray", vals[:1], [4]float64{vals[0], vals[0], vals[0], 1}, false)
		}
	case space == "DeviceRGB" || (space == "" && len(vals) == 3):
		if len(vals) >= 3 {
			ip.setPaint(stroke, "DeviceRGB", vals[:3], [4]float64{vals[0], vals[1], vals[2], 1}, false)
		}
	case space == "DeviceCMYK" || (space == "" && len(vals) == 4):
		if len(vals) >= 4 {
			k := vals[3]
			rgba := [4]float64{(1 - vals[0]) * (1 - k), (1 - vals[1]) * (1 - k), (1 - vals[2]) * (1 - k), 1}
			ip.diagOnce(diag.Info, "CLR-CMYK-APPROX", "CMYK converted with fixed approximation")
			ip.setPaint(stroke, "DeviceCMYK", vals[:4], rgba, true)
		}
	default:
		ip.diagOnce(diag.Degraded, "CLR-SPACE-DEFERRED", "unsupported color space")
	}
}

func (ip *interp) setPaint(stroke bool, space string, raw []float64, rgba [4]float64, approx bool) {
	p := displaylist.Paint{Kind: displaylist.PaintSolid, RGBA: rgba, Space: space, Raw: append([]float64(nil), raw...), Approx: approx}
	if stroke {
		ip.st.stroke = p
	} else {
		ip.st.fill = p
	}
}

func (ip *interp) transform(x, y float64) displaylist.Point {
	px, py := ip.st.ctm.apply(x, y)
	return displaylist.Point{X: px, Y: py}
}

func (ip *interp) move() {
	ops, ok := ip.take(2)
	if !ok {
		return
	}
	x, ok1 := num(ops[0])
	y, ok2 := num(ops[1])
	if !ok1 || !ok2 {
		return
	}
	p := ip.transform(x, y)
	ip.path = append(ip.path, displaylist.Segment{Kind: displaylist.SegmentMove, P0: p})
	ip.current = displaylist.Point{X: x, Y: y}
	ip.startPt = ip.current
	ip.haveCur = true
}

func (ip *interp) line() {
	ops, ok := ip.take(2)
	if !ok || !ip.haveCur {
		return
	}
	x, ok1 := num(ops[0])
	y, ok2 := num(ops[1])
	if !ok1 || !ok2 {
		return
	}
	ip.path = append(ip.path, displaylist.Segment{Kind: displaylist.SegmentLine, P0: ip.transform(ip.current.X, ip.current.Y), P3: ip.transform(x, y)})
	ip.current = displaylist.Point{X: x, Y: y}
}

func (ip *interp) curve() {
	ops, ok := ip.take(6)
	if !ok || !ip.haveCur {
		return
	}
	var v [6]float64
	for i := range v {
		n, ok := num(ops[i])
		if !ok {
			return
		}
		v[i] = n
	}
	ip.path = append(ip.path, displaylist.Segment{
		Kind: displaylist.SegmentCubic,
		P0:   ip.transform(ip.current.X, ip.current.Y),
		P1:   ip.transform(v[0], v[1]),
		P2:   ip.transform(v[2], v[3]),
		P3:   ip.transform(v[4], v[5]),
	})
	ip.current = displaylist.Point{X: v[4], Y: v[5]}
}

func (ip *interp) curveV() {
	ops, ok := ip.take(4)
	if !ok || !ip.haveCur {
		return
	}
	x2, _ := num(ops[0])
	y2, _ := num(ops[1])
	x3, _ := num(ops[2])
	y3, _ := num(ops[3])
	ip.path = append(ip.path, displaylist.Segment{
		Kind: displaylist.SegmentCubic,
		P0:   ip.transform(ip.current.X, ip.current.Y),
		P1:   ip.transform(ip.current.X, ip.current.Y),
		P2:   ip.transform(x2, y2),
		P3:   ip.transform(x3, y3),
	})
	ip.current = displaylist.Point{X: x3, Y: y3}
}

func (ip *interp) curveY() {
	ops, ok := ip.take(4)
	if !ok || !ip.haveCur {
		return
	}
	x1, _ := num(ops[0])
	y1, _ := num(ops[1])
	x3, _ := num(ops[2])
	y3, _ := num(ops[3])
	ip.path = append(ip.path, displaylist.Segment{
		Kind: displaylist.SegmentCubic,
		P0:   ip.transform(ip.current.X, ip.current.Y),
		P1:   ip.transform(x1, y1),
		P2:   ip.transform(x3, y3),
		P3:   ip.transform(x3, y3),
	})
	ip.current = displaylist.Point{X: x3, Y: y3}
}

func (ip *interp) closePath() {
	ip.stack = nil
	if !ip.haveCur {
		return
	}
	ip.path = append(ip.path, displaylist.Segment{Kind: displaylist.SegmentClose, P0: ip.transform(ip.current.X, ip.current.Y), P3: ip.transform(ip.startPt.X, ip.startPt.Y)})
	ip.current = ip.startPt
}

func (ip *interp) rect() {
	ops, ok := ip.take(4)
	if !ok {
		return
	}
	x, ok1 := num(ops[0])
	y, ok2 := num(ops[1])
	w, ok3 := num(ops[2])
	h, ok4 := num(ops[3])
	if !ok1 || !ok2 || !ok3 || !ok4 {
		return
	}
	p1 := ip.transform(x, y)
	p2 := ip.transform(x+w, y)
	p3 := ip.transform(x+w, y+h)
	p4 := ip.transform(x, y+h)
	ip.path = append(ip.path,
		displaylist.Segment{Kind: displaylist.SegmentMove, P0: p1},
		displaylist.Segment{Kind: displaylist.SegmentLine, P0: p1, P3: p2},
		displaylist.Segment{Kind: displaylist.SegmentLine, P0: p2, P3: p3},
		displaylist.Segment{Kind: displaylist.SegmentLine, P0: p3, P3: p4},
		displaylist.Segment{Kind: displaylist.SegmentClose, P0: p4, P3: p1},
	)
	ip.current = displaylist.Point{X: x, Y: y}
	ip.startPt = ip.current
	ip.haveCur = true
}

func (ip *interp) markClip(evenOdd bool) {
	ip.stack = nil
	if len(ip.path) == 0 {
		return
	}
	rule := displaylist.NonZero
	if evenOdd {
		rule = displaylist.EvenOdd
	}
	seg := append([]displaylist.Segment(nil), ip.path...)
	ip.pendingClip = &displaylist.Clip{Path: seg, Rule: rule, Bounds: segmentBounds(seg)}
}

func (ip *interp) paint(op string) error {
	if err := ip.flushText(); err != nil {
		return err
	}
	if len(ip.path) == 0 {
		return nil
	}
	if ip.pendingClip != nil {
		if ip.curClip >= 0 {
			ip.diagOnce(diag.Degraded, "CNT-CLIP-INTERSECT", "clip intersection is not represented exactly")
		}
		ip.clips = append(ip.clips, *ip.pendingClip)
		ip.curClip = len(ip.clips) - 1
		ip.pendingClip = nil
	}
	if op == "n" {
		ip.path = nil
		ip.haveCur = false
		return nil
	}
	var fill, stroke bool
	rule := displaylist.NonZero
	switch op {
	case "S":
		stroke = true
	case "s":
		stroke = true
		ip.closeCurrent()
	case "f", "F":
		fill = true
	case "f*":
		fill, rule = true, displaylist.EvenOdd
	case "B":
		fill, stroke = true, true
	case "B*":
		fill, stroke, rule = true, true, displaylist.EvenOdd
	case "b":
		fill, stroke = true, true
		ip.closeCurrent()
	case "b*":
		fill, stroke, rule = true, true, displaylist.EvenOdd
		ip.closeCurrent()
	}
	seg := append([]displaylist.Segment(nil), ip.path...)
	bounds := segmentBounds(seg)
	c := ip.common()
	item := &displaylist.Path{Common: c, Segments: seg, FillRule: rule, Hint: displaylist.PathGeneric}
	if fill {
		p := ip.st.fill
		item.Fill = &p
	}
	if stroke {
		p := ip.st.stroke
		item.Stroke = &p
	}
	scale := ip.st.ctm.scale()
	item.Line = displaylist.LineStyle{
		Width: ip.st.lineWidth * scale, Cap: ip.st.cap, Join: ip.st.join,
		Miter: ip.st.miter, Dash: append([]float64(nil), ip.st.dash...), Phase: ip.st.dashPhase,
	}
	if bounds.X1 < ip.pd.CropBox.X0 || bounds.X0 > ip.pd.CropBox.X1 || bounds.Y1 < ip.pd.CropBox.Y0 || bounds.Y0 > ip.pd.CropBox.Y1 {
		item.Flags |= displaylist.FlagOutOfBounds
	}
	if fill && ip.st.fill.Approx || stroke && ip.st.stroke.Approx || scale != 1 {
		item.Flags |= displaylist.FlagApprox
	}
	item.Bounds = bounds
	item.Closed = op == "s" || op == "b" || op == "b*" || op == "f*" || op == "B*"
	if err := ip.emit(item); err != nil {
		return err
	}
	ip.path = nil
	ip.haveCur = false
	return nil
}

func (ip *interp) closeCurrent() {
	if ip.haveCur {
		ip.path = append(ip.path, displaylist.Segment{Kind: displaylist.SegmentClose, P0: ip.transform(ip.current.X, ip.current.Y), P3: ip.transform(ip.startPt.X, ip.startPt.Y)})
	}
}

func (ip *interp) common() displaylist.Common {
	return displaylist.Common{Z: ip.z, Clip: ip.curClip, Alpha: 1, MCID: -1, StructRef: -1}
}

func (ip *interp) emit(item displaylist.Item) error {
	c := item.Base()
	c.Z = ip.z
	c.ID = ip.id
	ip.z++
	ip.id++
	return ip.sink(item)
}

func (ip *interp) flushText() error {
	if ip.tr == nil {
		return nil
	}
	ip.tr.Bounds = ip.trBounds
	if ip.trBounds.X1 < ip.pd.CropBox.X0 || ip.trBounds.X0 > ip.pd.CropBox.X1 || ip.trBounds.Y1 < ip.pd.CropBox.Y0 || ip.trBounds.Y0 > ip.pd.CropBox.Y1 {
		ip.tr.Flags |= displaylist.FlagOutOfBounds
	}
	if err := ip.sink(ip.tr); err != nil {
		return err
	}
	ip.tr = nil
	ip.trHasBounds = false
	return nil
}

func (ip *interp) setFont() error {
	ops, ok := ip.take(2)
	if !ok {
		return nil
	}
	name, ok := name(ops[0])
	if !ok {
		return nil
	}
	size, ok := num(ops[1])
	if !ok {
		return nil
	}
	if err := ip.flushText(); err != nil {
		return err
	}
	ip.st.font = ip.loadFont(name)
	ip.st.fontSize = size
	return nil
}

func (ip *interp) loadFont(name string) *font.Font {
	if ip.fonts == nil {
		ip.fonts = map[string]*font.Font{}
	}
	if f, ok := ip.fonts[name]; ok {
		return f
	}
	var f *font.Font
	res := ip.pd.Resources
	if res != nil {
		if fonts, ok := res.DictValue("Font"); ok {
			if v, ok := fonts[syntax.Name(name)]; ok {
				if d, ok := ip.resolveDict(v); ok {
					if ff, err := font.Load(ip.ctx, ip.pd.File, d, ip.pd.Diag); err == nil {
						f = ff
					}
				}
			}
		}
	}
	if f == nil {
		ip.diagOnce(diag.Warn, "FNT-MISSING", "font resource missing: "+name)
	}
	ip.fonts[name] = f
	return f
}

func (ip *interp) resolveDict(o syntax.Object) (syntax.Dict, bool) {
	if d, ok := o.(syntax.Dict); ok {
		return d, true
	}
	if ip.pd.File == nil {
		return nil, false
	}
	v, err := ip.pd.File.Resolve(ip.ctx, o)
	if err != nil {
		return nil, false
	}
	d, ok := v.(syntax.Dict)
	return d, ok
}

func (ip *interp) td() error {
	ops, ok := ip.take(2)
	if !ok {
		return nil
	}
	tx, ok1 := num(ops[0])
	ty, ok2 := num(ops[1])
	if !ok1 || !ok2 {
		return nil
	}
	ip.tlm = mmul(translate(tx, ty), ip.tlm)
	ip.tm = ip.tlm
	return nil
}

func (ip *interp) tdLeading() error {
	ops, ok := ip.take(2)
	if !ok {
		return nil
	}
	tx, ok1 := num(ops[0])
	ty, ok2 := num(ops[1])
	if !ok1 || !ok2 {
		return nil
	}
	ip.st.leading = -ty
	ip.tlm = mmul(translate(tx, ty), ip.tlm)
	ip.tm = ip.tlm
	return nil
}

func (ip *interp) setTM() error {
	ops, ok := ip.take(6)
	if !ok {
		return nil
	}
	var m matrix
	for i := range m {
		v, ok := num(ops[i])
		if !ok {
			return nil
		}
		m[i] = v
	}
	ip.tm, ip.tlm = m, m
	return nil
}

func (ip *interp) nextLine() error {
	ip.tlm = mmul(translate(0, -ip.st.leading), ip.tlm)
	ip.tm = ip.tlm
	return nil
}

func (ip *interp) show(s syntax.String, _ float64) error {
	if len(s) == 0 {
		return nil
	}
	if ip.tr != nil && (ip.trClip != ip.curClip || !ip.runMatches()) {
		if err := ip.flushText(); err != nil {
			return err
		}
	}
	var codes []font.Code
	if ip.st.font != nil {
		codes = ip.st.font.Codes([]byte(s))
		if len(codes)*ip.st.font.CodeBytes() != len(s) {
			ip.diagOnce(diag.Warn, "CNT-CODE-TRUNCATED", "truncated character code at end of string")
		}
	} else {
		codes = make([]font.Code, len(s))
		for i, b := range s {
			codes[i] = font.Code{Value: uint32(b), Bytes: 1}
		}
	}
	if len(codes) == 0 {
		return nil
	}
	if ip.tr == nil {
		ip.tr = &displaylist.TextRun{
			Font: fontInfo(ip.st.font), Size: ip.st.fontSize, Fill: ip.st.fill,
			RenderMode: ip.st.render, CharSpace: ip.st.charSpace,
			WordSpace: ip.st.wordSpace, HScale: ip.st.hscale, Rise: ip.st.rise,
			Vertical: ip.st.font != nil && ip.st.font.Vertical(),
		}
		ip.tr.Common = ip.common()
		ip.trClip = ip.curClip
		if ip.st.render == 3 {
			ip.tr.TextFlags |= displaylist.TextInvisible
		}
		if ip.st.render == 1 || ip.st.render == 2 {
			ip.tr.TextFlags |= displaylist.TextStroked
			p := ip.st.stroke
			ip.tr.Stroke = &p
		}
	}
	for _, c := range codes {
		trm := ip.textMatrix()
		ox, oy := trm.apply(0, 0)
		width := 500.0
		text, ok := "\uFFFD", false
		if ip.st.font != nil {
			width = ip.st.font.WidthCode(c)
			text, ok = ip.st.font.DecodeCode(c)
		}
		adv := (width/1000*ip.st.fontSize + ip.st.charSpace) * ip.st.hscale
		if c.Bytes == 1 && c.Value == 32 {
			adv = (width/1000*ip.st.fontSize + ip.st.charSpace + ip.st.wordSpace) * ip.st.hscale
		}
		g := displaylist.Glyph{
			Text: text, UnicodeOK: ok, Code: c.Value, CID: c.CID,
			Origin: displaylist.Point{X: ox, Y: oy}, Advance: adv, Matrix: displaylist.Matrix(trm),
		}
		ip.tr.Glyphs = append(ip.tr.Glyphs, g)
		ip.growTextBounds(g)
		ip.tm = mmul(translate(adv, 0), ip.tm)
	}
	return nil
}

func (ip *interp) runMatches() bool {
	if ip.tr == nil {
		return true
	}
	f := fontInfo(ip.st.font)
	return ip.tr.Font == f && ip.tr.Size == ip.st.fontSize && ip.tr.Vertical == (ip.st.font != nil && ip.st.font.Vertical()) &&
		ip.tr.RenderMode == ip.st.render && paintEqual(ip.tr.Fill, ip.st.fill) &&
		ip.tr.CharSpace == ip.st.charSpace && ip.tr.WordSpace == ip.st.wordSpace &&
		ip.tr.HScale == ip.st.hscale && ip.tr.Rise == ip.st.rise
}

func paintEqual(a, b displaylist.Paint) bool {
	if a.Kind != b.Kind || a.RGBA != b.RGBA || a.Space != b.Space || a.Approx != b.Approx || len(a.Raw) != len(b.Raw) {
		return false
	}
	for i := range a.Raw {
		if a.Raw[i] != b.Raw[i] {
			return false
		}
	}
	return true
}

func (ip *interp) textMatrix() matrix {
	s := matrix{ip.st.fontSize * ip.st.hscale, 0, 0, ip.st.fontSize, 0, ip.st.rise}
	return mmul(ip.st.ctm, mmul(ip.tm, s))
}

func (ip *interp) growTextBounds(g displaylist.Glyph) {
	r := displaylist.Rect{X0: g.Origin.X, Y0: g.Origin.Y - ip.st.fontSize, X1: g.Origin.X + g.Advance, Y1: g.Origin.Y + ip.st.fontSize*0.2}
	if !ip.trHasBounds {
		ip.trBounds = r
		ip.trHasBounds = true
		return
	}
	if r.X0 < ip.trBounds.X0 {
		ip.trBounds.X0 = r.X0
	}
	if r.Y0 < ip.trBounds.Y0 {
		ip.trBounds.Y0 = r.Y0
	}
	if r.X1 > ip.trBounds.X1 {
		ip.trBounds.X1 = r.X1
	}
	if r.Y1 > ip.trBounds.Y1 {
		ip.trBounds.Y1 = r.Y1
	}
}

func (ip *interp) showTJ() error {
	ops, ok := ip.take(1)
	if !ok {
		return nil
	}
	arr, ok := ops[0].(syntax.Array)
	if !ok {
		return nil
	}
	for _, o := range arr {
		switch v := o.(type) {
		case syntax.String:
			if err := ip.show(v, 0); err != nil {
				return err
			}
		default:
			if n, ok := num(o); ok {
				ip.tm = mmul(translate(-n*ip.st.fontSize*ip.st.hscale/1000, 0), ip.tm)
			}
		}
	}
	return nil
}

func segmentBounds(seg []displaylist.Segment) displaylist.Rect {
	if len(seg) == 0 {
		return displaylist.Rect{}
	}
	r := displaylist.Rect{X0: math.Inf(1), Y0: math.Inf(1), X1: math.Inf(-1), Y1: math.Inf(-1)}
	add := func(p displaylist.Point) {
		if p.X < r.X0 {
			r.X0 = p.X
		}
		if p.Y < r.Y0 {
			r.Y0 = p.Y
		}
		if p.X > r.X1 {
			r.X1 = p.X
		}
		if p.Y > r.Y1 {
			r.Y1 = p.Y
		}
	}
	for _, s := range seg {
		switch s.Kind {
		case displaylist.SegmentMove:
			add(s.P0)
		case displaylist.SegmentLine, displaylist.SegmentClose:
			add(s.P0)
			add(s.P3)
		case displaylist.SegmentCubic:
			add(s.P0)
			add(s.P1)
			add(s.P2)
			add(s.P3)
		}
	}
	return r
}

func fontInfo(f *font.Font) *displaylist.FontInfo {
	if f == nil {
		return nil
	}
	return f.Info
}
