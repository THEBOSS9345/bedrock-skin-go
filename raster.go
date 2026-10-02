package bedrockskin

import (
	"image"
	"image/color"
	"math"

	"github.com/fogleman/fauxgl"
)

// raster is fauxgl's triangle stage specialised to the one shader the
// renderer uses: nearest-neighbour texture sampling with an alpha test, depth
// tested, blended where a fragment is not fully opaque.
//
// fauxgl's general rasterizer interpolates every vertex attribute for every
// pixel - position, normal, colour, clip position - calls the shader through
// an interface, and takes a mutex per pixel for its parallel mode. This one
// interpolates the texture coordinate alone and has no locks, which made a
// head render three times faster. Every operation on the values it keeps is
// fauxgl's, in fauxgl's order, so the image is bit-identical: the golden and
// parity fixtures check that. The vertex stage and clipping still run
// through fauxgl. See docs/design-decisions.md#why-the-rasterizer-is-specialised.
type raster struct {
	width, height int
	color         *image.NRGBA
	depth         []float64
	screen        fauxgl.Matrix
}

func newRaster(width, height int) *raster {
	r := &raster{
		width:  width,
		height: height,
		color:  image.NewNRGBA(image.Rect(0, 0, width, height)),
		depth:  make([]float64, width*height),
		screen: fauxgl.Screen(width, height),
	}
	for i := range r.depth {
		r.depth[i] = math.MaxFloat64
	}
	return r
}

// drawTriangle runs the vertex stage, clips what leaves the view volume, and
// rasterizes. Back faces are drawn too: cube winding is not consistent.
func (r *raster) drawTriangle(t *fauxgl.Triangle, matrix fauxgl.Matrix, tex *fastImageTexture) {
	v1, v2, v3 := t.V1, t.V2, t.V3
	v1.Output = matrix.MulPositionW(v1.Position)
	v2.Output = matrix.MulPositionW(v2.Position)
	v3.Output = matrix.MulPositionW(v3.Position)
	if v1.Outside() || v2.Outside() || v3.Outside() {
		for _, c := range fauxgl.ClipTriangle(fauxgl.NewTriangle(v1, v2, v3)) {
			r.drawClipped(c.V1, c.V2, c.V3, tex)
		}
		return
	}
	r.drawClipped(v1, v2, v3, tex)
}

func (r *raster) drawClipped(v0, v1, v2 fauxgl.Vertex, tex *fastImageTexture) {
	ndc0 := v0.Output.DivScalar(v0.Output.W).Vector()
	ndc1 := v1.Output.DivScalar(v1.Output.W).Vector()
	ndc2 := v2.Output.DivScalar(v2.Output.W).Vector()
	a := (ndc1.X-ndc0.X)*(ndc2.Y-ndc0.Y) - (ndc2.X-ndc0.X)*(ndc1.Y-ndc0.Y)
	if a < 0 {
		v0, v2 = v2, v0
		ndc0, ndc2 = ndc2, ndc0
	}
	s0 := r.screen.MulPosition(ndc0)
	s1 := r.screen.MulPosition(ndc1)
	s2 := r.screen.MulPosition(ndc2)
	r.rasterize(v0, v1, v2, s0, s1, s2, tex)
}

func screenEdge(a, b, c fauxgl.Vector) float64 {
	return (b.X-c.X)*(a.Y-c.Y) - (b.Y-c.Y)*(a.X-c.X)
}

func (r *raster) rasterize(v0, v1, v2 fauxgl.Vertex, s0, s1, s2 fauxgl.Vector, tex *fastImageTexture) {
	lo := s0.Min(s1.Min(s2)).Floor()
	hi := s0.Max(s1.Max(s2)).Ceil()
	x0, x1 := int(lo.X), int(hi.X)
	y0, y1 := int(lo.Y), int(hi.Y)

	p := fauxgl.Vector{X: float64(x0) + 0.5, Y: float64(y0) + 0.5}
	w00 := screenEdge(s1, s2, p)
	w01 := screenEdge(s2, s0, p)
	w02 := screenEdge(s0, s1, p)
	a01 := s1.Y - s0.Y
	b01 := s0.X - s1.X
	a12 := s2.Y - s1.Y
	b12 := s1.X - s2.X
	a20 := s0.Y - s2.Y
	b20 := s2.X - s0.X

	ra := 1 / screenEdge(s0, s1, s2)
	r0 := 1 / v0.Output.W
	r1 := 1 / v1.Output.W
	r2 := 1 / v2.Output.W
	ra12 := 1 / a12
	ra20 := 1 / a20
	ra01 := 1 / a01

	t0, t1, t2 := v0.Texture, v1.Texture, v2.Texture
	pix := r.color.Pix
	for y := y0; y <= y1; y++ {
		var d float64
		d0 := -w00 * ra12
		d1 := -w01 * ra20
		d2 := -w02 * ra01
		if w00 < 0 && d0 > d {
			d = d0
		}
		if w01 < 0 && d1 > d {
			d = d1
		}
		if w02 < 0 && d2 > d {
			d = d2
		}
		d = float64(int(d))
		if d < 0 {
			d = 0
		}
		w0 := w00 + a12*d
		w1 := w01 + a20*d
		w2 := w02 + a01*d
		wasInside := false
		for x := x0 + int(d); x <= x1; x++ {
			b0 := w0 * ra
			b1 := w1 * ra
			b2 := w2 * ra
			w0 += a12
			w1 += a20
			w2 += a01
			if b0 < 0 || b1 < 0 || b2 < 0 {
				if wasInside {
					break
				}
				continue
			}
			wasInside = true
			i := y*r.width + x
			if i < 0 || i >= len(r.depth) {
				continue
			}
			z := b0*s0.Z + b1*s1.Z + b2*s2.Z
			bz := z + 0 // fauxgl adds its depth bias, zero
			if bz > r.depth[i] {
				continue
			}
			// Perspective-correct texture coordinate, as fauxgl's
			// InterpolateVectors: ((0 + t0*bx) + t1*by + t2*bz) * bw. The
			// conversions stop the multiplies fusing into the adds.
			bx, by, bzw := b0*r0, b1*r1, b2*r2
			bw := 1 / (bx + by + bzw)
			u := float64(float64(float64(0+float64(t0.X*bx))+float64(t1.X*by))+float64(t2.X*bzw)) * bw
			v := float64(float64(float64(0+float64(t0.Y*bx))+float64(t1.Y*by))+float64(t2.Y*bzw)) * bw
			// The texel's bytes stand in for fauxgl's Color: every byte
			// survives byte/255*255 exactly, alpha/255 < 0.5 is alpha < 128,
			// and alpha/255 < 1 is alpha < 255.
			k := tex.offset(u, v)
			c := tex.pix[k : k+4 : k+4]
			if c[3] < 128 {
				continue
			}
			// Written as fauxgl writes it, not as bz > depth: a NaN depth
			// passes the early test above and must fail this one.
			if !(bz <= r.depth[i]) {
				continue
			}
			r.depth[i] = z
			// fauxgl blends at the pixel offset unchecked, but stores an
			// opaque fragment with SetNRGBA, which ignores a point off the
			// image; both are kept.
			j := i * 4
			if c[3] < 255 {
				sr, sg, sb, sa := color.NRGBA{R: c[0], G: c[1], B: c[2], A: c[3]}.RGBA()
				a := (0xffff - sa) * 0x101
				pix[j+0] = uint8((uint32(pix[j+0])*a/0xffff + sr) >> 8)
				pix[j+1] = uint8((uint32(pix[j+1])*a/0xffff + sg) >> 8)
				pix[j+2] = uint8((uint32(pix[j+2])*a/0xffff + sb) >> 8)
				pix[j+3] = uint8((uint32(pix[j+3])*a/0xffff + sa) >> 8)
			} else if x >= 0 && x < r.width && y >= 0 && y < r.height {
				copy(pix[j:j+4], c)
			}
		}
		w00 += b12
		w01 += b20
		w02 += b01
	}
}
