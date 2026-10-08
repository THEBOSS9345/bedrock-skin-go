package bedrockskin

import (
	"bytes"
	_ "embed"
	"fmt"
	"image"
	"math"
	"sort"

	"github.com/fogleman/fauxgl"
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
	// Elytra is the elytra's texture (textures/models/armor/elytra.png),
	// worn on the back. It takes the chestplate's slot, as in game: with
	// both set, only the elytra is worn. See docs/equipment.md#elytra.
	Elytra image.Image
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
var armorPieces = [5]string{
	"geometry.humanoid.armor.helmet",
	"geometry.humanoid.armor.chestplate",
	"geometry.humanoid.armor.leggings",
	"geometry.humanoid.armor.boots",
	"geometry.elytra",
}

// elytraPiece is the elytra's index in armorPieces.
const elytraPiece = 4

func (a Armor) textures() [5]image.Image {
	chest := a.Chestplate
	if a.Elytra != nil {
		chest = nil
	}
	return [5]image.Image{a.Helmet, chest, a.Leggings, a.Boots, a.Elytra}
}

// elytraPose is pose with the elytra's own resting pose on top, vanilla's
// animation.elytra.default: the body bone scaled up, the wings spread out
// and back. See docs/equipment.md#elytra.
func elytraPose(pose Pose) Pose {
	return pose.with(map[string]BonePose{
		"body":       {Scale: [3]float64{1.067, 1.067, 1.067}, Scaled: true},
		"left_wing":  {Position: [3]float64{4.5, 4, -2}, Rotation: [3]float64{15, 0, -13}, Scale: [3]float64{1, 1, 2}, Scaled: true},
		"right_wing": {Position: [3]float64{-4.5, 4, -2}, Rotation: [3]float64{15, 0, 13}, Scale: [3]float64{1, 1, 2}, Scaled: true},
	})
}

// Scale resizes the figure or any of its bones. The zero value changes
// nothing. A held item has its own scale, in ItemAdjust. See
// docs/equipment.md#scale.
type Scale struct {
	// Model is the figure's size in the image: 2 draws it twice as large,
	// cropping what no longer fits; 0.5 half as large. Zero means 1. The
	// camera frames the model whatever its size, so only this changes how
	// big it looks.
	Model float64
	// Parts scales bones by name, ignoring case, each about its own pivot
	// and carrying everything parented under it: the armor on it, and an
	// arm's held item. 0 hides a bone.
	Parts map[string]float64
}

// partsPose is pose with Scale.Parts applied.
func (s Scale) partsPose(pose Pose) Pose {
	if len(s.Parts) == 0 {
		return pose
	}
	extra := make(map[string]BonePose, len(s.Parts))
	for name, k := range s.Parts {
		extra[name] = BonePose{Scale: [3]float64{k, k, k}, Scaled: true}
	}
	return pose.with(extra)
}

// then is p with q applied after it: rotations and positions add, scales
// multiply.
func (p BonePose) then(q BonePose) BonePose {
	out := BonePose{
		Rotation: [3]float64{p.Rotation[0] + q.Rotation[0], p.Rotation[1] + q.Rotation[1], p.Rotation[2] + q.Rotation[2]},
		Position: [3]float64{p.Position[0] + q.Position[0], p.Position[1] + q.Position[1], p.Position[2] + q.Position[2]},
		Scale:    p.Scale,
		Scaled:   p.Scaled || q.Scaled,
	}
	switch {
	case p.Scaled && q.Scaled:
		out.Scale = [3]float64{p.Scale[0] * q.Scale[0], p.Scale[1] * q.Scale[1], p.Scale[2] * q.Scale[2]}
	case q.Scaled:
		out.Scale = q.Scale
	}
	return out
}

// with is a copy of p with each of extra applied after the pose p already
// gives that bone. A bone's entry is found as Pose.of finds it and stored
// under the name extra uses, so no other spelling of the name shadows it.
// extra is applied in name order, so the result never depends on map order.
func (p Pose) with(extra map[string]BonePose) Pose {
	out := make(Pose, len(p)+len(extra))
	for name, bp := range p {
		out[name] = bp
	}
	names := make([]string, 0, len(extra))
	for name := range extra {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		bp := out.of(name).then(extra[name])
		for other := range out {
			if sameBone(other, name) {
				delete(out, other)
			}
		}
		out[name] = bp
	}
	return out
}

// Held is an item held in one hand. The zero value holds nothing. See
// docs/equipment.md#held-items.
type Held struct {
	// Item is the item's sprite, e.g. textures/items/diamond_sword.png,
	// drawn extruded and placed where the game places it, with the arm
	// held forward. Nil holds nothing; nor does geometry without the arm.
	Item image.Image
	// Flat holds it as the game holds an item that is not a tool or
	// weapon - food, materials. False holds it upright, as a sword.
	Flat bool
	// Adjust moves the item from where the game puts it, for an item that
	// placement does not suit.
	Adjust ItemAdjust
}

// ItemAdjust moves a held item from the game's placement, about its grip and
// in the hand's frame, so it follows the arm. The zero value moves nothing.
// See docs/equipment.md#adjusting-an-item.
type ItemAdjust struct {
	// Offset moves the item, in model units along the model's axes, as a
	// bone's position moves a bone.
	Offset [3]float64
	// Rotation turns the item about its grip, in degrees, as a bone's
	// rotation turns a bone: a positive X tips its top forward.
	Rotation [3]float64
	// Scale resizes the item about its grip. Zero means 1.
	Scale float64
}

// hand is what differs between the hands: the bone names, where a model
// without an item bone grips, and the game's display transforms.
type hand struct {
	arm, item string // the model's bones, matched ignoring case
	bone      string // the item's own bone; no skin uses the name
	gripX     float64
	// tool and flat take item space (blocks, the sprite's longer side one
	// block) to the hand's frame (blocks), as standard right-handed
	// matrices: tools and weapons upright, anything else flat.
	tool, flat fauxgl.Matrix
}

// hands are the right hand and the left, in that order. The left is the
// game's off hand: not a mirror of the right, but its own offset.
var hands = [2]hand{
	{
		arm: "rightArm", item: "rightItem", bone: "bedrockskin:right_item", gripX: -1,
		tool: mul(scale(-1, -1, 1), rotY(180), translate(0.1, 0.265, 0), scale(0.625, 0.625, 0.625),
			rotX(80), rotY(45), spriteItemTransform()),
		flat: mul(scale(-1, -1, 1), translate(0.3125, 0.1875, -0.1875), scale(0.375, 0.375, 0.375),
			rotZ(60), rotX(-90), rotZ(20), spriteItemTransform()),
	},
	{
		arm: "leftArm", item: "leftItem", bone: "bedrockskin:left_item", gripX: 1,
		tool: mul(scale(-1, -1, 1), translate(-0.125, 0, 0), rotY(180), translate(0, 0.265, 0), scale(0.625, 0.625, 0.625),
			rotX(80), rotY(45), spriteItemTransform()),
		flat: mul(scale(-1, -1, 1), translate(-0.125, 0, 0), translate(0.3125, 0.1875, -0.1875), scale(0.375, 0.375, 0.375),
			rotZ(60), rotX(-90), rotZ(20), spriteItemTransform()),
	},
}

// toModel takes item space to model units relative to the grip, in the
// geometry's frame: the hand's display, X mirrored and scaled by 16, then
// the caller's adjustment.
func (h Held) toModel(side hand) fauxgl.Matrix {
	display := side.tool
	if h.Flat {
		display = side.flat
	}
	return h.Adjust.matrix().Mul(scale(-16, 16, 16)).Mul(display)
}

// matrix is the adjustment as a transform about the origin: scale, then
// rotation, then offset.
func (a ItemAdjust) matrix() fauxgl.Matrix {
	k := a.Scale
	if k == 0 {
		k = 1
	}
	return translate(a.Offset[0], a.Offset[1], a.Offset[2]).
		Mul(rotationMatrix(a.Rotation[:])).
		Mul(scale(k, k, k))
}

// ItemOptions describes a render of one item on its own, extruded as a held
// item is. Only Item is required. See docs/equipment.md#an-item-on-its-own.
type ItemOptions struct {
	// Item is the item's sprite, e.g. textures/items/diamond_sword.png.
	Item image.Image
	// Angle picks the camera preset: AngleFront faces the sprite, as an
	// inventory icon; AngleIso turns it to show its depth. Zero means
	// AngleFront. Ignored when Camera is set.
	Angle Angle
	// Camera overrides Angle with an explicit position.
	Camera *Camera
	// Size is the output edge length; the image is square. Zero means
	// DefaultSize.
	Size int
	// Adjust turns and resizes the item about its centre. The camera frames
	// the item whatever its size or offset, so Scale and Offset change only
	// its shape against other parts, not how big it looks.
	Adjust ItemAdjust
}

// RenderItem renders an item on its own: the sprite extruded one texel deep,
// as the game draws a held item, centred and framed by the camera.
func RenderItem(opts ItemOptions) (image.Image, error) {
	triangles, err := opts.triangles(0)
	if err != nil {
		return nil, err
	}
	fov, margin, yaw, pitch := opts.camera()
	eye, center := cameraForYawPitch(triangles, fov, margin, yaw, pitch)
	return rasterize([]layer{{triangles, opts.Item}}, eye, center, fov, opts.size()), nil
}

func (opts ItemOptions) size() int {
	if opts.Size <= 0 {
		return DefaultSize
	}
	return opts.Size
}

// triangles is the item turned spin degrees about its upright axis, after
// its adjustment.
func (opts ItemOptions) triangles(spin float64) ([]*fauxgl.Triangle, error) {
	if opts.Item == nil {
		return nil, ErrNoTexture
	}
	b := opts.Item.Bounds()
	w, h := float64(b.Dx()), float64(b.Dy())
	if w == 0 || h == 0 {
		return nil, ErrEmptyView
	}
	// Item space centred on the origin, so the adjustment and the spin turn
	// it about its middle; then into model units, X mirrored as a held
	// item's is.
	t := 1 / max(w, h)
	toModel := rotationMatrix([]float64{0, spin, 0}).
		Mul(opts.Adjust.matrix()).
		Mul(scale(-16, 16, 16)).
		Mul(translate(w*t/2, -h*t/2, t/2))
	return buildHeldItem(opts.Item, toModel, fauxgl.Identity()), nil
}

// camera is the item's framing: the field of view, margin, yaw and pitch.
func (opts ItemOptions) camera() (fov, margin, yaw, pitch float64) {
	fov, margin = 35.0, 1.2
	if opts.Camera != nil {
		yaw, pitch = opts.Camera.Yaw, opts.Camera.Pitch
		if opts.Camera.FOV > 0 {
			fov = opts.Camera.FOV
		}
		if opts.Camera.Margin > 0 {
			margin = opts.Camera.Margin
		}
	} else {
		angle := opts.Angle
		if angle == "" {
			angle = AngleFront
		}
		if angle == AngleIso {
			margin *= 1.25
		}
		yaw, pitch = angleToYawPitch(angle)
	}
	return fov, margin, yaw, pitch
}

// ItemAnimationOptions spins an item on its own: one full turn about its
// upright axis each loop, as a dropped item turns. See
// docs/equipment.md#an-item-on-its-own.
type ItemAnimationOptions struct {
	ItemOptions

	// Duration is how long one turn takes, in seconds. Zero means 3.
	Duration float64
	// FPS is frames per second; zero means 20.
	FPS int
	// Frames is how many frames to render; zero means one turn.
	Frames int
}

// RenderItemFrames renders the spinning item frame by frame. Every frame
// shares one camera, fitted around the whole turn, so the item turns in a
// still frame.
func RenderItemFrames(opts ItemAnimationOptions) ([]image.Image, error) {
	duration := opts.Duration
	if duration <= 0 {
		duration = 3
	}
	fps, frames := opts.FPS, opts.Frames
	if fps <= 0 {
		fps = 20
	}
	if frames <= 0 {
		frames = max(1, int(math.Round(duration*float64(fps))))
	}
	turns := make([][]*fauxgl.Triangle, frames)
	var sweep []*fauxgl.Triangle
	for i := range turns {
		spin := 360 * (float64(i) / float64(fps)) / duration
		tris, err := opts.triangles(spin)
		if err != nil {
			return nil, err
		}
		turns[i] = tris
		sweep = append(sweep, tris...)
	}
	fov, margin, yaw, pitch := opts.camera()
	eye, center := cameraForYawPitch(sweep, fov, margin, yaw, pitch)
	out := make([]image.Image, frames)
	for i, tris := range turns {
		out[i] = rasterize([]layer{{tris, opts.Item}}, eye, center, fov, opts.size())
	}
	return out, nil
}

// RenderItemGIF renders the spinning item as a looping animated GIF, as
// RenderGIF encodes one.
func RenderItemGIF(opts ItemAnimationOptions) ([]byte, error) {
	frames, err := RenderItemFrames(opts)
	if err != nil {
		return nil, err
	}
	fps := opts.FPS
	if fps <= 0 {
		fps = 20
	}
	var buf bytes.Buffer
	if err := encodeGIF(&buf, frames, fps); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// spriteItemTransform is the game's legacy item transform, applied before
// either display.
func spriteItemTransform() fauxgl.Matrix {
	return mul(scale(1.5, 1.5, 1.5), rotY(50), rotZ(335), translate(0.075, -0.245, -0.1))
}

func mul(ms ...fauxgl.Matrix) fauxgl.Matrix {
	out := ms[0]
	for _, m := range ms[1:] {
		out = out.Mul(m)
	}
	return out
}

func translate(x, y, z float64) fauxgl.Matrix {
	return fauxgl.Matrix{X00: 1, X03: x, X11: 1, X13: y, X22: 1, X23: z, X33: 1}
}

func scale(x, y, z float64) fauxgl.Matrix {
	return fauxgl.Matrix{X00: x, X11: y, X22: z, X33: 1}
}

// rotX, rotY and rotZ are the standard right-handed rotations, in degrees -
// not fauxgl's Rotate, which turns the other way (see rotationMatrix).
func rotX(deg float64) fauxgl.Matrix {
	s, c := math.Sin(deg*degToRad), math.Cos(deg*degToRad)
	return fauxgl.Matrix{X00: 1, X11: c, X12: -s, X21: s, X22: c, X33: 1}
}

func rotY(deg float64) fauxgl.Matrix {
	s, c := math.Sin(deg*degToRad), math.Cos(deg*degToRad)
	return fauxgl.Matrix{X00: c, X02: s, X11: 1, X20: -s, X22: c, X33: 1}
}

func rotZ(deg float64) fauxgl.Matrix {
	s, c := math.Sin(deg*degToRad), math.Cos(deg*degToRad)
	return fauxgl.Matrix{X00: c, X01: -s, X10: s, X11: c, X22: 1, X33: 1}
}

// holdingPose is pose with the hand's arm held out as vanilla's
// animation.player.holding holds it: the arm's X turn becomes this*0.5 - 18,
// half its swing and 18 degrees forward. arm is the model's name for it.
func (h hand) holdingPose(pose Pose, arm string) Pose {
	x := pose.of(arm).Rotation[0]
	return pose.with(map[string]BonePose{h.arm: {Rotation: [3]float64{-x*0.5 - 18, 0, 0}}})
}

// skeleton is geo's skeleton with no cubes, plus a bone at the hand's grip
// for the item: the model's item bone, else where the standard arm's would
// be. arm is the model's name for the arm; ok is false when geo has none.
func (h hand) skeleton(geo Geometry) (skel Geometry, arm string, ok bool) {
	byName := boneMap(geo)
	var armBone, grip Bone
	var hasArm, hasGrip bool
	for _, b := range geo.Bones {
		if !hasArm && sameBone(b.Name, h.arm) {
			armBone, hasArm = b, true
		}
		if !hasGrip && sameBone(b.Name, h.item) {
			grip, hasGrip = b, true
		}
	}
	if hasGrip {
		if parent, found := byName[grip.Parent]; found {
			armBone, hasArm = parent, true
		} else {
			hasGrip = false
		}
	}
	if !hasArm {
		return Geometry{}, "", false
	}
	pivot := []float64{at(armBone.Pivot, 0) + h.gripX, at(armBone.Pivot, 1) - 7, at(armBone.Pivot, 2) + 1}
	if hasGrip {
		pivot = []float64{at(grip.Pivot, 0), at(grip.Pivot, 1), at(grip.Pivot, 2)}
	}
	bones := make([]Bone, 0, len(geo.Bones)+1)
	for _, b := range geo.Bones {
		bones = append(bones, Bone{Name: b.Name, Parent: b.Parent, Pivot: b.Pivot, Rotation: b.Rotation})
	}
	bones = append(bones, Bone{Name: h.bone, Parent: armBone.Name, Pivot: pivot})
	return Geometry{Identifier: h.bone, Bones: bones}, armBone.Name, true
}

// buildHeldItem builds the sprite's triangles: a front and a back face over
// the whole sprite, and an edge strip along every side of an opaque texel
// that has no opaque neighbour there. Opaque means passing the shader's
// alpha test (see alphaThreshold): alpha/255 >= 0.5, a byte of 128 or more.
func buildHeldItem(item image.Image, toModel, world fauxgl.Matrix) []*fauxgl.Triangle {
	tex := newFastImageTexture(item)
	w, h := tex.width, tex.height
	if w == 0 || h == 0 {
		return nil
	}
	t := 1 / float64(max(w, h))
	vertex := func(x, y, z, u, v float64) fauxgl.Vertex {
		p := world.MulPosition(toModel.MulPosition(fauxgl.Vector{X: x, Y: y, Z: z}))
		p.X = -p.X
		// V is pre-flipped, as in addCube.
		return fauxgl.Vertex{Position: p, Texture: fauxgl.Vector{X: u, Y: 1 - v}}
	}
	var tris []*fauxgl.Triangle
	quad := func(a, b, c, d fauxgl.Vertex) {
		tris = append(tris, fauxgl.NewTriangle(a, b, c), fauxgl.NewTriangle(a, c, d))
	}
	// Column c spans X from -c*t to -(c+1)*t, row r spans Y from (h-r)*t
	// down to (h-r-1)*t, and the slab runs from Z=0 back to Z=-t.
	x0, x1, y1 := 0.0, -float64(w)*t, float64(h)*t
	for _, z := range []float64{0, -t} {
		quad(vertex(x0, 0, z, 0, 1), vertex(x1, 0, z, 1, 1), vertex(x1, y1, z, 1, 0), vertex(x0, y1, z, 0, 0))
	}
	opaque := func(c, r int) bool {
		return c >= 0 && r >= 0 && c < w && r < h && tex.pix[(r*w+c)*4+3] >= 128
	}
	for r := 0; r < h; r++ {
		for c := 0; c < w; c++ {
			if !opaque(c, r) {
				continue
			}
			u, v := (float64(c)+0.5)/float64(w), (float64(r)+0.5)/float64(h)
			left, right := -float64(c)*t, -float64(c+1)*t
			top, bottom := float64(h-r)*t, float64(h-r-1)*t
			edge := func(xa, ya, xb, yb float64) {
				quad(vertex(xa, ya, 0, u, v), vertex(xa, ya, -t, u, v), vertex(xb, yb, -t, u, v), vertex(xb, yb, 0, u, v))
			}
			if !opaque(c-1, r) {
				edge(left, bottom, left, top)
			}
			if !opaque(c+1, r) {
				edge(right, bottom, right, top)
			}
			if !opaque(c, r-1) {
				edge(left, top, right, top)
			}
			if !opaque(c, r+1) {
				edge(left, bottom, right, bottom)
			}
		}
	}
	return tris
}
