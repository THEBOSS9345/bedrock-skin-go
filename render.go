package bedrockskin

import (
	"fmt"
	"image"
	"math"
	"strings"

	"github.com/fogleman/fauxgl"
)

// View selects which part of the model to render. Bone inclusion is
// ancestry-based rather than a fixed list, so a custom skin's extra bones
// (ears, tails, wings, party hats) are picked up automatically as long as
// they are parented somewhere under a standard anchor.
//
// See docs/views-and-cameras.md.
type View string

const (
	ViewBody   View = "body"   // full figure
	ViewChest  View = "chest"  // waist-up / bust, arms included
	ViewHead   View = "head"   // head only (+ descendants: hat, custom ear/horn/helmet bones, etc.)
	ViewAvatar View = "avatar" // square head icon, closer-framed than ViewHead
)

func boneMap(geo Geometry) map[string]Bone {
	m := make(map[string]Bone, len(geo.Bones))
	for _, b := range geo.Bones {
		m[b.Name] = b
	}
	return m
}

// sameBone compares bone names ignoring ASCII case, as Bedrock does: persona
// models name their limbs "leftarm" where vanilla says "leftArm". See
// docs/geometry-format.md#bone-names-ignore-case.
func sameBone(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		x, y := a[i], b[i]
		if 'A' <= x && x <= 'Z' {
			x += 'a' - 'A'
		}
		if 'A' <= y && y <= 'Z' {
			y += 'a' - 'A'
		}
		if x != y {
			return false
		}
	}
	return true
}

func isDescendant(byName map[string]Bone, name, ancestor string) bool {
	seen := map[string]bool{}
	cur := name
	for cur != "" && !seen[cur] {
		if sameBone(cur, ancestor) {
			return true
		}
		seen[cur] = true
		cur = byName[cur].Parent
	}
	return false
}

func includeForView(geo Geometry, view View) func(name string) bool {
	byName := boneMap(geo)
	switch view {
	case ViewHead, ViewAvatar:
		return func(name string) bool { return isDescendant(byName, name, "head") }
	case ViewChest:
		// A bust: head, torso and both arms with their own descendants
		// (sleeves, held-item locators, arm-mounted custom bones).
		return func(name string) bool {
			return isDescendant(byName, name, "head") ||
				isDescendant(byName, name, "leftArm") ||
				isDescendant(byName, name, "rightArm") ||
				sameBone(name, "body") || sameBone(name, "waist")
		}
	default: // ViewBody
		return nil // include everything
	}
}

// includeForParts builds an inclusion filter from a caller-supplied bone
// list. Each named bone and everything parented under it is included, the
// same way includeForView works. An empty list means everything.
func includeForParts(geo Geometry, parts []string) func(name string) bool {
	if len(parts) == 0 {
		return nil
	}
	byName := boneMap(geo)
	return func(name string) bool {
		for _, p := range parts {
			if isDescendant(byName, name, p) {
				return true
			}
		}
		return false
	}
}

// ParseParts splits a comma-separated bone list into trimmed, non-empty
// names. Blank input returns nil, which Options.Parts reads as "everything".
func ParseParts(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	fields := strings.Split(raw, ",")
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if f != "" {
			out = append(out, f)
		}
	}
	return out
}

// ParseView resolves a view name, as it would arrive in a query string or a
// config file, to a View. Matching ignores case and surrounding space, and
// blank input returns ViewBody.
//
// An unrecognised name is an error wrapping ErrUnknownView rather than a
// silent fallback. Options.View is a bare string type that accepts anything,
// so a request for "avatr" would otherwise render a full body and look like
// the service ignoring its caller.
func ParseView(raw string) (View, error) {
	switch v := View(strings.ToLower(strings.TrimSpace(raw))); v {
	case "":
		return ViewBody, nil
	case ViewBody, ViewChest, ViewHead, ViewAvatar:
		return v, nil
	default:
		return "", fmt.Errorf("%w %q", ErrUnknownView, raw)
	}
}

// ParseAngle resolves an angle name to an Angle, the same way ParseView
// resolves a view. Blank input returns the zero Angle, which Options reads as
// "the default for the chosen view". An unrecognised name wraps
// ErrUnknownAngle.
func ParseAngle(raw string) (Angle, error) {
	switch a := Angle(strings.ToLower(strings.TrimSpace(raw))); a {
	case "":
		return "", nil
	case AngleFront, AngleIso:
		return a, nil
	default:
		return "", fmt.Errorf("%w %q", ErrUnknownAngle, raw)
	}
}

// boundingBoxOf returns the min/max corners spanning every vertex position
// across triangles.
func boundingBoxOf(triangles []*fauxgl.Triangle) (min, max fauxgl.Vector) {
	first := true
	extend := func(p fauxgl.Vector) {
		if first {
			min, max = p, p
			first = false
			return
		}
		min.X, max.X = math.Min(min.X, p.X), math.Max(max.X, p.X)
		min.Y, max.Y = math.Min(min.Y, p.Y), math.Max(max.Y, p.Y)
		min.Z, max.Z = math.Min(min.Z, p.Z), math.Max(max.Z, p.Z)
	}
	for _, t := range triangles {
		extend(t.V1.Position)
		extend(t.V2.Position)
		extend(t.V3.Position)
	}
	return
}

// Angle selects one of the two named camera presets. Options.Camera bypasses
// these entirely and takes explicit yaw/pitch instead.
type Angle string

const (
	AngleFront Angle = "front"
	AngleIso   Angle = "iso"
)

// defaultAngleFor returns the angle used when the caller doesn't pick one.
// Head defaults to iso because a front portrait hides the top of the head,
// which is exactly where custom bones tend to sit; everything else reads
// better straight on. See docs/views-and-cameras.md#angles.
func defaultAngleFor(view View) Angle {
	if view == ViewHead {
		return AngleIso
	}
	return AngleFront
}

// isoYawDegrees/isoPitchDegrees show three faces at once (front, top, one
// side) without foreshortening any of them away to nothing.
const (
	isoYawDegrees   = 35.0
	isoPitchDegrees = 25.0
)

func angleToYawPitch(angle Angle) (yawDegrees, pitchDegrees float64) {
	if angle == AngleIso {
		return isoYawDegrees, isoPitchDegrees
	}
	return 0, 0
}

// cameraForYawPitch frames the camera from the triangles' actual bounding
// box, never a hardcoded distance - a fixed distance works only for models
// whose extent you already assumed.
//
// yaw=0,pitch=0 sits the camera on the -Z side looking toward +Z, up=+Y.
// Positive yaw swings the eye toward -X, the model's left once model space is
// mirrored into the world; positive pitch raises it to look down. See docs/rendering-pipeline.md#stage-4--framing-the-camera for how
// that baseline was established.
func cameraForYawPitch(triangles []*fauxgl.Triangle, fovDegrees, marginFactor, yawDegrees, pitchDegrees float64) (eye, center fauxgl.Vector) {
	min, max := boundingBoxOf(triangles)
	return cameraForBounds(min, max, fovDegrees, marginFactor, yawDegrees, pitchDegrees)
}

// cameraForBounds is cameraForYawPitch with the bounding box already worked
// out, so a caller holding one - Frames, across many draws - can refit the
// camera without walking the triangles again.
func cameraForBounds(min, max fauxgl.Vector, fovDegrees, marginFactor, yawDegrees, pitchDegrees float64) (eye, center fauxgl.Vector) {
	center = fauxgl.Vector{X: (min.X + max.X) / 2, Y: (min.Y + max.Y) / 2, Z: (min.Z + max.Z) / 2}
	halfExtent := math.Max((max.X-min.X)/2, math.Max((max.Y-min.Y)/2, (max.Z-min.Z)/2))
	if halfExtent <= 0 {
		halfExtent = 1
	}
	halfFovRad := fovDegrees * math.Pi / 360
	distance := halfExtent / math.Tan(halfFovRad) * marginFactor

	yaw, pitch := yawDegrees*math.Pi/180, pitchDegrees*math.Pi/180
	offset := fauxgl.Vector{
		X: -distance * math.Sin(yaw) * math.Cos(pitch),
		Y: distance * math.Sin(pitch),
		Z: -distance * math.Cos(yaw) * math.Cos(pitch),
	}
	eye = fauxgl.Vector{X: center.X + offset.X, Y: center.Y + offset.Y, Z: center.Z + offset.Z}
	return eye, center
}

// rasterize does the GPU-free drawing, given resolved triangles and camera
// parameters. capeTriangles/capeTexture may be nil to skip the cape.
func rasterize(layers []layer, eye, center fauxgl.Vector, fovDegrees float64, size int) image.Image {
	// Clip space only - do NOT chain .Viewport(...) here. The rasterizer
	// applies the NDC->screen mapping itself after the perspective divide;
	// adding one before the divide renders a fully blank image.
	// See docs/design-decisions.md#why-no-viewport-in-the-shader-matrix.
	matrix := fauxgl.LookAt(eye, center, fauxgl.Vector{Y: 1}).
		Perspective(fovDegrees, 1.0, 1, 500)

	// One triangle at a time, in order: drawing order decides ties in the
	// depth test, so it is part of the image. See
	// docs/design-decisions.md#why-rasterization-is-single-threaded.
	r := newRaster(size, size)
	for _, l := range layers {
		if len(l.triangles) == 0 {
			continue
		}
		tex := newFastImageTexture(l.texture)
		for _, t := range l.triangles {
			r.drawTriangle(t, matrix, tex)
		}
	}
	return r.color
}

// buildCapeTriangles builds the cape mesh. A cape entry is its own bone
// chain (waist -> body -> cape), hung on skel's skeleton by capeOnSkeleton so
// a pose moves it with the skin.
func buildCapeTriangles(capeGeo, skel Geometry, pose Pose) []*fauxgl.Triangle {
	include := func(name string) bool { return name == "cape" }
	return buildTriangles(capeOnSkeleton(capeGeo, skel), include, pose)
}

// capeOnSkeleton is capeGeo with the bones skel has above it added, without
// their cubes. The cape's chain stops at the waist, while the skin's goes on
// up to a root that animations move - swimming, sitting, sneaking - so a
// cape left on its own chain stayed where the skin had been. See
// docs/equipment.md#capes-follow-the-skin.
func capeOnSkeleton(capeGeo, skel Geometry) Geometry {
	have := make(map[string]bool, len(capeGeo.Bones))
	for _, b := range capeGeo.Bones {
		have[b.Name] = true
	}
	skelBones := make(map[string]Bone, len(skel.Bones))
	for _, b := range skel.Bones {
		if _, dup := skelBones[b.Name]; !dup {
			skelBones[b.Name] = b
		}
	}
	bones := append([]Bone(nil), capeGeo.Bones...)
	for i := range capeGeo.Bones {
		b := bones[i]
		if b.Parent != "" && have[b.Parent] {
			continue
		}
		sb, ok := skelBones[b.Name]
		if !ok || sb.Parent == "" || have[sb.Parent] {
			continue
		}
		bones[i].Parent = sb.Parent
		for name := sb.Parent; name != "" && !have[name]; {
			up, ok := skelBones[name]
			if !ok {
				break
			}
			have[name] = true
			bones = append(bones, Bone{Name: up.Name, Parent: up.Parent, Pivot: up.Pivot, Rotation: up.Rotation})
			name = up.Parent
		}
	}
	out := capeGeo
	out.Bones = bones
	return out
}
