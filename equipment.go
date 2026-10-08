package bedrockskin

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"image"
	"strconv"
)

// Armor is the armor a skin wears: one texture per piece, as a resource pack
// lays them out. The helmet, chestplate and boots take the set's first layer
// (e.g. textures/models/armor/diamond_1.png), the leggings its second
// (diamond_2.png). A nil piece is not worn, so pieces from different sets
// mix freely. See docs/equipment.md.
type Armor struct {
	Helmet     image.Image
	Chestplate image.Image
	Leggings   image.Image
	Boots      image.Image
}

// ArmorSet is a full set of one material: layer1 for the helmet, chestplate
// and boots, layer2 for the leggings.
func ArmorSet(layer1, layer2 image.Image) Armor {
	return Armor{Helmet: layer1, Chestplate: layer1, Leggings: layer2, Boots: layer1}
}

// armorGeometryJSON is the vanilla armor model, one entry per piece: the
// sizes and inflates of the game's geometry.humanoid.armor1 and armor2, on
// the player model's skeleton so a pose moves it with the skin. See
// docs/equipment.md#the-armor-model.
//
//go:embed armor_geometry.json
var armorGeometryJSON []byte

var armorGeometry = mustParseArmor()

func mustParseArmor() map[string]Geometry {
	geos, err := ParseGeometry(armorGeometryJSON)
	if err != nil {
		panic(fmt.Sprintf("bedrockskin: embedded armor_geometry.json is invalid: %v", err))
	}
	out := make(map[string]Geometry, len(geos))
	for _, g := range geos {
		out[g.Identifier] = g
	}
	return out
}

// armorPieces names each piece's model, in the order Armor.textures lists
// them, which is also the order they are drawn.
var armorPieces = [4]string{
	"geometry.humanoid.armor.helmet",
	"geometry.humanoid.armor.chestplate",
	"geometry.humanoid.armor.leggings",
	"geometry.humanoid.armor.boots",
}

func (a Armor) textures() [4]image.Image {
	return [4]image.Image{a.Helmet, a.Chestplate, a.Leggings, a.Boots}
}

// Held items are drawn the way the game draws a flat item: every opaque
// pixel of the sprite becomes a cube one pixel deep. See
// docs/equipment.md#held-items.
const (
	// heldItemLength is how long the sprite is held, edge to edge, in model
	// units, whatever its resolution. An arm is 12.
	heldItemLength = 10.0
	// heldItemPitch tips the sprite forward from upright, in degrees, so a
	// sword points ahead of the fist.
	heldItemPitch = 20.0
	// heldItemBone names the bone the sprite hangs from. It is not a name
	// any skin uses, so poses never move it on their own.
	heldItemBone = "bedrockskin:held_item"
)

// heldItemGeometry builds the model for item held in the right hand of geo:
// geo's skeleton with no cubes, plus a bone under the right arm carrying a
// cube per opaque pixel. ok is false when geo has no right arm to hold it.
func heldItemGeometry(geo Geometry, item image.Image) (Geometry, bool) {
	byName := boneMap(geo)
	var arm, grip Bone
	var hasArm, hasGrip bool
	for _, b := range geo.Bones {
		if !hasArm && sameBone(b.Name, "rightArm") {
			arm, hasArm = b, true
		}
		if !hasGrip && sameBone(b.Name, "rightItem") {
			grip, hasGrip = b, true
		}
	}
	if hasGrip {
		if parent, ok := byName[grip.Parent]; ok {
			arm, hasArm = parent, true
		} else {
			hasGrip = false
		}
	}
	if !hasArm {
		return Geometry{}, false
	}
	// Where the game's rightItem sits on a standard arm, when geo has none.
	pivot := [3]float64{at(arm.Pivot, 0) - 1, at(arm.Pivot, 1) - 7, at(arm.Pivot, 2) + 1}
	if hasGrip {
		pivot = [3]float64{at(grip.Pivot, 0), at(grip.Pivot, 1), at(grip.Pivot, 2)}
	}

	tex := newFastImageTexture(item)
	w, h := float64(tex.width), float64(tex.height)
	s := heldItemLength / w
	// The grip: tool sprites put the handle near the bottom-left corner.
	gx, gy := w*3/16, h*13/16

	var cubes []Cube
	for y := 0; y < tex.height; y++ {
		for x := 0; x < tex.width; x++ {
			// Only pixels that pass the shader's alpha test (see
			// alphaThreshold): alpha/255 >= 0.5 is alpha >= 128.
			if tex.pix[(y*tex.width+x)*4+3] < 128 {
				continue
			}
			cubes = append(cubes, Cube{
				Origin: []float64{
					pivot[0] - s/2,
					pivot[1] + (gy-float64(y)-1)*s,
					pivot[2] - (float64(x)-gx+1)*s,
				},
				Size: []float64{s, s, s},
				UV:   pixelUV(x, y),
			})
		}
	}

	bones := make([]Bone, 0, len(geo.Bones)+1)
	for _, b := range geo.Bones {
		bones = append(bones, Bone{Name: b.Name, Parent: b.Parent, Pivot: b.Pivot, Rotation: b.Rotation})
	}
	bones = append(bones, Bone{
		Name:     heldItemBone,
		Parent:   arm.Name,
		Pivot:    pivot[:],
		Rotation: []float64{heldItemPitch, 0, 0},
		Cubes:    cubes,
	})
	return Geometry{Identifier: heldItemBone, TextureWidth: w, TextureHeight: h, Bones: bones}, true
}

// pixelUV maps all six faces of a cube to the middle of one texel, so nearest
// sampling never strays into a neighbour at the cube's edges.
func pixelUV(x, y int) json.RawMessage {
	face := `{"uv":[` + strconv.Itoa(x) + `.25,` + strconv.Itoa(y) + `.25],"uv_size":[0.5,0.5]}`
	return json.RawMessage(`{"north":` + face + `,"east":` + face + `,"south":` + face +
		`,"west":` + face + `,"up":` + face + `,"down":` + face + `}`)
}
