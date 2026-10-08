package bedrockskin

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"github.com/fogleman/fauxgl"
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
	zero := renderOrFail(t, Options{Texture: tex, Size: 64, Armor: Armor{}, RightHand: Held{}, LeftHand: Held{}, Scale: Scale{}})
	if !imagesEqual(plain, zero) {
		t.Fatal("zero equipment changed the render")
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
		"elytra":     {Elytra: armor},
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

func TestElytraTakesTheChestplatesSlot(t *testing.T) {
	tex, armor, ely := testTexture(), testArmorTexture(), testTexture()
	both := renderOrFail(t, Options{Texture: tex, Size: 64, Armor: Armor{Chestplate: armor, Elytra: ely}})
	elytra := renderOrFail(t, Options{Texture: tex, Size: 64, Armor: Armor{Elytra: ely}})
	if !imagesEqual(both, elytra) {
		t.Fatal("a chestplate was drawn under the elytra")
	}
}

func TestHeadViewsShowOnlyTheHelmet(t *testing.T) {
	tex, armor := testTexture(), testArmorTexture()
	helmet := renderOrFail(t, Options{Texture: tex, View: ViewAvatar, Size: 64, Armor: Armor{Helmet: armor}})
	full := renderOrFail(t, Options{
		Texture: tex, View: ViewAvatar, Size: 64,
		Armor:     Armor{Helmet: armor, Chestplate: armor, Leggings: armor, Boots: armor, Elytra: armor},
		RightHand: Held{Item: testItemTexture()},
		LeftHand:  Held{Item: testItemTexture()},
	})
	if !imagesEqual(helmet, full) {
		t.Fatal("an avatar drew more than the helmet")
	}
}

func TestEachHandHoldsItsOwnItem(t *testing.T) {
	tex, item := testTexture(), testItemTexture()
	bare := renderOrFail(t, Options{Texture: tex, Size: 64})
	right := renderOrFail(t, Options{Texture: tex, Size: 64, RightHand: Held{Item: item}})
	left := renderOrFail(t, Options{Texture: tex, Size: 64, LeftHand: Held{Item: item}})
	both := renderOrFail(t, Options{Texture: tex, Size: 64, RightHand: Held{Item: item}, LeftHand: Held{Item: item}})
	for name, img := range map[string]image.Image{"right": right, "left": left} {
		if imagesEqual(bare, img) {
			t.Errorf("the %s hand's item did not change the render", name)
		}
	}
	if imagesEqual(right, left) || imagesEqual(both, right) || imagesEqual(both, left) {
		t.Fatal("the hands do not hold their items apart")
	}
}

func TestHeldItemFollowsTheAlphaTest(t *testing.T) {
	// The test sprite's diagonal has 28 opaque texels and one half-transparent
	// one; the half-transparent one must add no edges.
	plain := testItemTexture().(*image.NRGBA)
	without := image.NewNRGBA(plain.Rect)
	copy(without.Pix, plain.Pix)
	without.SetNRGBA(0, 0, color.NRGBA{})
	world, to := fauxgl.Identity(), Held{}.toModel(hands[0])
	if a, b := len(buildHeldItem(plain, to, world)), len(buildHeldItem(without, to, world)); a != b {
		t.Fatalf("a half-transparent texel changed the mesh: %d vs %d triangles", a, b)
	}
}

func TestHeldItemGripsAtTheItemBone(t *testing.T) {
	wide, slim := DefaultGeometry()[1], DefaultGeometry()[2]
	for _, tc := range []struct {
		geo   Geometry
		side  hand
		arm   string
		pivot []float64
	}{
		{wide, hands[0], "rightArm", []float64{-6, 15, 1}},
		{wide, hands[1], "leftArm", []float64{6, 15, 1}},
		{slim, hands[0], "rightArm", []float64{-6, 14.5, 1}},
		{slim, hands[1], "leftArm", []float64{6, 14.5, 1}},
	} {
		skel, arm, ok := tc.side.skeleton(tc.geo)
		if !ok || arm != tc.arm {
			t.Fatalf("%s %s: got arm %q", tc.geo.Identifier, tc.side.arm, arm)
		}
		item, _ := skel.BoneByName(tc.side.bone)
		if item.Parent != tc.arm || !floatsEqual(item.Pivot, tc.pivot) {
			t.Fatalf("%s %s: item on %q at %v, want %s at %v", tc.geo.Identifier, tc.side.arm, item.Parent, item.Pivot, tc.arm, tc.pivot)
		}
	}
}

func TestHeldItemGripsWithoutAnItemBone(t *testing.T) {
	geo := Geometry{Bones: []Bone{
		{Name: "body", Pivot: []float64{0, 24, 0}},
		{Name: "LEFTARM", Parent: "body", Pivot: []float64{5, 22, 0}},
	}}
	skel, arm, ok := hands[1].skeleton(geo)
	if !ok || arm != "LEFTARM" {
		t.Fatalf("got arm %q, ok %v", arm, ok)
	}
	item, _ := skel.BoneByName(hands[1].bone)
	if want := []float64{6, 15, 1}; !floatsEqual(item.Pivot, want) {
		t.Fatalf("grip at %v, want %v", item.Pivot, want)
	}
	if _, _, ok := hands[0].skeleton(geo); ok {
		t.Fatal("found a right arm on a model with none")
	}
}

func TestHeldItemNeedsTheArm(t *testing.T) {
	geo := Geometry{Identifier: "geometry.test", TextureWidth: 64, TextureHeight: 64, Bones: []Bone{{
		Name:  "head",
		Pivot: []float64{0, 24, 0},
		Cubes: []Cube{{Origin: []float64{-4, 24, -4}, Size: []float64{8, 8, 8}, UV: []byte("[0,0]")}},
	}}}
	tex := testTexture()
	bare := renderOrFail(t, Options{Texture: tex, Geometry: []Geometry{geo}, Size: 64})
	held := renderOrFail(t, Options{Texture: tex, Geometry: []Geometry{geo}, Size: 64, RightHand: Held{Item: testItemTexture()}})
	if !imagesEqual(bare, held) {
		t.Fatal("a model with no right arm held the item anyway")
	}
}

func TestHeldSwordPointsForwardAndUp(t *testing.T) {
	// Handle and tip of a 16-texel sword, in model units from the grip.
	to := Held{}.toModel(hands[0])
	handle := to.MulPosition(fauxgl.Vector{X: -1.0 / 16, Y: 1 - 15.0/16})
	tip := to.MulPosition(fauxgl.Vector{X: -15.0 / 16, Y: 1 - 1.0/16})
	blade := tip.Sub(handle)
	// Forward is -Z; the blade runs about 18 units ahead and a little up.
	if blade.Z > -15 || blade.Y <= 0 {
		t.Fatalf("blade %v does not point forward and up", blade)
	}
}

func TestItemAdjustMovesTheItem(t *testing.T) {
	p := fauxgl.Vector{X: -0.5, Y: 0.5}
	base := Held{}.toModel(hands[0]).MulPosition(p)

	moved := Held{Adjust: ItemAdjust{Offset: [3]float64{1, 2, 3}}}.toModel(hands[0]).MulPosition(p)
	if d := moved.Sub(base); d != (fauxgl.Vector{X: 1, Y: 2, Z: 3}) {
		t.Fatalf("an offset of 1,2,3 moved the item by %v", d)
	}
	scaled := Held{Adjust: ItemAdjust{Scale: 2}}.toModel(hands[0]).MulPosition(p)
	if d := scaled.Sub(base.MulScalar(2)).Length(); d > 1e-12 {
		t.Fatalf("a scale of 2 did not double the item about the grip: off by %v", d)
	}
	turned := Held{Adjust: ItemAdjust{Rotation: [3]float64{0, 90, 0}}}.toModel(hands[0]).MulPosition(p)
	if d := turned.Length() - base.Length(); d > 1e-12 || d < -1e-12 {
		t.Fatalf("a rotation changed the distance from the grip by %v", d)
	}
	if turned.Sub(base).Length() < 1 {
		t.Fatal("a 90 degree turn hardly moved the item")
	}
}

func TestHoldingSwingsTheArmForward(t *testing.T) {
	pose := hands[0].holdingPose(Pose{"rightarm": {Rotation: [3]float64{40, 0, 5}}, "head": {}}, "rightarm")
	if got := pose["rightArm"].Rotation; got != [3]float64{2, 0, 5} {
		t.Fatalf("right arm turn %v, want [2 0 5]", got)
	}
	if _, ok := pose["rightarm"]; ok {
		t.Fatal("the old right arm entry is still there")
	}
	if _, ok := pose["head"]; !ok {
		t.Fatal("holding dropped another bone's pose")
	}
	if got := hands[1].holdingPose(nil, "leftArm")["leftArm"].Rotation; got != [3]float64{-18, 0, 0} {
		t.Fatalf("rest left arm turn %v, want [-18 0 0]", got)
	}
}

func TestFlatItemsAreHeldDifferently(t *testing.T) {
	tex, item := testTexture(), testItemTexture()
	upright := renderOrFail(t, Options{Texture: tex, Size: 64, RightHand: Held{Item: item}})
	flat := renderOrFail(t, Options{Texture: tex, Size: 64, RightHand: Held{Item: item, Flat: true}})
	if imagesEqual(upright, flat) {
		t.Fatal("Flat made no difference")
	}
}

func TestScaleParts(t *testing.T) {
	tex := testTexture()
	bare := renderOrFail(t, Options{Texture: tex, Size: 64})
	if imagesEqual(bare, renderOrFail(t, Options{Texture: tex, Size: 64, Scale: Scale{Parts: map[string]float64{"HEAD": 1.5}}})) {
		t.Fatal("scaling the head changed nothing")
	}
	pose := Scale{Parts: map[string]float64{"head": 2}}.partsPose(Pose{"Head": {Scale: [3]float64{1, 3, 1}, Scaled: true}})
	if got := pose.of("head"); !got.Scaled || got.Scale != [3]float64{2, 6, 2} {
		t.Fatalf("head scale %v, want the pose's and the part's multiplied", got)
	}
	if len(pose) != 1 {
		t.Fatalf("pose has %d entries for one bone", len(pose))
	}
}

func TestScaleModelZooms(t *testing.T) {
	tex := testTexture()
	opaque := func(img image.Image) int {
		n := 0
		b := img.Bounds()
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				if _, _, _, a := img.At(x, y).RGBA(); a != 0 {
					n++
				}
			}
		}
		return n
	}
	normal := opaque(renderOrFail(t, Options{Texture: tex, Size: 64}))
	big := opaque(renderOrFail(t, Options{Texture: tex, Size: 64, Scale: Scale{Model: 1.5}}))
	small := opaque(renderOrFail(t, Options{Texture: tex, Size: 64, Scale: Scale{Model: 0.5}}))
	if !(small < normal && normal < big) {
		t.Fatalf("model scale did not resize the figure: 0.5 %d, 1 %d, 1.5 %d", small, normal, big)
	}
}

func TestEquipmentMovesWithAnimation(t *testing.T) {
	frames, err := RenderFrames(AnimationOptions{
		Options: Options{
			Texture: testTexture(), Size: 48,
			Armor:     ArmorSet(testArmorTexture(), testArmorTexture()),
			RightHand: Held{Item: testItemTexture()},
			LeftHand:  Held{Item: testItemTexture(), Flat: true},
		},
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
	adjust := ItemAdjust{Offset: [3]float64{0, 1, 0}, Rotation: [3]float64{10, 0, 0}, Scale: 1.2}
	winged := ArmorSet(armor, armor)
	winged.Elytra = tex
	scale := Scale{Model: 1.2, Parts: map[string]float64{"head": 1.3}}
	want, err := EncodePNG(renderOrFail(t, Options{
		Texture: tex, Size: 64,
		Armor:     winged,
		RightHand: Held{Item: item, Adjust: adjust},
		LeftHand:  Held{Item: item, Flat: true},
		Scale:     scale,
	}))
	if err != nil {
		t.Fatal(err)
	}
	got, err := RenderBytes(BytesOptions{
		Texture: enc(tex), Size: 64,
		Armor:     ArmorBytes{Helmet: enc(armor), Chestplate: enc(armor), Leggings: enc(armor), Boots: enc(armor), Elytra: enc(tex)},
		RightHand: HeldBytes{Item: enc(item), Adjust: adjust},
		LeftHand:  HeldBytes{Item: enc(item), Flat: true},
		Scale:     scale,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("RenderBytes with equipment differs from Render")
	}

	for _, tc := range []struct {
		opts BytesOptions
		want string
	}{
		{BytesOptions{Texture: enc(tex), Armor: ArmorBytes{Leggings: []byte("nope")}}, "armor leggings:"},
		{BytesOptions{Texture: enc(tex), LeftHand: HeldBytes{Item: []byte("nope")}}, "left hand item:"},
	} {
		if _, err := RenderBytes(tc.opts); err == nil || !strings.HasPrefix(err.Error(), tc.want) {
			t.Errorf("got %v, want an error starting %q", err, tc.want)
		}
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
