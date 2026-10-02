package bedrockskin

import (
	"bytes"
	"image"
	"os"
	"strings"
	"testing"
)

func TestWireSkinOptions(t *testing.T) {
	tex := testTexture().(*image.NRGBA)
	face := image.NewNRGBA(image.Rect(0, 0, 32, 64))
	geo, err := os.ReadFile("testdata/persona-mesh-geometry.json")
	if err != nil {
		t.Fatal(err)
	}
	w := WireSkin{
		SkinData: tex.Pix, SkinWidth: 64, SkinHeight: 64,
		CapeData: make([]byte, 64*32*4), CapeWidth: 64, CapeHeight: 32,
		Geometry:      geo,
		ResourcePatch: []byte(`{"geometry":{"default":"geometry.persona_test","animated_face":"geometry.animated_face_persona-test"}}`),
		Animations:    []WireAnimation{{Type: AnimatedFace, Data: face.Pix, Width: 32, Height: 64}},
	}
	o, err := w.Options()
	if err != nil {
		t.Fatal(err)
	}
	if o.Identifier != "geometry.persona_test" || len(o.Geometry) != 2 || o.Cape == nil || len(o.Animated) != 1 {
		t.Fatalf("options = id %q, %d entries, cape %v, %d animated", o.Identifier, len(o.Geometry), o.Cape != nil, len(o.Animated))
	}
	// The face texture is wired up: the head view draws.
	o.View = ViewHead
	if _, err := o.Render(); err != nil {
		t.Fatalf("head with the face attached: %v", err)
	}

	// "null" geometry and a patch that does not parse fall back quietly.
	w2 := WireSkin{SkinData: tex.Pix, SkinWidth: 64, SkinHeight: 64, Geometry: []byte("null"), ResourcePatch: []byte("{")}
	if o, err := w2.Options(); err != nil || o.Geometry != nil || o.Identifier != "" {
		t.Fatalf("fallbacks: %+v %v", o, err)
	}

	for name, bad := range map[string]WireSkin{
		"skin":      {SkinData: tex.Pix[:10], SkinWidth: 64, SkinHeight: 64},
		"cape":      {SkinData: tex.Pix, SkinWidth: 64, SkinHeight: 64, CapeData: []byte{1}, CapeWidth: 64, CapeHeight: 32},
		"geometry":  {SkinData: tex.Pix, SkinWidth: 64, SkinHeight: 64, Geometry: []byte("{")},
		"animation": {SkinData: tex.Pix, SkinWidth: 64, SkinHeight: 64, Animations: []WireAnimation{{Type: AnimatedFace, Data: []byte{1}, Width: 32, Height: 64}}},
	} {
		if _, err := bad.Options(); err == nil || !strings.HasPrefix(err.Error(), name) {
			t.Errorf("%s: got %v, want an error naming it", name, err)
		}
	}

	if s, err := (WireSkin{SkinData: make([]byte, 64*64*4), SkinWidth: 64, SkinHeight: 64, Geometry: []byte("null")}).Skin(); err != nil || !s.IsInvisible() {
		t.Fatalf("a transparent wire skin should read invisible: %v", err)
	}
}

// The writers write exactly what the byte-returning functions return.
func TestWritersMatchRenderers(t *testing.T) {
	o := Options{Texture: testTexture(), Size: 64}
	want, _ := o.RenderPNG()
	var got bytes.Buffer
	if err := o.WritePNG(&got); err != nil || !bytes.Equal(got.Bytes(), want) {
		t.Fatalf("WritePNG differs from RenderPNG (%v)", err)
	}
	a := AnimationOptions{Options: o, Animation: MotionWave, FPS: 6}
	wantGIF, _ := RenderGIF(a)
	got.Reset()
	if err := a.WriteGIF(&got); err != nil || !bytes.Equal(got.Bytes(), wantGIF) {
		t.Fatalf("WriteGIF differs from RenderGIF (%v)", err)
	}
}
