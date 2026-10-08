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
// head render three times faster. It departs from fauxgl in two places,
// each fixing a visible defect: edge functions are evaluated exactly per
// pixel rather than stepped, and only the near and far planes clip. See
// docs/design-decisions.md#why-edges-are-not-stepped. Everything else is
// fauxgl's arithmetic in fauxgl's order; the vertex stage and near/far
// clipping still run through fauxgl. See
// docs/design-decisions.md#why-the-rasterizer-is-specialised.
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
	// Only a triangle crossing the near or far plane is clipped. One that
	// merely runs off the image is drawn whole and rasterize keeps to the
	// image: clipping its two halves apart left a gap down a face's
	// diagonal. See docs/design-decisions.md#why-edges-are-not-stepped.
	if depthOutside(v1) || depthOutside(v2) || depthOutside(v3) {
		for _, c := range fauxgl.ClipTriangle(fauxgl.NewTriangle(v1, v2, v3)) {
			r.drawClipped(c.V1, c.V2, c.V3, tex)
		}
		return
	}
	r.drawClipped(v1, v2, v3, tex)
}

// depthOutside reports a vertex beyond the near or far plane, or behind the
// camera.
func depthOutside(v fauxgl.Vertex) bool {
	o := v.Output
	return !(o.W > 0) || o.Z < -o.W || o.Z > o.W
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

	ra := 1 / screenEdge(s0, s1, s2)
	r0 := 1 / v0.Output.W
	r1 := 1 / v1.Output.W
	r2 := 1 / v2.Output.W

	t0, t1, t2 := v0.Texture, v1.Texture, v2.Texture
	pix := r.color.Pix
	// Only pixels on the image: off it there is nothing to draw, and a
	// pixel's index would wrap into the next row.
	x0, x1 = max(x0, 0), min(x1, r.width-1)
	y0, y1 = max(y0, 0), min(y1, r.height-1)
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			// Each pixel's edge functions are worked out afresh. fauxgl
			// steps them along the row, and the rounding that accumulates
			// let thin, edge-on triangles spill slivers past their edges.
			// See docs/design-decisions.md#why-edges-are-not-stepped.
			p := fauxgl.Vector{X: float64(x) + 0.5, Y: float64(y) + 0.5}
			b0 := screenEdge(s1, s2, p) * ra
			b1 := screenEdge(s2, s0, p) * ra
			b2 := screenEdge(s0, s1, p) * ra
			if b0 < 0 || b1 < 0 || b2 < 0 {
				continue
			}
			i := y*r.width + x
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
	}
}
