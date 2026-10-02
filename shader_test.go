package bedrockskin

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/fogleman/fauxgl"
)

// Verifies fastImageTexture.Sample matches fauxgl.ImageTexture.Sample exactly
// across the UV domain for an *image.NRGBA, so the orientation flip and
// coordinate handling produce identical output to the stock texture.
func TestFastTextureMatchesFauxgl(t *testing.T) {
	tex := makeTexture(255)
	for i := 0; i < len(tex.Pix); i += 4 {
		tex.Pix[i] = uint8(i * 7 % 255)
		tex.Pix[i+1] = uint8(i * 3 % 255)
		tex.Pix[i+2] = uint8(i * 11 % 255)
	}
	fast := newFastImageTexture(tex)
	old := fauxgl.NewImageTexture(tex)
	for i := 0; i < 5000; i++ {
		u := math.Mod(float64(i)*0.731, 2.0)
		v := math.Mod(float64(i)*0.337, 2.0)
		a := fast.Sample(u, v)
		b := old.Sample(u, v)
		if math.Abs(a.R-b.R) > 1e-6 || math.Abs(a.G-b.G) > 1e-6 ||
			math.Abs(a.B-b.B) > 1e-6 || math.Abs(a.A-b.A) > 1e-6 {
			t.Fatalf("mismatch at u=%f v=%f: fast=%+v old=%+v", u, v, a, b)
		}
	}
}

// A render through the fast texture must be free of the per-fragment
// allocations that dominated the old path. Loose bound: well under a tenth of
// the previous ~79k allocations at 512x512.
func TestFastTextureRenderFewerAllocs(t *testing.T) {
	tex := makeTexture(255)
	before := testing.AllocsPerRun(5, func() {
		if _, err := Render(Options{Texture: tex, Size: 512}); err != nil {
			t.Fatal(err)
		}
	})
	if before > 15000 {
		t.Errorf("Render still allocates %d times per run; fast texture not effective", int(before))
	}
	t.Logf("allocations per 512 render: %d", int(before))
}

// A palette PNG - how most stored skins are saved - must render exactly as
// the same pixels in straight RGBA. Its half-transparent pixels used to be
// sampled premultiplied, which drew them darker; skincheck in the Rust
// version found it on real skins.
func TestPalettedTextureMatchesNRGBA(t *testing.T) {
	pal := color.Palette{color.NRGBA{}, color.NRGBA{R: 219, G: 161, B: 75, A: 219}, color.NRGBA{R: 40, G: 90, B: 200, A: 255}, color.NRGBA{R: 250, G: 250, B: 10, A: 140}}
	paletted := image.NewPaletted(image.Rect(0, 0, 64, 64), pal)
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			paletted.SetColorIndex(x, y, uint8(1+(x+y)%3))
		}
	}
	raw, err := EncodePNG(paletted)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeImage(raw)
	if err != nil {
		t.Fatal(err)
	}
	straight, ok := decoded.(*image.NRGBA)
	if !ok {
		t.Fatalf("DecodeImage gave %T, want *image.NRGBA", decoded)
	}
	if got := straight.NRGBAAt(0, 0); got != pal[1] {
		t.Fatalf("decoded pixel %v, want %v", got, pal[1])
	}
	for _, view := range []View{ViewBody, ViewHead} {
		a, _ := Render(Options{Texture: paletted, View: view, Angle: AngleIso, Size: 64})
		b, _ := Render(Options{Texture: straight, View: view, Angle: AngleIso, Size: 64})
		if diff := compareImages(a, b); diff != "" {
			t.Errorf("%s: palette texture renders differently: %s", view, diff)
		}
	}
}
