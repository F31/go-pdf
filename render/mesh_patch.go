package render

import (
	"context"

	"github.com/F31/go-pdf/displaylist"
	pdfcolor "github.com/F31/go-pdf/internal/color"
	"github.com/F31/go-pdf/internal/syntax"
)

type meshVertex struct {
	p displaylist.Point
	c [4]float64
}

func (rr *regionRenderer) patchMesh(s *displaylist.Shading) error {
	if (s.MeshType != 6 && s.MeshType != 7) || s.BitsPerCoordinate <= 0 || s.BitsPerComponent <= 0 || s.BitsPerFlag <= 0 || len(s.Raw) == 0 {
		return nil
	}
	space, err := pdfcolor.Load(context.Background(), nil, syntax.Name(s.ColorSpace), nil)
	if err != nil {
		return nil
	}
	ncomp := space.Components()
	if ncomp <= 0 {
		return nil
	}
	decode := s.Decode
	need := 2 * (2 + ncomp)
	if len(decode) < need {
		decode = make([]float64, need)
		for i := 0; i < 2; i++ {
			decode[2*i], decode[2*i+1] = 0, 1
		}
		for i := 0; i < ncomp; i++ {
			decode[4+2*i], decode[5+2*i] = 0, 1
		}
	}
	br := &bitReader{data: s.Raw}
	maxc := float64(uint64(1)<<uint(s.BitsPerCoordinate) - 1)
	maxg := float64(uint64(1)<<uint(s.BitsPerComponent) - 1)
	if s.BitsPerCoordinate == 32 {
		maxc = 4294967295
	}
	if s.BitsPerComponent == 32 {
		maxg = 4294967295
	}
	readCoord := func() (displaylist.Point, bool) {
		xr := float64(br.read(s.BitsPerCoordinate)) / maxc
		yr := float64(br.read(s.BitsPerCoordinate)) / maxc
		if br.err {
			return displaylist.Point{}, false
		}
		x := decode[0] + xr*(decode[1]-decode[0])
		y := decode[2] + yr*(decode[3]-decode[2])
		return apply(mul(rr.pageToPx, s.Matrix), x, y), true
	}
	readColor := func() ([4]float64, bool) {
		comps := make([]float64, ncomp)
		for j := 0; j < ncomp; j++ {
			raw := float64(br.read(s.BitsPerComponent)) / maxg
			comps[j] = decode[4+2*j] + raw*(decode[5+2*j]-decode[4+2*j])
		}
		rgba, _, err := space.RGBA(comps)
		if err != nil {
			return [4]float64{}, false
		}
		return rgba, true
	}

	var verts []meshVertex
	var cols [][4]float64
	var ps [16]int
	var cs [4]int
	clip := rr.clipMask(s.Clip)
	for br.bit+8*s.BitsPerCoordinate+2*s.BitsPerComponent <= len(br.data)*8 {
		f := int(br.read(s.BitsPerFlag))
		if br.err || f < 0 || f > 3 {
			return nil
		}
		nv, nc := 12, 4
		if s.MeshType == 7 {
			nv = 16
		}
		if f != 0 {
			nv -= 4
			nc = 2
		}
		baseP, baseC := len(verts), len(cols)
		for i := 0; i < nv; i++ {
			p, ok := readCoord()
			if !ok {
				return nil
			}
			verts = append(verts, meshVertex{p: p})
		}
		for i := 0; i < nc; i++ {
			c, ok := readColor()
			if !ok {
				return nil
			}
			cols = append(cols, c)
		}
		oldPS, oldCS := ps, cs
		switch s.MeshType {
		case 6:
			switch f {
			case 0:
				ps[12], ps[13], ps[14], ps[15] = baseP+3, baseP+4, baseP+5, baseP+6
				ps[8], ps[11] = baseP+2, baseP+7
				ps[4], ps[7] = baseP+1, baseP+8
				ps[0], ps[1], ps[2], ps[3] = baseP, baseP+11, baseP+10, baseP+9
				cs[2], cs[3], cs[0], cs[1] = baseC+1, baseC+2, baseC, baseC+3
			case 1:
				ps[12], ps[13], ps[14], ps[15] = oldPS[15], baseP, baseP+1, baseP+2
				ps[8], ps[11] = oldPS[14], baseP+3
				ps[4], ps[7] = oldPS[13], baseP+4
				ps[0], ps[1], ps[2], ps[3] = oldPS[12], baseP+7, baseP+6, baseP+5
				cs[2], cs[3], cs[0], cs[1] = oldCS[3], baseC, oldCS[2], baseC+1
			case 2:
				ps[12], ps[13], ps[14], ps[15] = oldPS[3], baseP, baseP+1, baseP+2
				ps[8], ps[11] = oldPS[7], baseP+3
				ps[4], ps[7] = oldPS[11], baseP+4
				ps[0], ps[1], ps[2], ps[3] = oldPS[15], baseP+7, baseP+6, baseP+5
				cs[2], cs[3], cs[0], cs[1] = oldCS[1], baseC, oldCS[3], baseC+1
			case 3:
				ps[12], ps[13], ps[14], ps[15] = oldPS[0], baseP, baseP+1, baseP+2
				ps[8], ps[11] = oldPS[1], baseP+3
				ps[4], ps[7] = oldPS[2], baseP+4
				ps[0], ps[1], ps[2], ps[3] = oldPS[3], baseP+7, baseP+6, baseP+5
				cs[2], cs[3], cs[0], cs[1] = oldCS[0], baseC, oldCS[1], baseC+1
			}
			p := func(i int) displaylist.Point { return verts[ps[i]].p }
			add := func(idx int, pt displaylist.Point) {
				verts = append(verts, meshVertex{p: pt})
				ps[idx] = len(verts) - 1
			}
			add(5, displaylist.Point{
				X: (-4*p(0).X - p(15).X + 6*(p(4).X+p(1).X) - 2*(p(12).X+p(3).X) + 3*(p(13).X+p(7).X)) / 9,
				Y: (-4*p(0).Y - p(15).Y + 6*(p(4).Y+p(1).Y) - 2*(p(12).Y+p(3).Y) + 3*(p(13).Y+p(7).Y)) / 9,
			})
			add(6, displaylist.Point{
				X: (-4*p(3).X - p(12).X + 6*(p(2).X+p(7).X) - 2*(p(0).X+p(15).X) + 3*(p(4).X+p(14).X)) / 9,
				Y: (-4*p(3).Y - p(12).Y + 6*(p(2).Y+p(7).Y) - 2*(p(0).Y+p(15).Y) + 3*(p(4).Y+p(14).Y)) / 9,
			})
			add(9, displaylist.Point{
				X: (-4*p(12).X - p(3).X + 6*(p(8).X+p(13).X) - 2*(p(0).X+p(15).X) + 3*(p(11).X+p(1).X)) / 9,
				Y: (-4*p(12).Y - p(3).Y + 6*(p(8).Y+p(13).Y) - 2*(p(0).Y+p(15).Y) + 3*(p(11).Y+p(1).Y)) / 9,
			})
			add(10, displaylist.Point{
				X: (-4*p(15).X - p(0).X + 6*(p(11).X+p(14).X) - 2*(p(12).X+p(3).X) + 3*(p(2).X+p(8).X)) / 9,
				Y: (-4*p(15).Y - p(0).Y + 6*(p(11).Y+p(14).Y) - 2*(p(12).Y+p(3).Y) + 3*(p(2).Y+p(8).Y)) / 9,
			})
		case 7:
			switch f {
			case 0:
				ps[12], ps[13], ps[14], ps[15] = baseP+3, baseP+4, baseP+5, baseP+6
				ps[8], ps[9], ps[10], ps[11] = baseP+2, baseP+13, baseP+14, baseP+7
				ps[4], ps[5], ps[6], ps[7] = baseP+1, baseP+12, baseP+15, baseP+8
				ps[0], ps[1], ps[2], ps[3] = baseP, baseP+11, baseP+10, baseP+9
				cs[2], cs[3], cs[0], cs[1] = baseC+1, baseC+2, baseC, baseC+3
			case 1:
				ps[12], ps[13], ps[14], ps[15] = oldPS[15], baseP, baseP+1, baseP+2
				ps[8], ps[9], ps[10], ps[11] = oldPS[14], baseP+9, baseP+10, baseP+3
				ps[4], ps[5], ps[6], ps[7] = oldPS[13], baseP+8, baseP+11, baseP+4
				ps[0], ps[1], ps[2], ps[3] = oldPS[12], baseP+7, baseP+6, baseP+5
				cs[2], cs[3], cs[0], cs[1] = oldCS[3], baseC, oldCS[2], baseC+1
			case 2:
				ps[12], ps[13], ps[14], ps[15] = oldPS[3], baseP, baseP+1, baseP+2
				ps[8], ps[9], ps[10], ps[11] = oldPS[7], baseP+9, baseP+10, baseP+3
				ps[4], ps[5], ps[6], ps[7] = oldPS[11], baseP+8, baseP+11, baseP+4
				ps[0], ps[1], ps[2], ps[3] = oldPS[15], baseP+7, baseP+6, baseP+5
				cs[2], cs[3], cs[0], cs[1] = oldCS[1], baseC, oldCS[3], baseC+1
			case 3:
				ps[12], ps[13], ps[14], ps[15] = oldPS[0], baseP, baseP+1, baseP+2
				ps[8], ps[9], ps[10], ps[11] = oldPS[1], baseP+9, baseP+10, baseP+3
				ps[4], ps[5], ps[6], ps[7] = oldPS[2], baseP+8, baseP+11, baseP+4
				ps[0], ps[1], ps[2], ps[3] = oldPS[3], baseP+7, baseP+6, baseP+5
				cs[2], cs[3], cs[0], cs[1] = oldCS[0], baseC, oldCS[1], baseC+1
			}
		}
		for i := 0; i < 16; i++ {
			if ps[i] < 0 || ps[i] >= len(verts) {
				return nil
			}
		}
		for i := 0; i < 4; i++ {
			if cs[i] < 0 || cs[i] >= len(cols) {
				return nil
			}
		}
		grid := [16]meshVertex{}
		for i := 0; i < 16; i++ {
			grid[i] = verts[ps[i]]
		}
		cc := [4][4]float64{cols[cs[0]], cols[cs[1]], cols[cs[2]], cols[cs[3]]}
		const n = 6
		pts := [n + 1][n + 1]meshVertex{}
		for v := 0; v <= n; v++ {
			for u := 0; u <= n; u++ {
				tu, tv := float64(u)/n, float64(v)/n
				bu := bezier(tu)
				bv := bezier(tv)
				var pt displaylist.Point
				for j := 0; j < 4; j++ {
					for i := 0; i < 4; i++ {
						w := bu[i] * bv[j]
						pt.X += grid[j*4+i].p.X * w
						pt.Y += grid[j*4+i].p.Y * w
					}
				}
				c0 := lerpColor(cc[0], cc[1], tu)
				c1 := lerpColor(cc[2], cc[3], tu)
				col := lerpColor(c0, c1, tv)
				pts[v][u] = meshVertex{p: pt, c: col}
			}
		}
		for v := 0; v < n; v++ {
			for u := 0; u < n; u++ {
				a, b, c := pts[v][u], pts[v][u+1], pts[v+1][u]
				d, e := pts[v+1][u+1], pts[v+1][u]
				rr.meshTri(a.p, b.p, c.p, a.c, b.c, c.c, s, clip)
				rr.meshTri(b.p, d.p, e.p, b.c, d.c, e.c, s, clip)
			}
		}
	}
	return nil
}

func bezier(t float64) [4]float64 {
	u := 1 - t
	return [4]float64{u * u * u, 3 * u * u * t, 3 * u * t * t, t * t * t}
}

func lerpColor(a, b [4]float64, t float64) [4]float64 {
	var c [4]float64
	for i := range c {
		c[i] = a[i] + (b[i]-a[i])*t
	}
	return c
}
