package bedrockskin

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
)

// testArmorTexture is a procedural armor layer, 64x32 like the game's: a
// gradient with the lower arm left transparent, as real chestplates do.
func testArmorTexture() image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, 64, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 64; x++ {
			a := uint8(255)
			if x >= 40 && x < 56 && y >= 26 {
				a = 0
			}
			img.Set(x, y, color.NRGBA{R: 40, G: uint8(120 + x*2), B: uint8(140 + y*3), A: a})
		}
	}
	return img
}

// testItemTexture is a procedural 16x16 sprite: a diagonal blade from the
// bottom-left handle to the top-right tip, plus one half-transparent pixel
// that the alpha test must drop.
func testItemTexture() image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	for i := 1; i < 15; i++ {
		img.Set(i, 15-i, color.NRGBA{R: uint8(i * 16), G: 200, B: 220, A: 255})
		img.Set(i+1, 15-i, color.NRGBA{R: 30, G: 60, B: uint8(i * 16), A: 255})
	}
	img.Set(0, 0, color.NRGBA{R: 255, A: 127})
	return img
}

func renderOrFail(t *testing.T, opts Options) image.Image {
	t.Helper()
	img, err := Render(opts)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	return img
}

func TestNoEquipmentRendersAsBefore(t *testing.T) {
	tex := testTexture()
	plain := renderOrFail(t, Options{Texture: tex, Size: 64})
	zero := renderOrFail(t, Options{Texture: tex, Size: 64, Armor: Armor{}, HeldItem: nil})
	if !imagesEqual(plain, zero) {
		t.Fatal("zero Armor and nil HeldItem changed the render")
	}
}

func TestEachArmorPieceDraws(t *testing.T) {
	tex, armor := testTexture(), testArmorTexture()
	bare := renderOrFail(t, Options{Texture: tex, Size: 64})
	pieces := map[string]Armor{
		"helmet":     {Helmet: armor},
		"chestplate": {Chestplate: armor},
		"leggings":   {Leggings: armor},
		"boots":      {Boots: armor},
	}
	for name, a := range pieces {
		if imagesEqual(bare, renderOrFail(t, Options{Texture: tex, Size: 64, Armor: a})) {
			t.Errorf("%s did not change the render", name)
		}
	}
}

func TestArmorSetWearsEveryPiece(t *testing.T) {
	l1, l2 := testArmorTexture(), testTexture()
	got := ArmorSet(l1, l2)
	want := Armor{Helmet: l1, Chestplate: l1, Leggings: l2, Boots: l1}
	if got != want {
		t.Fatalf("ArmorSet = %+v, want %+v", got, want)
	}
}

func TestHeadViewsShowOnlyTheHelmet(t *testing.T) {
	tex, armor := testTexture(), testArmorTexture()
	helmet := renderOrFail(t, Options{Texture: tex, View: ViewAvatar, Size: 64, Armor: Armor{Helmet: armor}})
	full := renderOrFail(t, Options{Texture: tex, View: ViewAvatar, Size: 64, Armor: ArmorSet(armor, armor), HeldItem: testItemTexture()})
	if !imagesEqual(helmet, full) {
		t.Fatal("an avatar drew more than the helmet")
	}
}

func TestHeldItemDraws(t *testing.T) {
	tex := testTexture()
	bare := renderOrFail(t, Options{Texture: tex, Size: 64})
	held := renderOrFail(t, Options{Texture: tex, Size: 64, HeldItem: testItemTexture()})
	if imagesEqual(bare, held) {
		t.Fatal("the held item did not change the render")
	}
}

func TestHeldItemCubesFollowTheAlphaTest(t *testing.T) {
	g, ok := heldItemGeometry(DefaultGeometry()[1], testItemTexture())
	if !ok {
		t.Fatal("no right arm found on the default model")
	}
	item, _ := g.BoneByName(heldItemBone)
	// 14 blade pixels and 14 edge pixels; the half-transparent one is dropped.
	if got := len(item.Cubes); got != 28 {
		t.Fatalf("got %d cubes, want 28", got)
	}
	if item.Parent != "rightArm" {
		t.Fatalf("item hangs from %q, want rightArm", item.Parent)
	}
	if want := []float64{-6, 15, 1}; !floatsEqual(item.Pivot, want) {
		t.Fatalf("grip at %v, want the model's rightItem %v", item.Pivot, want)
	}
}

func TestHeldItemNeedsARightArm(t *testing.T) {
	geo := Geometry{Identifier: "geometry.test", TextureWidth: 64, TextureHeight: 64, Bones: []Bone{{
		Name:  "head",
		Pivot: []float64{0, 24, 0},
		Cubes: []Cube{{Origin: []float64{-4, 24, -4}, Size: []float64{8, 8, 8}, UV: []byte("[0,0]")}},
	}}}
	tex := testTexture()
	bare := renderOrFail(t, Options{Texture: tex, Geometry: []Geometry{geo}, Size: 64})
	held := renderOrFail(t, Options{Texture: tex, Geometry: []Geometry{geo}, Size: 64, HeldItem: testItemTexture()})
	if !imagesEqual(bare, held) {
		t.Fatal("a model with no right arm held the item anyway")
	}
}

func TestHeldItemUsesTheModelsGrip(t *testing.T) {
	// The slim model's rightItem sits half a unit lower than the wide one's.
	slim := DefaultGeometry()[2]
	if slim.Identifier != "geometry.humanoid.customSlim" {
		t.Fatalf("unexpected default entry %q", slim.Identifier)
	}
	g, _ := heldItemGeometry(slim, testItemTexture())
	item, _ := g.BoneByName(heldItemBone)
	if want := []float64{-6, 14.5, 1}; !floatsEqual(item.Pivot, want) {
		t.Fatalf("grip at %v, want %v", item.Pivot, want)
	}
}

func TestEquipmentMovesWithAnimation(t *testing.T) {
	frames, err := RenderFrames(AnimationOptions{
		Options:   Options{Texture: testTexture(), Size: 48, Armor: ArmorSet(testArmorTexture(), testArmorTexture()), HeldItem: testItemTexture()},
		Animation: MotionWalk,
		FPS:       4,
		Workers:   1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if imagesEqual(frames[0], frames[1]) {
		t.Fatal("equipped frames did not move")
	}
}

func TestBytesOptionsCarryEquipment(t *testing.T) {
	enc := func(img image.Image) []byte {
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			t.Fatal(err)
		}
		return buf.Bytes()
	}
	tex, armor, item := testTexture(), testArmorTexture(), testItemTexture()
	want, err := EncodePNG(renderOrFail(t, Options{Texture: tex, Size: 64, Armor: ArmorSet(armor, armor), HeldItem: item}))
	if err != nil {
		t.Fatal(err)
	}
	got, err := RenderBytes(BytesOptions{Texture: enc(tex), Size: 64, Armor: ArmorSetBytes(enc(armor), enc(armor)), HeldItem: enc(item)})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("RenderBytes with equipment differs from Render")
	}

	_, err = RenderBytes(BytesOptions{Texture: enc(tex), Armor: ArmorBytes{Leggings: []byte("nope")}})
	if err == nil || !strings.HasPrefix(err.Error(), "armor leggings:") {
		t.Fatalf("bad leggings: got %v", err)
	}
}

func imagesEqual(a, b image.Image) bool {
	if a.Bounds() != b.Bounds() {
		return false
	}
	for y := a.Bounds().Min.Y; y < a.Bounds().Max.Y; y++ {
		for x := a.Bounds().Min.X; x < a.Bounds().Max.X; x++ {
			if a.At(x, y) != b.At(x, y) {
				return false
			}
		}
	}
	return true
}

func floatsEqual(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
