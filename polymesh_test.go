package bedrockskin

import (
	"errors"
	"image"
	"image/color"
	"os"
	"testing"
)

func personaMeshGeometry(t *testing.T) []Geometry {
	t.Helper()
	raw, err := os.ReadFile("testdata/persona-mesh-geometry.json")
	if err != nil {
		t.Fatal(err)
	}
	geos, err := ParseGeometry(raw)
	if err != nil {
		t.Fatal(err)
	}
	return geos
}

func opaqueCount(img image.Image) int {
	n := 0
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a > 0 {
				n++
			}
		}
	}
	return n
}

// A persona's body is poly meshes, drawn in 3D rather than as a flat crop;
// its head comes only from the face texture.
func TestPersonaMeshRendersIn3D(t *testing.T) {
	geos := personaMeshGeometry(t)
	tex := makeTexture(255)
	face := image.NewNRGBA(image.Rect(0, 0, 32, 64))
	for y := 0; y < 16; y++ {
		for x := 0; x < 32; x++ {
			face.Set(x, y, color.NRGBA{R: 255, A: 255})
		}
	}

	body, err := Options{Texture: tex, Geometry: geos, Size: 64}.Render()
	if err != nil || opaqueCount(body) == 0 {
		t.Fatalf("body: %v, %d pixels", err, opaqueCount(body))
	}
	if _, err := (Options{Texture: tex, Geometry: geos, View: ViewHead}).Render(); !errors.Is(err, ErrEmptyView) {
		t.Fatalf("head without a face texture: got %v, want ErrEmptyView", err)
	}
	head, err := Options{Texture: tex, Geometry: geos, View: ViewHead, Size: 64,
		Animated: []AnimatedTexture{{Type: AnimatedFace, Texture: face}}}.Render()
	if err != nil {
		t.Fatal(err)
	}
	if r, _, _, a := head.At(32, 32).RGBA(); a == 0 || r>>8 != 255 {
		t.Fatalf("head centre should be the face's red, got r=%d a=%d", r>>8, a>>8)
	}
}

func TestPolygonsSkipsMalformed(t *testing.T) {
	b := Bone{PolyMesh: []byte(`{"positions":[[0,0,0],[1,0,0],[1,1,0],[0,1,0]],"uvs":[[0,0],[1,0],[1,1],[0,1]],
		"polys":[[[0,0,0],[1,0,1],[2,0,2]],[[0,0,0],[1,0,1]],[[0,0,0],[1,0,1],[4,0,3]],[[0,0,0],[1,0,1],[2,0,2.5]]]}`)}
	m, ok := b.Mesh()
	if !ok {
		t.Fatal("mesh did not read")
	}
	if got := len(m.Polygons()); got != 1 {
		t.Fatalf("got %d polygons, want 1", got)
	}
	m.Polys = []byte(`"quad_list"`)
	if got := len(m.Polygons()); got != 1 {
		t.Fatalf("quad_list: got %d polygons, want 1", got)
	}
	if _, ok := (Bone{PolyMesh: []byte(`[1]`)}).Mesh(); ok {
		t.Fatal("a non-object mesh should not read")
	}
}

// Persona bodies are poly meshes on lowercase bones; the detector measures
// them instead of calling the skin invisible.
func TestDetectorMeasuresPersonaMesh(t *testing.T) {
	raw, _ := os.ReadFile("testdata/persona-mesh-geometry.json")
	r := ValidateSkinInvisibility(makeTexture(255), raw)
	if r.IsInvisible || r.VisibleParts != 6 {
		t.Fatalf("got invisible=%v visible=%d, want all 6 parts", r.IsInvisible, r.VisibleParts)
	}
	r = ValidateSkinInvisibility(makeTexture(0), raw)
	if !r.IsInvisible {
		t.Fatal("a transparent texture on a persona mesh should be invisible")
	}
}

// Polygons hands back each corner looked up: position, normal and UV, with a
// missing normal left zero rather than dropping the polygon.
func TestPolygonsResolveCorners(t *testing.T) {
	m, ok := Bone{PolyMesh: []byte(`{"normalized_uvs":true,
		"positions":[[0,0,0],[1,0,0],[1,1,0],[0,1,0]],
		"normals":[[0,0,-1]],
		"uvs":[[0,0],[1,0],[1,1],[0,1]],
		"polys":[[[0,0,0],[1,0,1],[2,0,2]],[[0,0,0],[2,5,2],[3,0,3]]]}`)}.Mesh()
	if !ok || !m.NormalizedUVs {
		t.Fatalf("mesh: %+v %v", m, ok)
	}
	got := m.Polygons()
	if len(got) != 2 || len(got[0]) != 3 {
		t.Fatalf("got %d polygons", len(got))
	}
	want := PolyVertex{Position: [3]float64{1, 1, 0}, Normal: [3]float64{0, 0, -1}, UV: [2]float64{1, 1}}
	if got[0][2] != want {
		t.Fatalf("corner = %+v, want %+v", got[0][2], want)
	}
	if got[1][1].Normal != ([3]float64{}) || got[1][1].Position != ([3]float64{1, 1, 0}) {
		t.Fatalf("a missing normal should leave it zero: %+v", got[1][1])
	}
	m.Polys = []byte(`"quad_list"`)
	if q := m.Polygons(); len(q) != 1 || q[0][3].UV != ([2]float64{0, 1}) {
		t.Fatalf("quad_list: %+v", q)
	}
}
