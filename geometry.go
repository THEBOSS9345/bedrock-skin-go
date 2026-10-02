package bedrockskin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
)

// Bone and Cube mirror Bedrock's geometry.json schema. UV stays raw JSON
// because a cube's "uv" field is either a [u,v] pair or a per-face object,
// resolved in mesh.go. See docs/geometry-format.md.
type Bone struct {
	Name     string    `json:"name"`
	Parent   string    `json:"parent,omitempty"`
	Pivot    []float64 `json:"pivot,omitempty"`
	Rotation []float64 `json:"rotation,omitempty"`
	Inflate  float64   `json:"inflate,omitempty"`
	Mirror   bool      `json:"mirror,omitempty"`
	Cubes    []Cube    `json:"cubes,omitempty"`

	// The rest of the schema, kept so a whole file can be read. The
	// renderer draws cubes only: poly meshes and texture meshes are parsed
	// but not drawn. See docs/geometry-format.md#everything-else-in-a-bone.
	BindPoseRotation []float64          `json:"bind_pose_rotation,omitempty"`
	Locators         map[string]Locator `json:"locators,omitempty"`
	PolyMesh         json.RawMessage    `json:"poly_mesh,omitempty"`
	TextureMeshes    json.RawMessage    `json:"texture_meshes,omitempty"`
}

// Locator is a named point on a bone - where an item is held, a lead ties,
// particles start. A file writes one as just an offset, [x, y, z], or as an
// object with an offset and a rotation; both read into this.
type Locator struct {
	Offset               []float64 `json:"offset"`
	Rotation             []float64 `json:"rotation,omitempty"`
	IgnoreInheritedScale bool      `json:"ignore_inherited_scale,omitempty"`
}

func (l *Locator) UnmarshalJSON(raw []byte) error {
	var offset []float64
	if err := json.Unmarshal(raw, &offset); err == nil {
		*l = Locator{Offset: offset}
		return nil
	}
	type plain Locator
	var p plain
	// A locator the renderer has no use for never fails a whole model: a
	// malformed one reads as empty.
	_ = json.Unmarshal(raw, &p)
	*l = Locator(p)
	return nil
}

type Cube struct {
	Origin  []float64       `json:"origin"`
	Size    []float64       `json:"size"`
	UV      json.RawMessage `json:"uv"`
	Inflate *float64        `json:"inflate,omitempty"`
	Mirror  bool            `json:"mirror,omitempty"`
	// Rotation turns the cube about Pivot, in degrees; Pivot is in model
	// space and defaults to the cube's centre. See
	// docs/geometry-format.md#rotation.
	Rotation []float64 `json:"rotation,omitempty"`
	Pivot    []float64 `json:"pivot,omitempty"`
}

// FaceUV is one face's texture area in a cube's per-face uv form.
type FaceUV struct {
	UV               []float64 `json:"uv"`
	UVSize           []float64 `json:"uv_size,omitempty"`
	UVRotation       float64   `json:"uv_rotation,omitempty"`
	MaterialInstance string    `json:"material_instance,omitempty"`
}

// BoxUV returns the cube's texture origin when its uv is the box form,
// [u, v], which lays all six faces out from that corner.
func (c Cube) BoxUV() (u, v float64, ok bool) {
	var arr []float64
	if json.Unmarshal(c.UV, &arr) != nil || len(arr) < 2 {
		return 0, 0, false
	}
	return arr[0], arr[1], true
}

// FaceUVs returns each face's texture area, by face name (north, east,
// south, west, up, down), when the cube's uv is the per-face form; nil for
// the box form. A face it leaves out is not drawn.
func (c Cube) FaceUVs() map[string]FaceUV {
	var faces map[string]FaceUV
	if json.Unmarshal(c.UV, &faces) != nil {
		return nil
	}
	return faces
}

// Geometry is one normalized model - a body, a cape - regardless of which of
// Bedrock's two wire formats it came from. See ParseGeometry.
type Geometry struct {
	Identifier    string
	TextureWidth  float64
	TextureHeight float64
	Bones         []Bone

	// The visible bounds: the box, in blocks, the game uses to decide the
	// model is on screen. Zero when the file leaves them out.
	VisibleBoundsWidth  float64
	VisibleBoundsHeight float64
	VisibleBoundsOffset []float64
}

// BoneByName returns the bone with the given name, and whether it exists.
func (g *Geometry) BoneByName(name string) (Bone, bool) {
	for _, b := range g.Bones {
		if b.Name == name {
			return b, true
		}
	}
	return Bone{}, false
}

// Children returns the bones whose parent is the named bone, in file order.
func (g *Geometry) Children(name string) []Bone {
	var out []Bone
	for _, b := range g.Bones {
		if b.Parent == name {
			out = append(out, b)
		}
	}
	return out
}

// Locator finds a locator by name on any bone, returning it and the bone it
// is on.
func (g *Geometry) Locator(name string) (Locator, Bone, bool) {
	for _, b := range g.Bones {
		if l, ok := b.Locators[name]; ok {
			return l, b, true
		}
	}
	return Locator{}, Bone{}, false
}

// IsEmpty reports whether raw carries no geometry at all: either nothing, or
// the literal JSON null a Bedrock client sends for a skin whose model is
// built into the client. Both mean "no mesh supplied", not "broken upload" -
// use it to tell those apart before calling ParseGeometry.
//
// See docs/skin-data.md#most-skins-send-no-geometry-at-all.
func IsEmpty(raw []byte) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null"))
}

// Complexity reports total bones and cubes across every entry in geos, which
// is roughly what mesh-building costs. It sums the whole document, not just
// the entry that will be rendered, because SelectGeometry's fallback and
// FindCape both walk all of it.
//
// The library enforces no limit itself - what counts as too large is policy.
// See docs/recipes.md#handling-untrusted-uploads.
func Complexity(geos []Geometry) (bones, cubes int) {
	for _, g := range geos {
		bones += len(g.Bones)
		cubes += g.TotalCubes()
	}
	return bones, cubes
}

// TotalCubes is the number of cubes across every bone in the entry. Zero
// means the entry carries no mesh data at all, which is exactly what a
// persona skin looks like: real bones, no cubes.
func (g *Geometry) TotalCubes() int {
	n := 0
	for _, b := range g.Bones {
		n += len(b.Cubes)
	}
	return n
}

// modernGeometryDoc is Bedrock's format_version >= 1.12.0 shape.
type modernGeometryDoc struct {
	MinecraftGeometry []struct {
		Description struct {
			Identifier          string    `json:"identifier"`
			TextureWidth        float64   `json:"texture_width"`
			TextureHeight       float64   `json:"texture_height"`
			VisibleBoundsWidth  float64   `json:"visible_bounds_width"`
			VisibleBoundsHeight float64   `json:"visible_bounds_height"`
			VisibleBoundsOffset []float64 `json:"visible_bounds_offset"`
		} `json:"description"`
		Bones []Bone `json:"bones"`
	} `json:"minecraft:geometry"`
}

// legacyGeometryEntryRaw is one entry of Bedrock's pre-1.12 flat format: the
// identifier is a top-level key, and texture dimensions lose their
// underscores. See docs/geometry-format.md#legacy-pre-112.
type legacyGeometryEntryRaw struct {
	TextureWidth        float64   `json:"texturewidth"`
	TextureHeight       float64   `json:"textureheight"`
	VisibleBoundsWidth  float64   `json:"visible_bounds_width"`
	VisibleBoundsHeight float64   `json:"visible_bounds_height"`
	VisibleBoundsOffset []float64 `json:"visible_bounds_offset"`
	Bones               []Bone    `json:"bones"`
}

// ParseGeometry parses raw into normalized entries, detecting whichever of
// Bedrock's two formats it is - bone and cube fields are identical between
// them, only the wrapper differs.
//
// Valid JSON carrying no geometry, including the literal "null" a client
// sends for a built-in model, returns zero entries and no error. An error
// means genuinely malformed input.
//
// Entry order is stable for the same input: modern keeps document order,
// legacy sorts by identifier.
func ParseGeometry(raw []byte) ([]Geometry, error) {
	var modern modernGeometryDoc
	if err := json.Unmarshal(raw, &modern); err == nil && len(modern.MinecraftGeometry) > 0 {
		out := make([]Geometry, len(modern.MinecraftGeometry))
		for i, g := range modern.MinecraftGeometry {
			d := g.Description
			out[i] = Geometry{
				Identifier:          d.Identifier,
				TextureWidth:        textureSize(d.TextureWidth),
				TextureHeight:       textureSize(d.TextureHeight),
				Bones:               g.Bones,
				VisibleBoundsWidth:  d.VisibleBoundsWidth,
				VisibleBoundsHeight: d.VisibleBoundsHeight,
				VisibleBoundsOffset: d.VisibleBoundsOffset,
			}
		}
		return out, nil
	}

	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		return nil, err
	}
	var out []Geometry
	for key, val := range top {
		if key == "format_version" {
			continue
		}
		var entry legacyGeometryEntryRaw
		if err := json.Unmarshal(val, &entry); err != nil || len(entry.Bones) == 0 {
			continue
		}
		out = append(out, Geometry{
			Identifier:          key,
			TextureWidth:        textureSize(entry.TextureWidth),
			TextureHeight:       textureSize(entry.TextureHeight),
			Bones:               entry.Bones,
			VisibleBoundsWidth:  entry.VisibleBoundsWidth,
			VisibleBoundsHeight: entry.VisibleBoundsHeight,
			VisibleBoundsOffset: entry.VisibleBoundsOffset,
		})
	}

	// Map order is random; SelectGeometry breaks ties by position.
	// See docs/design-decisions.md#why-legacy-entries-are-sorted.
	sort.Slice(out, func(i, j int) bool { return out[i].Identifier < out[j].Identifier })
	return out, nil
}

// textureSize is a declared texture dimension, or Minecraft's default of 64
// when the geometry leaves it out - real captures do. Left at zero, every UV
// divides by zero and the model renders blank. See
// docs/geometry-format.md#coordinates-are-in-texture-pixels.
func textureSize(v float64) float64 {
	if v > 0 {
		return v
	}
	return 64
}

// SelectGeometry picks the entry matching identifier, falling back to the one
// with the most cubes when identifier is empty or matches nothing.
//
// The fallback is deliberately not geos[0]: bundles commonly list the sparse
// cape entry first, so taking the first would select it and leave every
// head-scoped view empty. See docs/design-decisions.md#why-select-by-cube-count.
func SelectGeometry(geos []Geometry, identifier string) (Geometry, bool) {
	if identifier != "" {
		for _, g := range geos {
			if g.Identifier == identifier {
				return g, true
			}
		}
	}
	if len(geos) > 0 {
		best := geos[0]
		for _, g := range geos[1:] {
			if g.TotalCubes() > best.TotalCubes() {
				best = g
			}
		}
		return best, true
	}
	return Geometry{}, false
}

// FindCape returns the entry holding a bone literally named "cape" that has
// a cube. Capes always live in their own entry, never merged into the body.
func FindCape(geos []Geometry) (Geometry, bool) {
	for _, g := range geos {
		if b, ok := g.BoneByName("cape"); ok && len(b.Cubes) > 0 {
			return g, true
		}
	}
	return Geometry{}, false
}

// ResourcePatch is a skin's decoded SkinResourcePatch: the mapping from
// render slot to geometry identifier that a Bedrock client sends alongside the
// texture. See docs/skin-data.md#the-resource-patch-is-the-authoritative-model-selector.
type ResourcePatch struct {
	// Default is the identifier of the body geometry, e.g.
	// "geometry.humanoid.customSlim". Pass it as Options.Identifier.
	Default string
	// Cape is the cape geometry identifier when the patch names one, and
	// empty otherwise. Most patches name only Default.
	Cape string
}

// resourcePatchDoc is the wire shape: {"geometry":{"default":"...","cape":"..."}}.
type resourcePatchDoc struct {
	Geometry struct {
		Default string `json:"default"`
		Cape    string `json:"cape"`
	} `json:"geometry"`
}

// ParseResourcePatch decodes a skin's resource patch and returns the geometry
// identifiers it names.
//
//	patch, err := bedrockskin.ParseResourcePatch(raw)
//	if err != nil {
//		return err
//	}
//	img, err := bedrockskin.Render(bedrockskin.Options{
//		Texture:    tex,
//		Geometry:   geos,
//		Identifier: patch.Default,
//	})
//
// The patch is the authoritative wide-vs-slim selector, which is why this
// exists rather than leaving callers to hand-roll the struct: the login
// packet's ArmSize field disagrees with it on real captures, reporting "wide"
// for a skin whose patch names customSlim.
//
// Empty input, or the literal "null", returns a zero ResourcePatch and no
// error - the same "nothing was sent" case IsEmpty covers for geometry. A
// patch that parses but names no default is not an error either; check
// Default against "" if you need one, and fall back to SelectGeometry's
// cube-count heuristic when it is absent.
func ParseResourcePatch(raw []byte) (ResourcePatch, error) {
	if IsEmpty(raw) {
		return ResourcePatch{}, nil
	}
	var doc resourcePatchDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return ResourcePatch{}, fmt.Errorf("bedrockskin: resource patch: %w", err)
	}
	return ResourcePatch{Default: doc.Geometry.Default, Cape: doc.Geometry.Cape}, nil
}
