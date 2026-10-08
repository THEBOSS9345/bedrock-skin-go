package bedrockskin

import (
	"errors"
	"image"
	"strings"

	"github.com/fogleman/fauxgl"
)

// DefaultSize is the output edge length used when Options.Size is zero.
const DefaultSize = 512

// Camera positions the view explicitly, instead of letting View and Angle
// pick a framing. Set Options.Camera only when one of the presets won't do —
// the presets already handle the common cases.
type Camera struct {
	// Yaw rotates the camera around the vertical axis, in degrees. 0 is
	// straight-on front; positive values swing the camera around to bring
	// more of the subject's left side into view.
	Yaw float64
	// Pitch raises the camera, in degrees. 0 is level with the subject's
	// centre; positive values look down and reveal top-facing surfaces.
	Pitch float64
	// FOV is the field of view in degrees. Zero means 35. Small values
	// (~15-20) read as flat and zoomed in; large ones (~50+) add perspective
	// distortion, which suits dramatic close framing.
	FOV float64
	// Margin is how much room to leave around the subject. Zero means 1.5.
	// 1.0 frames as tightly as possible without clipping; larger values pull
	// the camera back.
	Margin float64
}

// Options describes one render. Only Texture is required.
type Options struct {
	// Texture is the decoded skin image. Required. Bedrock skins are
	// normally 64x64 or 128x128, but any size works.
	Texture image.Image

	// Geometry is the skin's model, as returned by ParseGeometry. Leave it
	// nil for a skin that uses a built-in model — DefaultGeometry stands in,
	// which is what a Bedrock client would render. See DefaultGeometry for
	// why this is the common case rather than a convenience.
	Geometry []Geometry

	// Identifier picks which entry of Geometry to render when it holds more
	// than one, which is usual: a bundle typically carries a cape entry plus
	// both arm variants. It matches the identifier named in the skin's
	// resource patch, e.g. "geometry.humanoid.customSlim".
	//
	// The resource patch is the authoritative wide-vs-slim selector. The
	// login packet's ArmSize field is not: real captures show the two
	// disagreeing, with ArmSize reporting "wide" for a skin whose patch
	// names customSlim.
	//
	// Empty, or naming an entry that isn't present, falls back to the entry
	// with the most cubes. See SelectGeometry.
	Identifier string

	// Cape is an equipped cape texture, or nil.
	//
	// The cape mesh comes from a "cape" bone in Geometry, or from the
	// built-in geometry.cape when Geometry has none — which is the usual
	// case, since capes live in their own entry and never travel merged into
	// a custom skin's body. Not drawn for ViewHead or ViewAvatar, which do
	// not show one.
	Cape image.Image

	// View selects the framing: body, chest, head or avatar. Zero means
	// ViewBody. Ignored when Parts is set.
	View View

	// Angle picks the camera preset for View: straight-on or the angled
	// 3-quarter "head icon" look. Zero means the default for the chosen
	// View, which is AngleIso for ViewHead and AngleFront elsewhere.
	// Ignored when Camera is set.
	Angle Angle

	// Parts names exactly which bones to render, e.g.
	// []string{"head", "leftArm"}. Each name pulls in everything parented
	// under it, so "head" also brings a hat or hair bone. Empty means use
	// View instead.
	Parts []string

	// Camera overrides the View/Angle framing with an explicit position.
	Camera *Camera

	// Size is the output edge length in pixels; the image is always square.
	// Zero means DefaultSize.
	Size int

	// Pose moves bones from where the geometry puts them, e.g. a frame of a
	// Motion (see Motion.Pose). Nil is the rest pose.
	Pose Pose

	// Animated holds the extra textures a persona skin's animations carry -
	// the face, and animated body parts. Each draws the geometry entry made
	// for it alongside the main one; a persona skin's head lives only in its
	// face entry. See docs/geometry-format.md#persona-skins.
	Animated []AnimatedTexture

	// Armor is the armor worn over the skin. The zero value wears none.
	// See docs/equipment.md.
	Armor Armor
	// RightHand and LeftHand are the items held in each hand. The zero
	// value holds nothing. See docs/equipment.md#held-items.
	RightHand Held
	LeftHand  Held
	// Scale resizes the figure or any bone. The zero value changes
	// nothing. See docs/equipment.md#scale.
	Scale Scale
	// HideSkin draws the equipment alone - armor, elytra, held items and
	// cape - posed and framed as it would be on the skin, so any piece can
	// be rendered by itself; Parts and View pick which. Texture may then be
	// nil. ErrEmptyView or ErrNoMatchingParts means no equipment was left
	// to draw. See docs/equipment.md#equipment-on-its-own.
	HideSkin bool
}

// AnimatedType is the kind of a skin animation, numbered as the Bedrock
// protocol numbers them.
type AnimatedType int

const (
	AnimatedFace    AnimatedType = 1 // the face: eyes that blink
	AnimatedBody32  AnimatedType = 2 // a 32x32 animated body part
	AnimatedBody128 AnimatedType = 3 // a 128x128 animated body part
)

// AnimatedTexture is one skin animation's image: its frames stacked top to
// bottom, as the client sends it.
type AnimatedTexture struct {
	Type    AnimatedType
	Texture image.Image
}

// entryPrefix is the identifier prefix of the geometry entry an animation
// type draws, e.g. geometry.animated_face_persona-<id>.
func (t AnimatedType) entryPrefix() string {
	switch t {
	case AnimatedFace:
		return "geometry.animated_face"
	case AnimatedBody32:
		return "geometry.animated_32x32"
	case AnimatedBody128:
		return "geometry.animated_128x128"
	}
	return ""
}

// animatedEntry finds the entry an animation type draws.
func animatedEntry(geos []Geometry, t AnimatedType) (Geometry, bool) {
	prefix := t.entryPrefix()
	if prefix == "" {
		return Geometry{}, false
	}
	for _, g := range geos {
		if strings.HasPrefix(g.Identifier, prefix) {
			return g, true
		}
	}
	return Geometry{}, false
}

// Errors returned by Render and the option parsers. Every one describes bad
// caller input rather than an internal failure, so a service can map the whole
// set to a 4xx with errors.Is and without matching on message text.
var (
	// ErrNoTexture is returned by Render when Options.Texture is nil.
	ErrNoTexture = errors.New("bedrockskin: texture is required")

	// ErrNoGeometry means Options.Geometry held no entry that could be
	// rendered. Leaving Geometry nil is not this error - it selects
	// DefaultGeometry.
	ErrNoGeometry = errors.New("bedrockskin: geometry has no usable entries")

	// ErrNoMatchingParts means no bone in the geometry matched Options.Parts,
	// usually a misspelled bone name.
	ErrNoMatchingParts = errors.New("bedrockskin: no bones matched the requested parts")

	// ErrEmptyView means the chosen View scoped to bones that have no cubes -
	// for example ViewHead on geometry with no head bone.
	ErrEmptyView = errors.New("bedrockskin: nothing to render for this view")

	// ErrUnknownView is returned by ParseView for an unrecognised name.
	ErrUnknownView = errors.New("bedrockskin: unknown view")

	// ErrUnknownAngle is returned by ParseAngle for an unrecognised name.
	ErrUnknownAngle = errors.New("bedrockskin: unknown angle")
)

// Render rasterizes a skin into a square image.
//
// The zero-ish case is the common one: with only a Texture set, this renders
// the full body of a standard humanoid, straight on, at 512x512.
//
//	img, err := bedrockskin.Render(bedrockskin.Options{Texture: tex})
//
// Persona skins render in 3D from their poly meshes; pass their animation
// images as Animated to get the head. Geometry whose bones draw nothing falls
// back to a flat crop of the texture (see Render2D) rather than failing.
func Render(opts Options) (image.Image, error) {
	sc, err := opts.scene(opts.Pose)
	if err != nil || sc.flat != nil {
		return sc.flat, err
	}
	eye, center := cameraForYawPitch(sc.framing(), sc.fov, sc.margin, sc.yaw, sc.pitch)
	return rasterize(sc.layers, eye, center, sc.fov, sc.size), nil
}

// scene is everything Render works out before placing the camera: the
// textured layers, the framing, and the output size. flat is set instead for
// geometry with nothing to rasterize (see Render2D). Animation builds one per
// frame and frames them all with one camera.
type scene struct {
	layers                  []layer
	fov, margin, yaw, pitch float64
	size                    int
	flat                    image.Image
}

// layer is triangles drawn with one texture. A scene draws its layers in
// order: the body, any animated persona parts, the armor, the right hand's
// item, the left hand's, then the cape.
type layer struct {
	triangles []*fauxgl.Triangle
	texture   image.Image
}

// framing is what the camera is fitted around: every layer.
func (sc scene) framing() []*fauxgl.Triangle {
	if len(sc.layers) == 1 {
		return sc.layers[0].triangles
	}
	var all []*fauxgl.Triangle
	for _, l := range sc.layers {
		all = append(all, l.triangles...)
	}
	return all
}

func (opts Options) scene(pose Pose) (scene, error) {
	if opts.Texture == nil && !opts.HideSkin {
		return scene{}, ErrNoTexture
	}

	size := opts.Size
	if size <= 0 {
		size = DefaultSize
	}

	geos := opts.Geometry
	if len(geos) == 0 {
		geos = DefaultGeometry()
	}
	geo, ok := SelectGeometry(geos, opts.Identifier)
	if !ok {
		return scene{}, ErrNoGeometry
	}

	view := opts.View
	if view == "" {
		view = ViewBody
	}

	// Part scales and holding an item change the pose of the skin, its
	// armor and the item alike.
	pose = opts.Scale.partsPose(pose)
	type holding struct {
		skel Geometry
		side hand
		held Held
	}
	var held []holding
	for i, h := range [2]Held{opts.RightHand, opts.LeftHand} {
		if h.Item == nil {
			continue
		}
		if skel, arm, ok := hands[i].skeleton(geo); ok {
			pose = hands[i].holdingPose(pose, arm)
			held = append(held, holding{skel, hands[i], h})
		}
	}

	// No cubes and no poly mesh anywhere: bones with nothing to draw. A
	// flat texture crop is the only output left. See
	// docs/design-decisions.md#why-persona-skins-fall-back-to-2d.
	if !geo.HasMesh() && !opts.HideSkin {
		return scene{flat: Render2D(opts.Texture, view, size)}, nil
	}

	var (
		triangles []*fauxgl.Triangle
		fov       = 35.0
		margin    = 1.5
		yaw       float64
		pitch     float64
	)

	include := func(g Geometry) func(string) bool {
		if len(opts.Parts) > 0 {
			return includeForParts(g, opts.Parts)
		}
		return includeForView(g, view)
	}
	var layers []layer
	empty := func() error {
		if len(opts.Parts) > 0 {
			return ErrNoMatchingParts
		}
		return ErrEmptyView
	}
	if !opts.HideSkin {
		triangles = buildTriangles(geo, include(geo), pose)
		layers = append(layers, layer{triangles, opts.Texture})
		drawn := len(triangles)
		for _, a := range opts.Animated {
			if a.Texture == nil {
				continue
			}
			if g, ok := animatedEntry(geos, a.Type); ok && g.Identifier != geo.Identifier {
				tris := buildTriangles(g, include(g), pose)
				layers = append(layers, layer{tris, a.Texture})
				drawn += len(tris)
			}
		}
		// With the skin drawn, equipment never decides whether the view has
		// anything in it: that is the skin's to answer.
		if drawn == 0 {
			return scene{}, empty()
		}
	}
	for i, tex := range opts.Armor.textures() {
		if tex == nil {
			continue
		}
		g := armorGeometry[armorPieces[i]]
		p := pose
		if i == elytraPiece {
			p = elytraPose(pose)
		}
		layers = append(layers, layer{buildTriangles(g, include(g), p), tex})
	}
	for _, h := range held {
		if in := include(h.skel); in == nil || in(h.side.bone) {
			world := boneWorldMatrices(h.skel, pose)[h.side.bone]
			layers = append(layers, layer{buildHeldItem(h.held.Item, h.held.toModel(h.side), world), h.held.Item})
		}
	}
	if len(opts.Parts) == 0 {
		fov, margin = framingFor(view)
	}

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
			angle = defaultAngleFor(view)
		}
		if angle == AngleIso {
			// The iso camera is offset diagonally rather than pulled
			// straight back, so it needs extra margin not to clip a corner.
			margin *= 1.25
		}
		yaw, pitch = angleToYawPitch(angle)
	}
	if opts.Scale.Model > 0 {
		// The camera fits the model's bounds; less room around them draws
		// it larger.
		margin /= opts.Scale.Model
	}

	// The cape is built before the camera, not after: it hangs behind and
	// below the body, so framing on the body alone can push it out of shot.
	var capeTriangles []*fauxgl.Triangle
	if opts.Cape != nil && capeVisibleIn(view, opts.Parts) {
		// Never from the entry already being rendered. A cape normally lives
		// in its own entry, but a custom model can define a "cape" bone in
		// the body itself, and drawing that bone twice - once from the body
		// mesh with the skin texture, once here with the cape texture -
		// leaves the two z-fighting.
		if capeGeo, found := capeGeometryFor(geos, geo); found {
			capeTriangles = buildCapeTriangles(capeGeo, pose)
		}
	}

	if opts.Cape != nil && len(capeTriangles) > 0 {
		layers = append(layers, layer{capeTriangles, opts.Cape})
	}
	if opts.HideSkin {
		drawn := 0
		for _, l := range layers {
			drawn += len(l.triangles)
		}
		if drawn == 0 {
			return scene{}, empty()
		}
	}
	return scene{layers: layers, fov: fov, margin: margin, yaw: yaw, pitch: pitch, size: size}, nil
}

// capeVisibleIn reports whether a framing shows the cape at all. A head or
// avatar crop does not, and building one for those views only put geometry in
// the scene that happened to fall outside the frame.
//
// A Parts request draws exactly the bones it names, so a cape appears only if
// it was asked for by name.
func capeVisibleIn(view View, parts []string) bool {
	if len(parts) > 0 {
		for _, p := range parts {
			if p == "cape" {
				return true
			}
		}
		return false
	}
	return view != ViewHead && view != ViewAvatar
}

// capeGeometryFor finds the entry to draw an equipped cape from, skipping the
// body entry already being rendered and falling back to the built-in
// "geometry.cape".
//
// The fallback is what makes Options.Cape work at all for a skin with a custom
// mesh: capes normally travel in their own entry and are not merged into a
// body, so a custom skin's geometry.json contains no cape bone. Searching only
// the supplied geometry meant those callers got a capeless image back with no
// error and no way to tell why.
func capeGeometryFor(geos []Geometry, body Geometry) (Geometry, bool) {
	for _, g := range geos {
		if g.Identifier == body.Identifier {
			continue
		}
		if b, ok := g.BoneByName("cape"); ok && len(b.Cubes) > 0 {
			return g, true
		}
	}
	return FindCape(DefaultGeometry())
}

// framingFor returns the field of view and margin that suit a given view:
// avatar is a tight zoomed-in crop, head leaves a little more headroom, and
// chest/body need a wider field and more margin to fit a much taller subject.
func framingFor(view View) (fov, margin float64) {
	switch view {
	case ViewAvatar:
		return 25.0, 1.15
	case ViewHead:
		return 30.0, 1.4
	case ViewChest:
		return 35.0, 1.5
	default:
		return 35.0, 1.6
	}
}
