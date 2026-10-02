package bedrockskin

import (
	"encoding/json"
	"math"

	"github.com/fogleman/fauxgl"
)

// uvRect is a texture-pixel-space rectangle for one cube face.
type uvRect struct{ x, y, w, h float64 }

// boxUVRects computes the standard Bedrock "unwrapped box" layout for a cube
// of size (w,h,d) with UV origin (u,v).
//
// See docs/geometry-format.md#box-uv-the-common-case for the layout diagram.
func boxUVRects(u, v, w, h, d float64) map[string]uvRect {
	return map[string]uvRect{
		"up":    {u + d, v, w, d},
		"down":  {u + d + w, v, w, d},
		"west":  {u, v + d, d, h},
		"north": {u + d, v + d, w, h},
		"east":  {u + d + w, v + d, d, h},
		"south": {u + d + w + d, v + d, w, h},
	}
}

// perFaceUVRect is Bedrock's alternative per-face uv object form:
// {"north":{"uv":[u,v],"uv_size":[w,h]}, ...}.
type perFaceUVEntry struct {
	UV     []float64 `json:"uv"`
	UVSize []float64 `json:"uv_size"`
}

func perFaceUVRects(raw json.RawMessage) map[string]uvRect {
	var faces map[string]perFaceUVEntry
	if err := json.Unmarshal(raw, &faces); err != nil {
		return nil
	}
	out := map[string]uvRect{}
	for name, f := range faces {
		if len(f.UV) < 2 {
			continue
		}
		w, h := 0.0, 0.0
		if len(f.UVSize) >= 2 {
			w, h = f.UVSize[0], f.UVSize[1]
		}
		out[name] = uvRect{f.UV[0], f.UV[1], w, h}
	}
	return out
}

// cubeDims returns a cube's size and origin as fixed triples, and whether both
// carry the three components the format requires.
//
// Bedrock's own geometry always does, but a caller relaying an arbitrary
// client's upload has no such guarantee, and indexing the slices blind panics
// on input as small as {"size":[8]}. Every caller skips the cube instead.
// See docs/design-decisions.md#why-malformed-cubes-are-skipped.
func cubeDims(c Cube) (size, origin [3]float64, ok bool) {
	if len(c.Size) < 3 || len(c.Origin) < 3 {
		return size, origin, false
	}
	copy(size[:], c.Size)
	copy(origin[:], c.Origin)
	return size, origin, true
}

func cubeUVRects(c Cube) map[string]uvRect {
	if len(c.UV) == 0 {
		return nil
	}
	var arr []float64
	if err := json.Unmarshal(c.UV, &arr); err == nil && len(arr) >= 2 {
		size, _, ok := cubeDims(c)
		if !ok {
			return nil
		}
		return boxUVRects(arr[0], arr[1], size[0], size[1], size[2])
	}
	return perFaceUVRects(c.UV)
}

// faceCorner maps the parametric corner loop [-1,-1]->[1,-1]->[1,1]->[-1,1]
// to a local 3D offset from the cube's centre, for each of the 6 faces, given
// half-extents hx,hy,hz.
//
// See docs/rendering-pipeline.md#face-geometry - note in particular that the
// bottom of a face pairs with the texture's bottom row; pairing them the
// other way renders every side face upside down.
func faceCorner(face string, u, v, hx, hy, hz float64) fauxgl.Vector {
	switch face {
	case "up":
		return fauxgl.Vector{X: u * hx, Y: hy, Z: v * hz}
	case "down":
		return fauxgl.Vector{X: u * hx, Y: -hy, Z: -v * hz}
	case "north":
		return fauxgl.Vector{X: -u * hx, Y: v * hy, Z: -hz}
	case "south":
		return fauxgl.Vector{X: u * hx, Y: v * hy, Z: hz}
	case "east":
		return fauxgl.Vector{X: hx, Y: v * hy, Z: -u * hz}
	case "west":
		return fauxgl.Vector{X: -hx, Y: v * hy, Z: u * hz}
	}
	return fauxgl.Vector{}
}

var faceOrder = []string{"up", "down", "north", "south", "east", "west"}

// addCube appends one cube's 6 faces as fauxgl triangles (2 per face) to
// triangles, with vertex positions transformed by worldMatrix (the owning
// bone's world transform) and UVs in 0..1 texture space.
//
// Positions are worked out in model space, the cube's own rotation then the
// bone's, and X is negated last: model space is X-mirrored against the
// world. Each face's U is flipped to match, so textures still read the right
// way round. See docs/rendering-pipeline.md#model-space-is-x-mirrored.
func addCube(triangles *[]*fauxgl.Triangle, c Cube, b Bone, worldMatrix fauxgl.Matrix, texW, texH float64) {
	bonePivot, boneInflate := b.Pivot, b.Inflate
	size, origin, ok := cubeDims(c)
	if !ok {
		return
	}
	rects := cubeUVRects(c)
	if rects == nil {
		return
	}
	inflate := boneInflate
	if c.Inflate != nil {
		inflate = *c.Inflate
	}
	sx := size[0] + 2*inflate
	sy := size[1] + 2*inflate
	sz := size[2] + 2*inflate
	hx, hy, hz := sx/2, sy/2, sz/2

	centerAbs := [3]float64{
		origin[0] + size[0]/2,
		origin[1] + size[1]/2,
		origin[2] + size[2]/2,
	}
	center := fauxgl.Vector{X: centerAbs[0], Y: centerAbs[1], Z: centerAbs[2]}
	pivotBone := fauxgl.Vector{X: at(bonePivot, 0), Y: at(bonePivot, 1), Z: at(bonePivot, 2)}
	place := func(local fauxgl.Vector) fauxgl.Vector {
		p := center.Add(local)
		if len(c.Rotation) >= 3 && (c.Rotation[0] != 0 || c.Rotation[1] != 0 || c.Rotation[2] != 0) {
			pivot := center
			if len(c.Pivot) >= 3 {
				pivot = fauxgl.Vector{X: c.Pivot[0], Y: c.Pivot[1], Z: c.Pivot[2]}
			}
			p = rotationMatrix(c.Rotation).MulPosition(p.Sub(pivot)).Add(pivot)
		}
		p = worldMatrix.MulPosition(p.Sub(pivotBone))
		p.X = -p.X
		return p
	}
	// A cube mirrors when it or its bone says so.
	mirror := c.Mirror || b.Mirror

	corners := [4][2]float64{{-1, -1}, {1, -1}, {1, 1}, {-1, 1}}

	for _, face := range faceOrder {
		// Mirroring flips the cube's texture left to right: east and west
		// trade places, and every face flips.
		src := face
		if mirror && face == "east" {
			src = "west"
		} else if mirror && face == "west" {
			src = "east"
		}
		rect, ok := rects[src]
		if !ok {
			continue
		}
		u0, v0, u1, v1 := rect.x, rect.y, rect.x+rect.w, rect.y+rect.h
		// Negating X flips every face; flipping U puts it back. A mirrored
		// cube's own flip cancels that.
		if !mirror {
			u0, u1 = u1, u0
		}
		uvCorners := [4][2]float64{{u0, v1}, {u1, v1}, {u1, v0}, {u0, v0}}

		var verts [4]fauxgl.Vertex
		for i := 0; i < 4; i++ {
			local := faceCorner(face, corners[i][0], corners[i][1], hx, hy, hz)
			verts[i] = fauxgl.Vertex{
				Position: place(local),
				// V is pre-flipped to cancel fauxgl's internal v=1-v. Do not
				// "simplify" this away: the failure mode is a random-looking
				// transparent/opaque pattern, not a cleanly mirrored image.
				// See docs/rendering-pipeline.md#the-texture-coordinate-flip.
				Texture: fauxgl.Vector{X: uvCorners[i][0] / texW, Y: 1 - uvCorners[i][1]/texH},
			}
		}
		*triangles = append(*triangles,
			fauxgl.NewTriangle(verts[0], verts[1], verts[2]),
			fauxgl.NewTriangle(verts[0], verts[2], verts[3]),
		)
	}
}

func at(v []float64, i int) float64 {
	if i < len(v) {
		return v[i]
	}
	return 0
}

const degToRad = math.Pi / 180

// rotationMatrix is a bone's or cube's rotation in model space: X, then Y,
// then Z. In standard right-handed terms it is Rz(-z)·Ry(y)·Rx(-x) -
// Blockbench's (-x, -y, z) seen back through the X mirror addCube applies
// last - which tips a bone's top forward for a positive X, as Minecraft's own
// sneak does. fauxgl's Rotate turns the opposite way to the standard
// rotation (its matrix is the transpose), so the signs here are +x, -y, +z.
// See docs/geometry-format.md#rotation.
func rotationMatrix(r []float64) fauxgl.Matrix {
	return fauxgl.Identity().
		Rotate(fauxgl.Vector{X: 1}, at(r, 0)*degToRad).
		Rotate(fauxgl.Vector{Y: 1}, -at(r, 1)*degToRad).
		Rotate(fauxgl.Vector{Z: 1}, at(r, 2)*degToRad)
}

// boneLocalMatrix builds a bone's local transform: its rotation (see
// rotationMatrix) about the bone's own origin, then a translation by
// ownPivot-parentPivot. A pose adds its rotation to the bone's and its
// position to the offset (see Pose).
func boneLocalMatrix(b Bone, parentPivot []float64, p BonePose) fauxgl.Matrix {
	own := b.Pivot
	offset := fauxgl.Vector{
		X: at(own, 0) - at(parentPivot, 0) + p.Position[0],
		Y: at(own, 1) - at(parentPivot, 1) + p.Position[1],
		Z: at(own, 2) - at(parentPivot, 2) + p.Position[2],
	}
	rot := []float64{at(b.Rotation, 0) + p.Rotation[0], at(b.Rotation, 1) + p.Rotation[1], at(b.Rotation, 2) + p.Rotation[2]}
	m := fauxgl.Identity()
	if p.Scaled {
		m = m.Scale(fauxgl.Vector{X: p.Scale[0], Y: p.Scale[1], Z: p.Scale[2]})
	}
	if rot[0] != 0 || rot[1] != 0 || rot[2] != 0 {
		m = rotationMatrix(rot).Mul(m)
	}
	m = m.Translate(offset)
	return m
}

// boneWorldMatrices composes every bone's absolute transform up the parent
// chain, memoized. fauxgl has no scene graph, so the hierarchy is baked into
// vertex positions instead. The seen set makes a malformed parent cycle
// resolve to identity rather than recursing forever.
//
// See docs/rendering-pipeline.md for the stage-by-stage walkthrough.
func boneWorldMatrices(geo Geometry, pose Pose) map[string]fauxgl.Matrix {
	byName := map[string]Bone{}
	for _, b := range geo.Bones {
		byName[b.Name] = b
	}
	result := map[string]fauxgl.Matrix{}
	var resolve func(name string, seen map[string]bool) fauxgl.Matrix
	resolve = func(name string, seen map[string]bool) fauxgl.Matrix {
		if m, ok := result[name]; ok {
			return m
		}
		b, ok := byName[name]
		if !ok || seen[name] {
			return fauxgl.Identity()
		}
		seen[name] = true
		parentPivot := []float64{}
		parentWorld := fauxgl.Identity()
		if b.Parent != "" {
			if pb, ok := byName[b.Parent]; ok {
				parentPivot = pb.Pivot
				parentWorld = resolve(b.Parent, seen)
			}
		}
		local := boneLocalMatrix(b, parentPivot, pose.of(b.Name))
		world := parentWorld.Mul(local)
		result[name] = world
		return world
	}
	for _, b := range geo.Bones {
		resolve(b.Name, map[string]bool{})
	}
	return result
}

// buildTriangles builds fauxgl triangles for every cube in geo whose bone
// name passes includeBone (nil = include everything), posed by pose (nil is
// the rest pose).
func buildTriangles(geo Geometry, includeBone func(name string) bool, pose Pose) []*fauxgl.Triangle {
	worlds := boneWorldMatrices(geo, pose)
	var triangles []*fauxgl.Triangle
	for _, b := range geo.Bones {
		if includeBone != nil && !includeBone(b.Name) {
			continue
		}
		world := worlds[b.Name]
		for _, c := range b.Cubes {
			addCube(&triangles, c, b, world, geo.TextureWidth, geo.TextureHeight)
		}
	}
	return triangles
}
