package skinapi

import (
	"errors"
	"reflect"
	"testing"
)

const modernGeo = `{
	"format_version": "1.12.0",
	"minecraft:geometry": [
		{
			"description": {"identifier": "geometry.cape", "texture_width": 64, "texture_height": 32},
			"bones": [{"name": "cape", "pivot": [0, 24, 3], "cubes": [{"origin": [-5, 8, 3], "size": [10, 16, 1], "uv": [0, 0]}]}]
		},
		{
			"description": {
				"identifier": "geometry.humanoid.custom", "texture_width": 64, "texture_height": 64,
				"visible_bounds_width": 1, "visible_bounds_height": 2, "visible_bounds_offset": [0, 1, 0]
			},
			"bones": [
				{"name": "root", "pivot": [0, 0, 0]},
				{"name": "body", "parent": "root", "pivot": [0, 24, 0], "cubes": [{"origin": [-4, 12, -2], "size": [8, 12, 4], "uv": [16, 16]}]},
				{"name": "rightArm", "parent": "body", "pivot": [-5, 22, 0],
					"cubes": [
						{"origin": [-8, 12, -2], "size": [4, 12, 4], "uv": [40, 16]},
						{"origin": [-8, 24, -2], "size": [4, 2, 4], "uv": {"north": {"uv": [1, 2], "uv_size": [4, 2], "material_instance": "glow"}}}
					]},
				{"name": "rightItem", "parent": "rightArm", "pivot": [-6, 15, 1],
					"locators": {"lead_hold": [-6, 15, 1], "particles": {"offset": [0, 26, 0], "rotation": [0, 90, 0]}},
					"some_future_field": {"kept": true}}
			]
		}
	]
}`

const legacyGeo = `{
	"format_version": "1.8.0",
	"geometry.hat:geometry.humanoid": {
		"texturewidth": 128, "textureheight": 64, "visible_bounds_width": 2,
		"bones": [{"name": "hat", "pivot": [0, 24, 0], "cubes": [{"origin": [-5, 31, -5], "size": [10, 4, 10], "uv": [0, 0]}]}]
	},
	"geometry.empty": {"bones": []}
}`

func mustTree(t *testing.T, raw string) *GeometryTree {
	t.Helper()
	tree, err := ParseGeometryTree([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	return tree
}

func TestGeometryTreeGet(t *testing.T) {
	tree := mustTree(t, modernGeo)
	if tree.FormatVersion != "1.12.0" {
		t.Errorf("FormatVersion = %q", tree.FormatVersion)
	}
	if got := tree.Identifiers(); !reflect.DeepEqual(got, []string{"geometry.cape", "geometry.humanoid.custom"}) {
		t.Errorf("Identifiers = %v", got)
	}
	v, ok := tree.Get("geometry.humanoid.custom/bones/rightArm/pivot")
	if !ok {
		t.Fatal("pivot not found")
	}
	if p, _ := v.Floats(); !reflect.DeepEqual(p, []float64{-5, 22, 0}) {
		t.Errorf("pivot = %v", p)
	}
	if v.Path != "geometry.humanoid.custom/bones/rightArm/pivot" {
		t.Errorf("path = %q", v.Path)
	}
	// One number, by index and from the end; case-insensitive names.
	if f, _ := mustGet(t, tree, "GEOMETRY.HUMANOID.CUSTOM/bones/RIGHTARM/cubes/0/size/1").Float(); f != 12 {
		t.Errorf("size y = %v", f)
	}
	if s, _ := mustGet(t, tree, "geometry.humanoid.custom/bones/-1/name").Text(); s != "rightItem" {
		t.Errorf("last bone = %q", s)
	}
	if w, _ := mustGet(t, tree, "geometry.humanoid.custom/description/visible_bounds_height").Float(); w != 2 {
		t.Errorf("visible_bounds_height = %v", w)
	}
	// Fields the library has no type for are kept.
	if v := mustGet(t, tree, "geometry.humanoid.custom/bones/rightItem/some_future_field/kept"); v.Value != true {
		t.Errorf("unknown field = %v", v.Value)
	}
	for _, missing := range []string{"geometry.nope", "geometry.cape/bones/arm", "geometry.cape/bones/7", "geometry.cape/bones/cape/pivot/x"} {
		if _, ok := tree.Get(missing); ok {
			t.Errorf("Get(%q) found something", missing)
		}
	}
}

func mustGet(t *testing.T, tree *GeometryTree, path string) GeometryValue {
	t.Helper()
	v, ok := tree.Get(path)
	if !ok {
		t.Fatalf("Get(%q): nothing", path)
	}
	return v
}

func TestGeometryTreeSelectWildcards(t *testing.T) {
	tree := mustTree(t, modernGeo)
	sizes := tree.Select("*/bones/*/cubes/*/size")
	if len(sizes) != 4 {
		t.Fatalf("got %d cube sizes, want 4", len(sizes))
	}
	if sizes[0].Path != "geometry.cape/bones/cape/cubes/0/size" || sizes[3].Path != "geometry.humanoid.custom/bones/rightArm/cubes/1/size" {
		t.Errorf("paths = %q ... %q", sizes[0].Path, sizes[3].Path)
	}
	var names []string
	for _, v := range tree.Select("geometry.humanoid.custom/bones/*/name") {
		s, _ := v.Text()
		names = append(names, s)
	}
	if !reflect.DeepEqual(names, []string{"root", "body", "rightArm", "rightItem"}) {
		t.Errorf("names = %v", names)
	}
	if got := len(tree.Select("*/bones/*/locators/*")); got != 2 {
		t.Errorf("got %d locators, want 2", got)
	}
	if got := len(tree.Select("")); got != 2 {
		t.Errorf("empty path: got %d models, want 2", got)
	}
}

func TestGeometryTreeDecode(t *testing.T) {
	tree := mustTree(t, modernGeo)
	var b Bone
	if err := mustGet(t, tree, "geometry.humanoid.custom/bones/rightItem").Decode(&b); err != nil {
		t.Fatal(err)
	}
	if b.Parent != "rightArm" || len(b.Locators) != 2 {
		t.Fatalf("bone = %+v", b)
	}
	if l := b.Locators["lead_hold"]; !reflect.DeepEqual(l.Offset, []float64{-6, 15, 1}) {
		t.Errorf("array locator = %+v", l)
	}
	if l := b.Locators["particles"]; !reflect.DeepEqual(l.Rotation, []float64{0, 90, 0}) {
		t.Errorf("object locator = %+v", l)
	}
	var c Cube
	if err := mustGet(t, tree, "geometry.humanoid.custom/bones/rightArm/cubes/1").Decode(&c); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := c.BoxUV(); ok {
		t.Error("per-face cube reported a box uv")
	}
	if f := c.FaceUVs()["north"]; f.MaterialInstance != "glow" || !reflect.DeepEqual(f.UVSize, []float64{4, 2}) {
		t.Errorf("north face = %+v", f)
	}
}

func TestGeometryTreeMatchesParseGeometry(t *testing.T) {
	for _, raw := range []string{modernGeo, legacyGeo} {
		want, err := ParseGeometry([]byte(raw))
		if err != nil {
			t.Fatal(err)
		}
		// The tree holds the same models in the same order, so paths and
		// typed models agree.
		var ids []string
		for _, g := range want {
			ids = append(ids, g.Identifier)
		}
		if got := mustTree(t, raw).Identifiers(); !reflect.DeepEqual(got, ids) {
			t.Errorf("Identifiers = %v, ParseGeometry has %v", got, ids)
		}
	}
}

func TestGeometryTreeLegacy(t *testing.T) {
	tree := mustTree(t, legacyGeo)
	// The empty model is skipped, as ParseGeometry skips it.
	if got := tree.Identifiers(); !reflect.DeepEqual(got, []string{"geometry.hat:geometry.humanoid"}) {
		t.Errorf("Identifiers = %v", got)
	}
	// Legacy texture sizes read under the modern names; the model is found
	// by the name before the colon too.
	if w, _ := mustGet(t, tree, "geometry.hat/description/texture_width").Float(); w != 128 {
		t.Errorf("texture_width = %v", w)
	}
	if v := mustGet(t, tree, "geometry.hat:geometry.humanoid/bones/hat/cubes/0/origin"); v.Path != "geometry.hat:geometry.humanoid/bones/hat/cubes/0/origin" {
		t.Errorf("path = %q", v.Path)
	}
}

func TestGeometryTreeErrors(t *testing.T) {
	if _, err := ParseGeometryTree([]byte(`{"format_version": "1.12.0"}`)); !errors.Is(err, ErrNoGeometryModels) {
		t.Errorf("no models: err = %v", err)
	}
	if _, err := ParseGeometryTree([]byte(`{`)); err == nil {
		t.Error("bad JSON: no error")
	}
}

func TestGeometryHelpers(t *testing.T) {
	geos, err := ParseGeometry([]byte(modernGeo))
	if err != nil {
		t.Fatal(err)
	}
	g, _ := SelectGeometry(geos, "geometry.humanoid.custom")
	if g.VisibleBoundsHeight != 2 || !reflect.DeepEqual(g.VisibleBoundsOffset, []float64{0, 1, 0}) {
		t.Errorf("visible bounds = %v %v", g.VisibleBoundsHeight, g.VisibleBoundsOffset)
	}
	if kids := g.Children("body"); len(kids) != 1 || kids[0].Name != "rightArm" {
		t.Errorf("Children(body) = %v", kids)
	}
	l, bone, ok := g.Locator("particles")
	if !ok || bone.Name != "rightItem" || !reflect.DeepEqual(l.Offset, []float64{0, 26, 0}) {
		t.Errorf("Locator(particles) = %+v on %s, %v", l, bone.Name, ok)
	}
}

// A malformed locator must not stop a skin rendering.
func TestMalformedLocatorStillParses(t *testing.T) {
	raw := `{"format_version":"1.12.0","minecraft:geometry":[{"description":{"identifier":"g"},
		"bones":[{"name":"b","locators":{"x":"nonsense"},"cubes":[{"origin":[0,0,0],"size":[1,1,1],"uv":[0,0]}]}]}]}`
	if _, err := ParseGeometry([]byte(raw)); err != nil {
		t.Fatal(err)
	}
}
