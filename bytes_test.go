package bedrockskin

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"testing"
)

func testTextureBytes(t *testing.T) []byte {
	t.Helper()
	b, err := EncodePNG(testTexture())
	if err != nil {
		t.Fatalf("EncodePNG: %v", err)
	}
	return b
}

func TestRenderBytesRoundTrip(t *testing.T) {
	out, err := RenderBytes(BytesOptions{
		Texture: testTextureBytes(t),
		View:    ViewAvatar,
		Size:    64,
	})
	if err != nil {
		t.Fatalf("RenderBytes: %v", err)
	}

	// The result must be a real PNG, not just non-empty bytes.
	img, err := png.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("output is not valid PNG: %v", err)
	}
	if img.Bounds().Dx() != 64 || img.Bounds().Dy() != 64 {
		t.Errorf("bounds = %v, want 64x64", img.Bounds())
	}
}

// The bytes path must agree with the image path exactly — it is meant to be
// the same render with decode/encode folded in, not a second implementation.
func TestRenderBytesMatchesRender(t *testing.T) {
	opts := Options{Texture: testTexture(), View: ViewHead, Size: 64}

	viaImages, err := opts.RenderPNG()
	if err != nil {
		t.Fatalf("RenderPNG: %v", err)
	}
	viaBytes, err := RenderBytes(BytesOptions{
		Texture: testTextureBytes(t),
		View:    ViewHead,
		Size:    64,
	})
	if err != nil {
		t.Fatalf("RenderBytes: %v", err)
	}

	if !bytes.Equal(viaImages, viaBytes) {
		t.Error("bytes path and image path produced different output")
	}
}

func TestRenderBytesRequiresTexture(t *testing.T) {
	if _, err := RenderBytes(BytesOptions{}); err != ErrNoTexture {
		t.Errorf("err = %v, want ErrNoTexture", err)
	}
}

// Geometry absent, empty and "null" must all reach the default model, since
// that is exactly what a stock skin's field looks like on the wire.
func TestRenderBytesGeometryFallbacks(t *testing.T) {
	tex := testTextureBytes(t)

	reference, err := RenderBytes(BytesOptions{Texture: tex, Size: 64})
	if err != nil {
		t.Fatalf("baseline: %v", err)
	}

	for _, tc := range []struct {
		name string
		geom []byte
	}{
		{"nil", nil},
		{"empty", []byte{}},
		{"null", []byte("null")},
		{"null with newline", []byte("null\n")},
	} {
		got, err := RenderBytes(BytesOptions{Texture: tex, Geometry: tc.geom, Size: 64})
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		if !bytes.Equal(got, reference) {
			t.Errorf("%s: did not match the default-geometry render", tc.name)
		}
	}
}

// Malformed geometry must still be an error — the fallback is for absent
// geometry, not for hiding a broken upload.
func TestRenderBytesRejectsMalformedGeometry(t *testing.T) {
	_, err := RenderBytes(BytesOptions{
		Texture:  testTextureBytes(t),
		Geometry: []byte("{ nope"),
	})
	if err == nil {
		t.Error("expected an error for malformed geometry")
	}
}

func TestRenderBytesRejectsBadTexture(t *testing.T) {
	_, err := RenderBytes(BytesOptions{Texture: []byte("not an image")})
	if err == nil {
		t.Error("expected an error for a texture that is not an image")
	}
}

func TestBytesOptionsRenderPNGMethod(t *testing.T) {
	out, err := BytesOptions{Texture: testTextureBytes(t), Size: 32}.RenderPNG()
	if err != nil {
		t.Fatalf("RenderPNG: %v", err)
	}
	if _, err := png.Decode(bytes.NewReader(out)); err != nil {
		t.Errorf("output is not valid PNG: %v", err)
	}
}

func TestOptionsRenderMethod(t *testing.T) {
	img, err := Options{Texture: testTexture(), Size: 32}.Render()
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if img.Bounds().Dx() != 32 {
		t.Errorf("width = %d, want 32", img.Bounds().Dx())
	}
}

func TestTextureFromRGBA(t *testing.T) {
	const w, h = 4, 2
	pix := make([]byte, w*h*4)
	for i := range pix {
		pix[i] = byte(i)
	}

	img, err := TextureFromRGBA(pix, w, h)
	if err != nil {
		t.Fatalf("TextureFromRGBA: %v", err)
	}
	if img.Bounds() != image.Rect(0, 0, w, h) {
		t.Errorf("bounds = %v", img.Bounds())
	}
	// The slice must back the image rather than being copied.
	if nrgba, ok := img.(*image.NRGBA); !ok || &nrgba.Pix[0] != &pix[0] {
		t.Error("pixel data was copied instead of shared")
	}
}

func TestTextureFromRGBARejectsWrongLength(t *testing.T) {
	if _, err := TextureFromRGBA(make([]byte, 10), 64, 64); err == nil {
		t.Error("expected an error for a byte count that disagrees with the dimensions")
	}
	if _, err := TextureFromRGBA(nil, 0, 0); err == nil {
		t.Error("expected an error for zero dimensions")
	}
}

// A texture arriving as raw RGBA must render the same as the equivalent PNG.
func TestTextureFromRGBARendersLikePNG(t *testing.T) {
	src := testTexture().(*image.NRGBA)

	fromRaw, err := TextureFromRGBA(src.Pix, 64, 64)
	if err != nil {
		t.Fatalf("TextureFromRGBA: %v", err)
	}

	a, err := Options{Texture: fromRaw, Size: 64}.RenderPNG()
	if err != nil {
		t.Fatalf("raw: %v", err)
	}
	b, err := RenderBytes(BytesOptions{Texture: testTextureBytes(t), Size: 64})
	if err != nil {
		t.Fatalf("png: %v", err)
	}
	if !bytes.Equal(a, b) {
		t.Error("raw RGBA and PNG textures produced different renders")
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	encoded, err := EncodePNG(testTexture())
	if err != nil {
		t.Fatalf("EncodePNG: %v", err)
	}
	decoded, err := DecodeImage(encoded)
	if err != nil {
		t.Fatalf("DecodeImage: %v", err)
	}
	if decoded.Bounds() != testTexture().Bounds() {
		t.Errorf("bounds = %v, want %v", decoded.Bounds(), testTexture().Bounds())
	}
}

func TestDecodeImageRejectsGarbage(t *testing.T) {
	if _, err := DecodeImage(nil); err == nil {
		t.Error("expected an error for empty data")
	}
	if _, err := DecodeImage([]byte("definitely not an image")); err == nil {
		t.Error("expected an error for garbage data")
	}
}

// The bytes path for animations is RenderGIF and RenderFrames with the decode
// and encode folded in: the same GIF, the same frames.
func TestAnimationBytesMatchRender(t *testing.T) {
	texBytes := testTextureBytes(t)
	tex, _ := DecodeImage(texBytes)
	opts := AnimationBytesOptions{BytesOptions: BytesOptions{Texture: texBytes, Size: 48}, Animation: MotionWave, FPS: 6}

	got, err := RenderGIFBytes(opts)
	if err != nil {
		t.Fatal(err)
	}
	want, err := RenderGIF(AnimationOptions{Options: Options{Texture: tex, Size: 48}, Animation: MotionWave, FPS: 6})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("RenderGIFBytes differs from RenderGIF")
	}

	pngs, err := opts.RenderFramesPNG()
	if err != nil {
		t.Fatal(err)
	}
	frames, _ := RenderFrames(AnimationOptions{Options: Options{Texture: tex, Size: 48}, Animation: MotionWave, FPS: 6})
	if len(pngs) != len(frames) {
		t.Fatalf("%d PNGs for %d frames", len(pngs), len(frames))
	}
	for i := range pngs {
		img, err := DecodeImage(pngs[i])
		if err != nil || !bytes.Equal(img.(*image.NRGBA).Pix, frames[i].(*image.NRGBA).Pix) {
			t.Fatalf("frame %d: %v", i, err)
		}
	}

	if _, err := RenderGIFBytes(AnimationBytesOptions{BytesOptions: BytesOptions{Texture: texBytes}}); !errors.Is(err, ErrNoAnimation) {
		t.Fatalf("no animation: got %v, want ErrNoAnimation", err)
	}
	if _, err := RenderGIFBytes(AnimationBytesOptions{Animation: MotionWalk}); !errors.Is(err, ErrNoTexture) {
		t.Fatalf("no texture: got %v, want ErrNoTexture", err)
	}
}
