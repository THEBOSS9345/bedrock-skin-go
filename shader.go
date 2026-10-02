package bedrockskin

import (
	"image"
	"image/color"
	"math"

	"github.com/fogleman/fauxgl"
)

// fastImageTexture is an allocation-free texture for the image types a
// skin actually is (*image.NRGBA in every path: DecodeImage normally yields
// it and TextureFromRGBA always does). fauxgl's stock ImageTexture calls
// image.Image.At once per sampled pixel, which boxes a fresh color value into
// an interface for every fragment - that single interface allocation is over
// 90% of all allocations in a render. Sampling the raw Pix slice directly
// removes it completely.
//
// Every skin the library decodes is *image.NRGBA (or TextureFromRGBA's NRGBA),
// so the common path aliases Pix with zero copies and zero per-pixel
// allocation. Any other image type is converted once, up front, into the same
// packed RGBA layout; sampling then never touches the generic image
// interface either.
type fastImageTexture struct {
	width, height int
	pix           []uint8 // packed R,G,B,A per pixel, straight (unpremultiplied) alpha
}

func newFastImageTexture(im image.Image) *fastImageTexture {
	if n, ok := im.(*image.NRGBA); ok {
		b := n.Bounds()
		if n.Stride == b.Dx()*4 && b.Min.X == 0 && b.Min.Y == 0 {
			return &fastImageTexture{width: b.Dx(), height: b.Dy(), pix: n.Pix}
		}
	}
	b := im.Bounds()
	w, h := b.Dx(), b.Dy()
	pix := make([]uint8, 0, w*h*4)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			// Straight alpha, as the sampler reads it. RGBA() would give
			// premultiplied colour, darkening every half-transparent pixel
			// of a palette PNG - the form most stored skins take.
			c := color.NRGBAModel.Convert(im.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA)
			pix = append(pix, c.R, c.G, c.B, c.A)
		}
	}
	return &fastImageTexture{width: w, height: h, pix: pix}
}

// Sample replicates fauxgl.ImageTexture.Sample's coordinate handling exactly -
// including its internal v=1-v, which cancels the V flip applied in mesh.go.
// The alpha is straight and each channel is byte/255, matching what
// MakeColor(At(...).RGBA()) produces for the byte values of NRGBA.
func (t *fastImageTexture) Sample(u, v float64) fauxgl.Color {
	v = 1 - v
	u -= math.Floor(u)
	v -= math.Floor(v)
	x := int(u * float64(t.width))
	y := int(v * float64(t.height))
	i := (y*t.width + x) * 4
	p := t.pix
	return fauxgl.Color{
		R: float64(p[i]) / 255,
		G: float64(p[i+1]) / 255,
		B: float64(p[i+2]) / 255,
		A: float64(p[i+3]) / 255,
	}
}

// offset is Sample's texel, as a byte offset into pix: the same coordinate
// handling, without building a Color. For u and v already in [0, 1) the
// floors are skipped - u - Floor(u) is u there, bar -0 becoming +0, which
// lands on the same texel.
func (t *fastImageTexture) offset(u, v float64) int {
	v = 1 - v
	if !(u >= 0 && u < 1) {
		u -= math.Floor(u)
	}
	if !(v >= 0 && v < 1) {
		v -= math.Floor(v)
	}
	x := int(u * float64(t.width))
	y := int(v * float64(t.height))
	return (y*t.width + x) * 4
}

// alphaThreshold is the alpha test: fragments below half opacity are
// discarded, colour and depth both. Discarding rather than blending is what
// lets the inflated overlay layer (hat, jacket, sleeves, pants) show the body
// underneath. Sampling is nearest-neighbour, not bilinear: atlas regions are
// packed edge to edge with no padding.
//
// See docs/rendering-pipeline.md#alpha-testing.
const alphaThreshold = 0.5
