package syntax

import (
	"bytes"
	"container/list"
	"context"
	"errors"
	"io"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/F31/go-pdf/internal/budget"
	"github.com/F31/go-pdf/internal/crypt"
	"github.com/F31/go-pdf/internal/diag"
	"github.com/F31/go-pdf/internal/errs"
	"github.com/F31/go-pdf/internal/filter"
)

type EntryType int

const (
	EntryFree EntryType = iota
	EntryInUse
	EntryCompressed
)

type XRefEntry struct {
	Type   EntryType
	Offset int64
	Gen    int
	ObjStm int
	Index  int
}

type Options struct {
	Repair   bool
	Password string
	Tracker  *budget.Tracker
	Diag     *diag.Collector
}

type cacheItem struct {
	ref Ref
	obj Object
}

type File struct {
	data    []byte
	opts    Options
	tracker *budget.Tracker
	diag    *diag.Collector

	mu         sync.Mutex
	closed     bool
	xref       map[int]XRefEntry
	trailer    Dict
	cache      map[Ref]*list.Element
	lru        *list.List
	objStm     map[Ref]Object
	repaired   map[Ref]Object
	expanding  map[int]bool
	crypt      *crypt.Handler
	encryptRef Ref
	encryptSet bool

	repairedFlag bool
	header       string
	headerOffset int64
}

const cacheCapacity = 8192

func Open(ctx context.Context, data []byte, opts Options) (f *File, err error) {
	defer func() {
		if p := recover(); p != nil {
			f = nil
			err = errs.Wrap(errs.ErrCorrupt, "SYN-PANIC", "open document", -1, 0, -1)
		}
	}()
	t := opts.Tracker
	if t == nil {
		t = budget.NewTracker(budget.From(ctx))
	}
	c := opts.Diag
	if c == nil {
		c = diag.New(diag.DefaultCapacity)
	}
	if err := t.CheckFileSize(int64(len(data))); err != nil {
		c.Add(diag.Diagnostic{Level: diag.Error, Code: "BUD-FILE-SIZE", Msg: "file exceeds budget", Page: -1, Offset: -1})
		return nil, err
	}
	f = &File{
		data: data, opts: opts, tracker: t, diag: c,
		xref: map[int]XRefEntry{}, trailer: Dict{},
		cache: map[Ref]*list.Element{}, lru: list.New(),
		objStm: map[Ref]Object{}, repaired: map[Ref]Object{},
		expanding: map[int]bool{},
	}
	if err := f.parseHeader(); err != nil {
		return nil, err
	}
	err = f.loadXref(ctx)
	if err != nil && opts.Repair {
		c.Add(diag.Diagnostic{Level: diag.Warn, Code: "SYN-REPAIR", Msg: "xref parse failed, rebuilding", Page: -1, Offset: -1})
		f.resetForRepair()
		if rerr := f.repair(ctx); rerr != nil {
			return nil, rerr
		}
		return f, nil
	} else if err != nil {
		return nil, err
	}
	if err := f.initSecurity(ctx, opts.Password); err != nil {
		return nil, err
	}
	if opts.Repair {
		if _, rerr := f.ResolveRoot(ctx); rerr != nil && errors.Is(rerr, errs.ErrCorrupt) {
			c.Add(diag.Diagnostic{Level: diag.Warn, Code: "SYN-REPAIR", Msg: "catalog unavailable, rebuilding", Page: -1, Offset: -1})
			f.resetForRepair()
			if rerr := f.repair(ctx); rerr != nil {
				return nil, rerr
			}
		}
	}
	return f, nil
}

func (f *File) resetForRepair() {
	f.xref = map[int]XRefEntry{}
	f.trailer = Dict{}
	f.cache = map[Ref]*list.Element{}
	f.lru = list.New()
	f.objStm = map[Ref]Object{}
	f.repaired = map[Ref]Object{}
	f.expanding = map[int]bool{}
}

func (f *File) parseHeader() error {
	idx := bytes.Index(f.data, []byte("%PDF-"))
	if idx < 0 || idx > 1024 {
		return errs.Wrap(errs.ErrCorrupt, "SYN-NO-HEADER", "parse header", 0, 0, -1)
	}
	f.headerOffset = int64(idx)
	end := idx + 5
	for end < len(f.data) && !isSpace(f.data[end]) && end-idx < 16 {
		end++
	}
	f.header = string(f.data[idx+5 : end])
	if bytes.Contains(f.data, []byte("/Linearized")) {
		f.diag.Add(diag.Diagnostic{Level: diag.Info, Code: "DOC-LINEARIZED", Msg: "linearized file read as a normal document", Page: -1, Offset: int64(idx)})
	}
	if !validVersion(f.header) {
		f.diag.Add(diag.Diagnostic{Level: diag.Warn, Code: "DOC-VERSION-UNKNOWN", Msg: "unrecognized PDF header version", Page: -1, Offset: int64(idx)})
	}
	return nil
}

func validVersion(v string) bool {
	parts := strings.Split(v, ".")
	if len(parts) != 2 {
		return false
	}
	_, e1 := strconv.Atoi(parts[0])
	_, e2 := strconv.Atoi(parts[1])
	return e1 == nil && e2 == nil
}

func (f *File) HeaderVersion() string { return f.header }
func (f *File) Repaired() bool        { return f.repairedFlag }
func (f *File) Diag() *diag.Collector { return f.diag }
func (f *File) Trailer() Dict         { return f.trailer }

func (f *File) loadXref(ctx context.Context) error {
	start, err := f.findStartXref()
	if err != nil {
		return err
	}
	seen := map[int64]bool{}
	cur := start
	for depth := 0; ; depth++ {
		if depth > f.tracker.Budget().MaxXRefChain {
			return errs.Wrap(errs.ErrBudget, "BUD-XREF-CHAIN", "load xref", cur, 0, -1)
		}
		if seen[cur] {
			return errs.Wrap(errs.ErrCorrupt, "SYN-XREF-CYCLE", "load xref", cur, 0, -1)
		}
		seen[cur] = true
		entries, trailer, err := f.parseXrefSection(ctx, cur)
		if err != nil {
			return err
		}
		f.mergeXref(entries)
		f.mergeTrailer(trailer)
		if stm, ok := trailer.IntValue("XRefStm"); ok {
			entries2, trailer2, err := f.parseXrefSection(ctx, stm)
			if err != nil {
				return err
			}
			f.mergeXref(entries2)
			f.mergeTrailer(trailer2)
		}
		prev, ok := trailer.IntValue("Prev")
		if !ok || prev <= 0 {
			break
		}
		cur = prev
	}
	if err := f.tracker.AddObjects(int64(len(f.xref))); err != nil {
		return err
	}
	return nil
}

func (f *File) findStartXref() (int64, error) {
	idx := bytes.LastIndex(f.data, []byte("startxref"))
	if idx < 0 {
		return 0, errs.Wrap(errs.ErrCorrupt, "SYN-NO-STARTXREF", "find startxref", -1, 0, -1)
	}
	p := newParser(f.data, idx+len("startxref"), false, 64)
	p.skipSpace()
	obj, _, err := p.parseNumber()
	if err != nil {
		return 0, err
	}
	n, ok := obj.(Int)
	if !ok || n < 0 || int(n) >= len(f.data) {
		return 0, errs.Wrap(errs.ErrCorrupt, "SYN-BAD-STARTXREF", "find startxref", int64(idx), 0, -1)
	}
	return int64(n), nil
}

func (f *File) mergeXref(entries map[int]XRefEntry) {
	for num, e := range entries {
		if _, exists := f.xref[num]; !exists {
			f.xref[num] = e
		}
	}
}

func (f *File) mergeTrailer(t Dict) {
	for k, v := range t {
		if _, exists := f.trailer[k]; !exists {
			f.trailer[k] = v
		}
	}
}

func (f *File) parseXrefSection(ctx context.Context, offset int64) (map[int]XRefEntry, Dict, error) {
	if offset < 0 || offset >= int64(len(f.data)) {
		return nil, nil, errs.Wrap(errs.ErrCorrupt, "SYN-XREF-OFFSET", "parse xref", offset, 0, -1)
	}
	p := newParser(f.data, int(offset), false, 64)
	p.skipSpace()
	if p.hasKeyword("xref") {
		return p.parseClassicXref()
	}
	ref, obj, err := f.parseIndirectAt(ctx, offset, false)
	if err != nil {
		return nil, nil, err
	}
	stm, ok := obj.(*Stream)
	if !ok {
		return nil, nil, errs.Wrap(errs.ErrCorrupt, "SYN-XREF-STREAM", "parse xref stream", offset, ref.Num, -1)
	}
	entries, err := f.parseXrefStream(ctx, stm)
	if err != nil {
		return nil, nil, err
	}
	return entries, stm.Dict, nil
}

func (p *parser) parseClassicXref() (map[int]XRefEntry, Dict, error) {
	p.pos += len("xref")
	entries := map[int]XRefEntry{}
	for {
		p.skipSpace()
		if p.eof() {
			return nil, nil, p.err("SYN-XREF-EOF", "unterminated xref table", p.pos)
		}
		if p.hasKeyword("trailer") {
			p.pos += len("trailer")
			break
		}
		startObj, _, err := p.parseNumber()
		if err != nil {
			return nil, nil, err
		}
		p.skipSpace()
		countObj, _, err := p.parseNumber()
		if err != nil {
			return nil, nil, err
		}
		start, ok1 := startObj.(Int)
		count, ok2 := countObj.(Int)
		if !ok1 || !ok2 || count < 0 || count > 10_000_000 {
			return nil, nil, p.err("SYN-XREF-SECTION", "invalid xref subsection", p.pos)
		}
		for i := 0; i < int(count); i++ {
			p.skipSpace()
			offObj, _, err := p.parseNumber()
			if err != nil {
				return nil, nil, err
			}
			p.skipSpace()
			genObj, _, err := p.parseNumber()
			if err != nil {
				return nil, nil, err
			}
			p.skipSpace()
			typ := p.readKeyword()
			off, ok1 := offObj.(Int)
			gen, ok2 := genObj.(Int)
			if !ok1 || !ok2 || (typ != "n" && typ != "f") {
				return nil, nil, p.err("SYN-XREF-ENTRY", "invalid xref entry", p.pos)
			}
			num := int(start) + i
			if typ == "n" {
				entries[num] = XRefEntry{Type: EntryInUse, Offset: int64(off), Gen: int(gen)}
			} else {
				entries[num] = XRefEntry{Type: EntryFree, Gen: int(gen)}
			}
		}
	}
	tobj, err := p.parseObject()
	if err != nil {
		return nil, nil, err
	}
	trailer, ok := tobj.(Dict)
	if !ok {
		return nil, nil, p.err("SYN-XREF-TRAILER", "trailer is not a dictionary", p.pos)
	}
	return entries, trailer, nil
}

func (f *File) parseXrefStream(ctx context.Context, stm *Stream) (map[int]XRefEntry, error) {
	wArr, ok := stm.Dict.ArrayValue("W")
	if !ok || len(wArr) < 3 {
		return nil, errs.Wrap(errs.ErrCorrupt, "SYN-XREF-W", "parse xref stream", stm.Offset, 0, -1)
	}
	var w [3]int
	for i := 0; i < 3; i++ {
		n, ok := AsInt(wArr[i])
		if !ok || n < 0 || n > 8 {
			return nil, errs.Wrap(errs.ErrCorrupt, "SYN-XREF-W", "parse xref stream", stm.Offset, 0, -1)
		}
		w[i] = int(n)
	}
	rowLen := w[0] + w[1] + w[2]
	if rowLen <= 0 {
		return nil, errs.Wrap(errs.ErrCorrupt, "SYN-XREF-W", "parse xref stream", stm.Offset, 0, -1)
	}
	size, _ := stm.Dict.IntValue("Size")
	var index []int
	if arr, ok := stm.Dict.ArrayValue("Index"); ok {
		for _, o := range arr {
			n, ok := AsInt(o)
			if !ok {
				return nil, errs.Wrap(errs.ErrCorrupt, "SYN-XREF-INDEX", "parse xref stream", stm.Offset, 0, -1)
			}
			index = append(index, int(n))
		}
	} else {
		index = []int{0, int(size)}
	}
	if len(index)%2 != 0 {
		return nil, errs.Wrap(errs.ErrCorrupt, "SYN-XREF-INDEX", "parse xref stream", stm.Offset, 0, -1)
	}
	data, err := f.decodeStream(ctx, stm, 0)
	if err != nil {
		return nil, err
	}
	if len(data) < rowLen {
		return nil, errs.Wrap(errs.ErrCorrupt, "SYN-XREF-STREAM-SHORT", "parse xref stream", stm.Offset, 0, -1)
	}
	entries := map[int]XRefEntry{}
	pos := 0
	for i := 0; i+1 < len(index); i += 2 {
		start, count := index[i], index[i+1]
		for j := 0; j < count; j++ {
			if pos+rowLen > len(data) {
				return nil, errs.Wrap(errs.ErrCorrupt, "SYN-XREF-STREAM-SHORT", "parse xref stream", stm.Offset, 0, -1)
			}
			fields := [3]int64{}
			off := pos
			for k := 0; k < 3; k++ {
				for b := 0; b < w[k]; b++ {
					fields[k] = fields[k]<<8 | int64(data[off])
					off++
				}
			}
			pos += rowLen
			typ := fields[0]
			if w[0] == 0 {
				typ = 1
			}
			num := start + j
			switch typ {
			case 0:
				entries[num] = XRefEntry{Type: EntryFree, Gen: int(fields[2])}
			case 1:
				entries[num] = XRefEntry{Type: EntryInUse, Offset: fields[1], Gen: int(fields[2])}
			case 2:
				entries[num] = XRefEntry{Type: EntryCompressed, ObjStm: int(fields[1]), Index: int(fields[2])}
			}
		}
	}
	return entries, nil
}

func (f *File) Resolve(ctx context.Context, obj Object) (Object, error) {
	if r, ok := obj.(Ref); ok {
		return f.ResolveRef(ctx, r)
	}
	return obj, nil
}

func (f *File) ResolveRef(ctx context.Context, r Ref) (v Object, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return nil, errs.Wrap(errs.ErrClosed, "SYN-CLOSED", "resolve object", -1, r.Num, -1)
	}
	defer func() {
		if p := recover(); p != nil {
			v = nil
			err = errs.Wrap(errs.ErrCorrupt, "SYN-PANIC", "resolve object", -1, r.Num, -1)
		}
	}()
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap(errs.ErrCanceled, "SYN-CANCELED", "resolve object", -1, r.Num, -1)
	}
	return f.resolveLocked(ctx, r)
}

func (f *File) resolveLocked(ctx context.Context, r Ref) (Object, error) {
	if obj, ok := f.repaired[r]; ok {
		return obj, nil
	}
	if obj, ok := f.objStm[r]; ok {
		return obj, nil
	}
	if el, ok := f.cache[r]; ok {
		f.lru.MoveToFront(el)
		return el.Value.(*cacheItem).obj, nil
	}
	e, ok := f.xref[r.Num]
	if !ok {
		f.diag.Add(diag.Diagnostic{Level: diag.Warn, Code: "SYN-MISSING-OBJECT", Msg: "object not in xref", Page: -1, Obj: r.Num, Offset: -1})
		return Null{}, nil
	}
	switch e.Type {
	case EntryFree:
		return Null{}, nil
	case EntryInUse:
		ref, obj, err := f.parseIndirectAt(ctx, e.Offset, false)
		if err != nil {
			return nil, err
		}
		if ref.Num != r.Num {
			return nil, errs.Wrap(errs.ErrCorrupt, "SYN-OBJ-MISMATCH", "resolve object", e.Offset, r.Num, -1)
		}
		if err := f.tracker.AddObjects(1); err != nil {
			return nil, err
		}
		f.cachePut(r, obj)
		return obj, nil
	case EntryCompressed:
		if err := f.expandObjStmLocked(ctx, e.ObjStm); err != nil {
			return nil, err
		}
		if obj, ok := f.objStm[r]; ok {
			return obj, nil
		}
		return nil, errs.Wrap(errs.ErrCorrupt, "SYN-OBJSTM-MISSING", "resolve compressed object", -1, r.Num, -1)
	}
	return Null{}, nil
}

func (f *File) cachePut(r Ref, obj Object) {
	if el, ok := f.cache[r]; ok {
		el.Value.(*cacheItem).obj = obj
		f.lru.MoveToFront(el)
		return
	}
	f.cache[r] = f.lru.PushFront(&cacheItem{ref: r, obj: obj})
	if f.lru.Len() > cacheCapacity {
		back := f.lru.Back()
		if back != nil {
			it := back.Value.(*cacheItem)
			delete(f.cache, it.ref)
			f.lru.Remove(back)
		}
	}
}

func (f *File) expandObjStmLocked(ctx context.Context, num int) error {
	if f.expanding[num] {
		return errs.Wrap(errs.ErrCorrupt, "SYN-OBJSTM-CYCLE", "expand object stream", -1, num, -1)
	}
	f.expanding[num] = true
	defer delete(f.expanding, num)
	obj, err := f.resolveLocked(ctx, Ref{Num: num, Gen: 0})
	if err != nil {
		return err
	}
	stm, ok := obj.(*Stream)
	if !ok {
		return errs.Wrap(errs.ErrCorrupt, "SYN-OBJSTM", "object stream is not a stream", -1, num, -1)
	}
	objs, err := f.objStmObjects(ctx, stm)
	if err != nil {
		return err
	}
	for r, o := range objs {
		f.objStm[r] = o
	}
	return nil
}

func (f *File) objStmObjects(ctx context.Context, stm *Stream) (map[Ref]Object, error) {
	n, ok1 := stm.Dict.IntValue("N")
	first, ok2 := stm.Dict.IntValue("First")
	if !ok1 || !ok2 || n < 0 || n > 10_000_000 || first < 0 {
		return nil, errs.Wrap(errs.ErrCorrupt, "SYN-OBJSTM-HEADER", "read object stream", stm.Offset, 0, -1)
	}
	data, err := f.decodeStream(ctx, stm, 0)
	if err != nil {
		return nil, err
	}
	hp := newParser(data, 0, false, 64)
	type pair struct{ num, off int }
	pairs := make([]pair, 0, n)
	for i := 0; i < int(n); i++ {
		hp.skipSpace()
		a, _, err := hp.parseNumber()
		if err != nil {
			return nil, err
		}
		hp.skipSpace()
		b, _, err := hp.parseNumber()
		if err != nil {
			return nil, err
		}
		an, ok1 := a.(Int)
		bn, ok2 := b.(Int)
		if !ok1 || !ok2 {
			return nil, errs.Wrap(errs.ErrCorrupt, "SYN-OBJSTM-HEADER", "read object stream", stm.Offset, 0, -1)
		}
		pairs = append(pairs, pair{int(an), int(bn)})
	}
	out := make(map[Ref]Object, len(pairs))
	for _, pr := range pairs {
		pos := int(first) + pr.off
		if pos < 0 || pos >= len(data) {
			return nil, errs.Wrap(errs.ErrCorrupt, "SYN-OBJSTM-OFFSET", "read object stream", stm.Offset, pr.num, -1)
		}
		p := newParser(data, pos, false, f.tracker.Budget().MaxNestingDepth)
		o, err := p.parseObject()
		if err != nil {
			return nil, err
		}
		out[Ref{Num: pr.num, Gen: 0}] = o
	}
	return out, nil
}

// decodeStream applies the stream's filter chain.
func (f *File) decodeStream(ctx context.Context, stm *Stream, decodedLimit int64) ([]byte, error) {
	r, err := f.streamReader(ctx, stm, decodedLimit)
	if err != nil {
		return nil, err
	}
	out, err := io.ReadAll(r)
	if err != nil {
		return nil, errs.Wrap(err, "FLT-DECODE", "read decoded stream", stm.Offset, 0, -1)
	}
	return out, nil
}

func (f *File) streamReader(ctx context.Context, stm *Stream, decodedLimit int64) (io.Reader, error) {
	raw := stm.Raw
	if f.crypt != nil && stm.Ref.Num != 0 {
		typ, _ := stm.Dict.NameValue("Type")
		if typ != "XRef" && !(typ == "Metadata" && !f.crypt.MetadataEncrypted()) {
			var err error
			raw, err = f.crypt.DecryptStream(stm.Ref.Num, stm.Ref.Gen, raw)
			if err != nil {
				return nil, err
			}
		}
	}
	names, parms, err := f.filterSpec(ctx, stm.Dict)
	if err != nil {
		return nil, err
	}
	if len(names) == 0 {
		return bytes.NewReader(raw), nil
	}
	return filter.DecodeChainLimit(ctx, bytes.NewReader(raw), names, parms, f.tracker, int64(len(raw)), decodedLimit)
}

func (f *File) filterSpec(ctx context.Context, d Dict) ([]string, []filter.Params, error) {
	var names []string
	switch v := d.Get("Filter").(type) {
	case Name:
		names = []string{string(v)}
	case Array:
		for _, o := range v {
			if n, ok := o.(Name); ok {
				names = append(names, string(n))
			}
		}
	case Null:
		return nil, nil, nil
	default:
		return nil, nil, errs.Wrap(errs.ErrCorrupt, "FLT-FILTER-TYPE", "read filter spec", -1, 0, -1)
	}
	parms := make([]filter.Params, len(names))
	var pv Object = d.Get("DecodeParms")
	if _, ok := pv.(Null); ok {
		pv = d.Get("DP")
	}
	switch v := pv.(type) {
	case Dict:
		if len(parms) > 0 {
			parms[0] = toParams(v)
		}
	case Array:
		for i := range parms {
			if i >= len(v) {
				break
			}
			switch e := v[i].(type) {
			case Dict:
				parms[i] = toParams(e)
			case Ref:
				o, err := f.resolveLocked(ctx, e)
				if err != nil {
					return nil, nil, err
				}
				if dd, ok := o.(Dict); ok {
					parms[i] = toParams(dd)
				}
			}
		}
	}
	return names, parms, nil
}

func toParams(d Dict) filter.Params {
	p := filter.Params{}
	for k, v := range d {
		key := strings.ToLower(string(k))
		switch x := v.(type) {
		case Int:
			p[key] = int64(x)
		case Real:
			p[key] = float64(x)
		case Bool:
			p[key] = bool(x)
		case Name:
			p[key] = string(x)
		}
	}
	return p
}

func (f *File) RootRef() (Ref, bool) {
	r, ok := f.trailer.RefValue("Root")
	return r, ok
}

func (f *File) ResolveRoot(ctx context.Context) (Dict, error) {
	v, ok := f.trailer["Root"]
	if !ok {
		return nil, errs.Wrap(errs.ErrCorrupt, "DOC-NO-ROOT", "resolve catalog", -1, 0, -1)
	}
	o, err := f.Resolve(ctx, v)
	if err != nil {
		return nil, err
	}
	d, ok := o.(Dict)
	if !ok {
		return nil, errs.Wrap(errs.ErrCorrupt, "DOC-ROOT-TYPE", "resolve catalog", -1, 0, -1)
	}
	return d, nil
}

func (f *File) EncryptDict(ctx context.Context) (Dict, bool, error) {
	v, ok := f.trailer["Encrypt"]
	if !ok {
		return nil, false, nil
	}
	o, err := f.Resolve(ctx, v)
	if err != nil {
		return nil, true, err
	}
	d, ok := o.(Dict)
	if !ok {
		return nil, true, errs.Wrap(errs.ErrCorrupt, "CRY-ENCRYPT-TYPE", "read encrypt dictionary", -1, 0, -1)
	}
	return d, true, nil
}

func (f *File) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	f.cache = map[Ref]*list.Element{}
	f.lru = list.New()
	f.objStm = map[Ref]Object{}
	f.repaired = map[Ref]Object{}
	return nil
}

func (f *File) Data() []byte { return f.data }

// ParseIndirectAt is used by repair and tests.
func (f *File) ParseIndirectAt(ctx context.Context, offset int64, lenient bool) (Ref, Object, error) {
	return f.parseIndirectAt(ctx, offset, lenient)
}

func (f *File) parseIndirectAt(ctx context.Context, offset int64, lenient bool) (Ref, Object, error) {
	if offset < 0 || offset >= int64(len(f.data)) {
		return Ref{}, nil, errs.Wrap(errs.ErrCorrupt, "SYN-OBJ-OFFSET", "parse indirect object", offset, 0, -1)
	}
	p := newParser(f.data, int(offset), lenient, f.tracker.Budget().MaxNestingDepth)
	p.skipSpace()
	a, _, err := p.parseNumber()
	if err != nil {
		return Ref{}, nil, err
	}
	p.skipSpace()
	b, _, err := p.parseNumber()
	if err != nil {
		return Ref{}, nil, err
	}
	num, ok1 := a.(Int)
	gen, ok2 := b.(Int)
	if !ok1 || !ok2 {
		return Ref{}, nil, errs.Wrap(errs.ErrCorrupt, "SYN-OBJ-HEADER", "parse indirect object", offset, 0, -1)
	}
	if err := p.expectKeyword("obj"); err != nil {
		return Ref{}, nil, err
	}
	obj, err := p.parseObject()
	if err != nil {
		return Ref{}, nil, err
	}
	ref := Ref{Num: int(num), Gen: int(gen)}
	if d, ok := obj.(Dict); ok {
		save := p.pos
		p.skipSpace()
		if p.hasKeyword("stream") {
			p.pos += len("stream")
			if p.pos < len(p.data) && p.data[p.pos] == '\r' {
				p.pos++
				if p.pos < len(p.data) && p.data[p.pos] == '\n' {
					p.pos++
				}
			} else if p.pos < len(p.data) && p.data[p.pos] == '\n' {
				p.pos++
			}
			start := p.pos
			length, haveLen := f.streamLength(ctx, d)
			if !haveLen || length < 0 || start+int(length) > len(p.data) {
				if !lenient {
					return Ref{}, nil, errs.Wrap(errs.ErrCorrupt, "SYN-LEN-MISMATCH", "read stream", int64(start), ref.Num, -1)
				}
				idx := bytes.Index(p.data[start:], []byte("endstream"))
				if idx < 0 {
					return Ref{}, nil, errs.Wrap(errs.ErrCorrupt, "SYN-STREAM-EOF", "read stream", int64(start), ref.Num, -1)
				}
				length = int64(idx)
				for length > 0 && (p.data[start+int(length)-1] == '\r' || p.data[start+int(length)-1] == '\n') {
					length--
				}
			}
			raw := p.data[start : start+int(length)]
			p.pos = start + int(length)
			p.skipSpace()
			if !p.hasKeyword("endstream") {
				if !lenient {
					return Ref{}, nil, errs.Wrap(errs.ErrCorrupt, "SYN-ENDSTREAM", "read stream", int64(p.pos), ref.Num, -1)
				}
				idx := bytes.Index(p.data[p.pos:], []byte("endstream"))
				if idx >= 0 {
					p.pos += idx
				}
			}
			if p.hasKeyword("endstream") {
				p.pos += len("endstream")
			}
			obj = &Stream{Dict: d, Raw: raw, Offset: offset, Ref: ref}
		} else {
			p.pos = save
		}
	}
	p.skipSpace()
	if p.hasKeyword("endobj") {
		p.pos += len("endobj")
	}
	if f.crypt != nil && !(f.encryptSet && ref == f.encryptRef) {
		obj = f.decryptObject(obj, ref)
	}
	return ref, obj, nil
}

func (f *File) streamLength(ctx context.Context, d Dict) (int64, bool) {
	v, ok := d["Length"]
	if !ok {
		return 0, false
	}
	if r, ok := v.(Ref); ok {
		o, err := f.resolveLocked(ctx, r)
		if err != nil {
			return 0, false
		}
		v = o
	}
	n, ok := AsInt(v)
	return n, ok
}

var objPattern = regexp.MustCompile(`(?m)(\d{1,10})[ \t\r\n\f\x00]+(\d{1,5})[ \t\r\n\f\x00]+obj\b`)

func (f *File) repair(ctx context.Context) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = errs.Wrap(errs.ErrCorrupt, "SYN-REPAIR-PANIC", "rebuild xref", -1, 0, -1)
		}
	}()
	count := 0
	for _, m := range objPattern.FindAllSubmatchIndex(f.data, -1) {
		if err := ctx.Err(); err != nil {
			return errs.Wrap(errs.ErrCanceled, "SYN-CANCELED", "rebuild xref", -1, 0, -1)
		}
		offset := m[0]
		ref, obj, err := f.parseIndirectAt(ctx, int64(offset), true)
		if err != nil {
			f.diag.Add(diag.Diagnostic{Level: diag.Warn, Code: "SYN-REPAIR-OBJ", Msg: "skipped unreadable object", Page: -1, Offset: int64(offset)})
			continue
		}
		f.repaired[ref] = obj
		f.xref[ref.Num] = XRefEntry{Type: EntryInUse, Offset: int64(offset), Gen: ref.Gen}
		count++
	}
	if count == 0 {
		return errs.Wrap(errs.ErrCorrupt, "SYN-REPAIR-EMPTY", "rebuild xref", -1, 0, -1)
	}
	for num, e := range f.xref {
		o, ok := f.repaired[Ref{Num: num, Gen: e.Gen}]
		if !ok {
			continue
		}
		if stm, ok := o.(*Stream); ok {
			if typ, _ := stm.Dict.NameValue("Type"); typ == "ObjStm" {
				objs, oerr := f.objStmObjects(ctx, stm)
				if oerr == nil {
					for r, oo := range objs {
						if _, exists := f.repaired[r]; !exists {
							f.repaired[r] = oo
						}
					}
				}
			}
		}
	}
	f.recoverTrailer(ctx)
	if r, ok := f.trailer["Root"].(Ref); ok {
		if _, exists := f.repaired[r]; !exists {
			delete(f.trailer, "Root")
		}
	}
	if _, ok := f.trailer["Root"]; !ok {
		for ref, o := range f.repaired {
			if d, ok := o.(Dict); ok {
				if t, _ := d.NameValue("Type"); t == "Catalog" {
					f.trailer["Root"] = ref
					break
				}
			}
		}
	}
	if _, ok := f.trailer["Root"]; !ok {
		return errs.Wrap(errs.ErrCorrupt, "DOC-NO-ROOT", "rebuild xref", -1, 0, -1)
	}
	f.repairedFlag = true
	f.diag.Add(diag.Diagnostic{Level: diag.Warn, Code: "SYN-REPAIRED", Msg: "document rebuilt by scanning " + strconv.Itoa(count) + " objects", Page: -1, Offset: -1})
	return nil
}

func (f *File) recoverTrailer(ctx context.Context) {
	for idx := 0; ; {
		i := bytes.Index(f.data[idx:], []byte("trailer"))
		if i < 0 {
			break
		}
		pos := idx + i + len("trailer")
		p := newParser(f.data, pos, true, 64)
		p.skipSpace()
		if o, err := p.parseObject(); err == nil {
			if d, ok := o.(Dict); ok {
				f.mergeTrailerFromRepair(d)
			}
		}
		idx = pos
	}
	for _, o := range f.repaired {
		d, ok := o.(Dict)
		if !ok {
			if stm, ok := o.(*Stream); ok {
				d = stm.Dict
			} else {
				continue
			}
		}
		typ, _ := d.NameValue("Type")
		if typ == "XRef" || typ == "Catalog" {
			f.mergeTrailerFromRepair(d)
		}
	}
}

func (f *File) mergeTrailerFromRepair(d Dict) {
	for _, k := range []string{"Root", "Info", "Encrypt", "ID", "Size"} {
		if v, ok := d[Name(k)]; ok {
			if _, exists := f.trailer[Name(k)]; !exists || k == "Root" {
				f.trailer[Name(k)] = v
			}
		}
	}
}

// Rebuild discards the current xref view and reconstructs it by scanning the
// file. It is used by the document layer when a later stage fails in repair
// mode.
func (f *File) Rebuild(ctx context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return errs.Wrap(errs.ErrClosed, "SYN-CLOSED", "rebuild xref", -1, 0, -1)
	}
	f.resetForRepair()
	f.repairedFlag = false
	return f.repair(ctx)
}

// DecodeStream exposes stream decoding to the document layer.
func (f *File) DecodeStream(ctx context.Context, stm *Stream) ([]byte, error) {
	return f.DecodeStreamLimit(ctx, stm, 0)
}

// DecodeStreamLimit decodes a stream with an optional decoded byte limit. A
// positive limit is used instead of the generic inflate-ratio guard.
func (f *File) DecodeStreamLimit(ctx context.Context, stm *Stream, decodedLimit int64) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return nil, errs.Wrap(errs.ErrClosed, "SYN-CLOSED", "decode stream", stm.Offset, 0, -1)
	}
	return f.decodeStream(ctx, stm, decodedLimit)
}

// DecodeStreamTo decodes a stream directly into w without materializing the
// decoded bytes. The encoded stream itself is still read from the in-memory or
// mapped file view.
func (f *File) DecodeStreamTo(ctx context.Context, stm *Stream, w io.Writer) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return 0, errs.Wrap(errs.ErrClosed, "SYN-CLOSED", "decode stream", stm.Offset, 0, -1)
	}
	r, err := f.streamReader(ctx, stm, 0)
	if err != nil {
		return 0, err
	}
	n, err := io.Copy(w, r)
	if err != nil {
		return n, errs.Wrap(err, "FLT-DECODE", "read decoded stream", stm.Offset, 0, -1)
	}
	return n, nil
}

func (f *File) initSecurity(ctx context.Context, password string) error {
	v, ok := f.trailer["Encrypt"]
	if !ok {
		return nil
	}
	if r, ok := v.(Ref); ok {
		f.encryptRef, f.encryptSet = r, true
	}
	obj, err := f.Resolve(ctx, v)
	if err != nil {
		return err
	}
	dict, ok := obj.(Dict)
	if !ok {
		return errs.Wrap(errs.ErrCorrupt, "CRY-ENCRYPT-TYPE", "read encrypt dictionary", -1, 0, -1)
	}
	filter, _ := dict.NameValue("Filter")
	if filter != "Standard" {
		return errs.Wrap(errs.ErrUnsupported, "CRY-PUBFILTER", "unsupported security handler", -1, 0, -1)
	}
	vNum, _ := dict.IntValue("V")
	rNum, _ := dict.IntValue("R")
	length, _ := dict.IntValue("Length")
	pNum, _ := dict.IntValue("P")
	oObj, _ := dict.Get("O").(String)
	uObj, _ := dict.Get("U").(String)
	oeObj, _ := dict.Get("OE").(String)
	ueObj, _ := dict.Get("UE").(String)
	permsObj, _ := dict.Get("Perms").(String)
	encMeta := true
	if b, ok := dict.Get("EncryptMetadata").(Bool); ok {
		encMeta = bool(b)
	}
	var id []byte
	if arr, ok := f.trailer.Get("ID").(Array); ok && len(arr) > 0 {
		if s, ok := arr[0].(String); ok {
			id = []byte(s)
		}
	}
	filters := map[string]crypt.Filter{}
	if cf, ok := dict.Get("CF").(Dict); ok {
		for name, raw := range cf {
			fd, ok := raw.(Dict)
			if !ok {
				continue
			}
			cfm, _ := fd.NameValue("CFM")
			flen, _ := fd.IntValue("Length")
			filters[string(name)] = crypt.Filter{CFM: cfm, Length: int(flen)}
		}
	}
	stmF, _ := dict.NameValue("StmF")
	strF, _ := dict.NameValue("StrF")
	h, err := crypt.New(crypt.Options{
		V: int(vNum), R: int(rNum), Length: int(length), P: int(pNum),
		O: []byte(oObj), U: []byte(uObj), OE: []byte(oeObj), UE: []byte(ueObj),
		Perms: []byte(permsObj), ID: id, EncryptMetadata: encMeta,
		StmF: stmF, StrF: strF, Filters: filters,
	}, password)
	if err != nil {
		return err
	}
	f.crypt = h
	return nil
}

func (f *File) decryptObject(obj Object, ref Ref) Object {
	switch v := obj.(type) {
	case String:
		b, err := f.crypt.DecryptString(ref.Num, ref.Gen, []byte(v))
		if err != nil {
			return v
		}
		return String(b)
	case Array:
		for i := range v {
			v[i] = f.decryptObject(v[i], ref)
		}
		return v
	case Dict:
		for k, val := range v {
			v[k] = f.decryptObject(val, ref)
		}
		return v
	case *Stream:
		if v.Dict != nil {
			v.Dict = f.decryptObject(v.Dict, ref).(Dict)
		}
		v.Ref = ref
		return v
	default:
		return obj
	}
}
