# API reference

Every exported symbol in `github.com/THEBOSS9345/bedrock-skin-go`.

```go
import bedrockskin "github.com/THEBOSS9345/bedrock-skin-go"
```

---

## Rendering

### `func Render(opts Options) (image.Image, error)`

The entry point. Rasterizes a skin into a square image.

```go
img, err := bedrockskin.Render(bedrockskin.Options{Texture: tex})
```

With only a texture, that renders the full body of a standard humanoid, straight on, at 512×512.

`Render` handles the awkward cases so callers do not have to:

- `Geometry` nil or empty → falls back to `DefaultGeometry()`.
- Selected geometry draws nothing (bones with neither cubes nor a poly mesh) → falls back to `Render2D` instead of failing.
- Persona skins render in 3D from their poly meshes. Their head is in the face animation's entry, drawn only when `Options.Animated` carries that image.
- `Identifier` matching nothing → falls back to the entry with the most cubes.

Errors:

| Condition | Error |
| --- | --- |
| `Texture` is nil | `ErrNoTexture` |
| Geometry contains no usable entries | `"geometry has no usable entries"` |
| `Parts` matched no bones | `"no bones matched the requested parts"` |
| A view matched no cubes | `"nothing to render for this view"` |

### `type Options`

```go
type Options struct {
	Texture    image.Image
	Geometry   []Geometry
	Identifier string
	Cape       image.Image
	View       View
	Angle      Angle
	Parts      []string
	Camera     *Camera
	Size       int
	Armor      Armor
	RightHand  Held
	LeftHand   Held
	Scale      Scale
}
```

| Field | Zero value behaviour |
| --- | --- |
| `Texture` | **Required.** Nil returns `ErrNoTexture`. |
| `Geometry` | Uses `DefaultGeometry()`. This is correct for most real skins — see [skin-data.md](skin-data.md). |
| `Identifier` | Picks the entry with the most cubes. |
| `Cape` | No cape. Drawn from a `cape` entry other than the body being rendered, falling back to the built-in `geometry.cape` — so an equipped cape still renders for a skin shipping a custom mesh. Skipped for `ViewHead`/`ViewAvatar`, which don't show one. |
| `View` | `ViewBody`. Ignored when `Parts` is set. |
| `Angle` | `defaultAngleFor(View)` — iso for head, front otherwise. Ignored when `Camera` is set. |
| `Parts` | Uses `View` instead. |
| `Camera` | Uses `View`/`Angle` presets. |
| `Size` | `DefaultSize` (512). Output is always square. |
| `Armor` | No armor. One texture per piece, the elytra included; see [equipment.md](equipment.md). |
| `RightHand`, `LeftHand` | Nothing held. An item in each hand, placed as the game places it; see [equipment.md](equipment.md#held-items). |
| `Scale` | Unchanged. The figure's size in the image and per-bone scales; see [equipment.md](equipment.md#scale). |

### `type Armor` and `func ArmorSet(layer1, layer2 image.Image) Armor`

```go
type Armor struct {
	Helmet     image.Image // layer 1
	Chestplate image.Image // layer 1
	Leggings   image.Image // layer 2
	Boots      image.Image // layer 1
	Elytra     image.Image // elytra.png; takes the chestplate's slot
}
```

A nil piece is not worn. `ArmorSet` wears all four armor pieces of one material. See [equipment.md](equipment.md).

### `type Held` and `type ItemAdjust`

```go
type Held struct {
	Item   image.Image // the sprite; nil holds nothing
	Flat   bool        // held flat, as food; false holds it upright, as a sword
	Adjust ItemAdjust
}

type ItemAdjust struct {
	Offset   [3]float64 // model units, as a bone's position
	Rotation [3]float64 // degrees about the grip, as a bone's rotation
	Scale    float64    // about the grip; zero means 1
}
```

See [equipment.md](equipment.md#adjusting-an-item).

### `type Scale`

```go
type Scale struct {
	Model float64            // the figure's size in the image; zero means 1
	Parts map[string]float64 // per bone, ignoring case, carrying its children; 0 hides
}
```

See [equipment.md](equipment.md#scale).

### `type Camera`

```go
type Camera struct {
	Yaw    float64 // degrees; positive swings toward the model's left
	Pitch  float64 // degrees; positive looks down
	FOV    float64 // degrees; 0 keeps the view's preset
	Margin float64 // 1.0 is tightest; 0 keeps the view's preset
}
```

See [views-and-cameras.md](views-and-cameras.md#explicit-cameras).

### `const DefaultSize = 512`

Used when `Options.Size` is zero.

### Errors

Every error `Render` and the option parsers return describes bad caller input, not an internal failure, so a service can map the whole set to a 4xx with `errors.Is` and no message matching.

| Sentinel | Returned when |
| --- | --- |
| `ErrNoTexture` | `Options.Texture` (or `BytesOptions.Texture`) is nil/empty |
| `ErrNoGeometry` | `Geometry` held no renderable entry — note that leaving it nil is *not* this, it selects `DefaultGeometry` |
| `ErrNoMatchingParts` | no bone matched `Options.Parts`, usually a misspelled name |
| `ErrEmptyView` | the chosen `View` scoped to bones with no cubes, e.g. `ViewHead` on headless geometry |
| `ErrUnknownView` | `ParseView` got a name it does not recognise |
| `ErrUnknownAngle` | `ParseAngle` got a name it does not recognise |

```go
img, err := bedrockskin.Render(opts)
switch {
case errors.Is(err, bedrockskin.ErrNoTexture), errors.Is(err, bedrockskin.ErrNoMatchingParts):
	http.Error(w, err.Error(), http.StatusBadRequest)
case err != nil:
	http.Error(w, "render failed", http.StatusInternalServerError)
}
```

`RenderBytes` wraps decode failures with `fmt.Errorf`, so `errors.Is` reaches through those too.

### `func Render2D(texture image.Image, view View, size int) image.Image`

Flat "paper doll" render, cropped from the texture's standard box-UV regions. No geometry involved.

`Render` calls this automatically for geometry that draws nothing, so you rarely need it directly. Reach for it when you explicitly want the flat look, or when you have a texture and know it uses standard proportions.

Honours `ViewHead`/`ViewAvatar` (head only), `ViewChest` (no legs) and `ViewBody` (everything).

---

## Bytes in, bytes out

For callers holding encoded file or wire bytes who want PNG bytes back, without touching `image.Decode` or `png.Encode`. Same renderer, same behaviour — decode and encode folded in.

### `func RenderBytes(opts BytesOptions) ([]byte, error)`

```go
out, err := bedrockskin.RenderBytes(bedrockskin.BytesOptions{
	Texture: textureBytes,   // encoded PNG or JPEG
	Geometry: geometryBytes, // raw geometry.json; nil or "null" is fine
	View:    bedrockskin.ViewAvatar,
	Size:    128,
})
```

Returns encoded PNG bytes. Identical output to the `image.Image` path — there is a test asserting the two are byte-for-byte equal.

`Geometry` takes the raw `geometry.json` bytes and applies `IsEmpty` internally, so a stock skin's field, including the literal `null`, passes straight through to the default model. Malformed geometry is still an error.

### `type BytesOptions`

```go
type BytesOptions struct {
	Texture    []byte // encoded PNG/JPEG. Required.
	Geometry   []byte // raw geometry.json. Nil, empty or "null" uses the default.
	Cape       []byte // encoded PNG/JPEG.
	Animated   []AnimatedTextureBytes // a persona's animation images, encoded: {Type, Texture []byte}
	Armor      ArmorBytes             // each piece encoded; ArmorSetBytes(layer1, layer2) for a set
	RightHand  HeldBytes              // Held with the item encoded
	LeftHand   HeldBytes
	Identifier string
	View       View
	Angle      Angle
	Parts      []string
	Camera     *Camera
	Size       int
}
```

Every field behaves exactly as its `Options` counterpart.

### Animations from bytes

```go
type AnimationBytesOptions struct {
	BytesOptions
	Animation Animator // required; ErrNoAnimation without one
	FPS, Frames, Workers int
}

func RenderGIFBytes(opts AnimationBytesOptions) ([]byte, error)    // the animation as GIF bytes
func RenderFramesPNG(opts AnimationBytesOptions) ([][]byte, error) // every frame as PNG bytes, in order
```

`RenderGIF` and `RenderFrames` with the decode and encode folded in - the same GIF and the same frames, byte for byte. Both have method forms on `AnimationBytesOptions`.

```go
gif, err := bedrockskin.RenderGIFBytes(bedrockskin.AnimationBytesOptions{
	BytesOptions: bedrockskin.BytesOptions{Texture: textureBytes, Size: 256},
	Animation:    bedrockskin.MotionWalk,
})
```

### Skins from the wire

```go
type WireSkin struct {
	SkinData              []byte // raw RGBA, SkinWidth*SkinHeight*4. Required.
	SkinWidth, SkinHeight int
	CapeData              []byte // raw RGBA; empty for no cape
	CapeWidth, CapeHeight int
	Geometry              []byte // SkinGeometryData; empty or "null" is the default model
	ResourcePatch         []byte // SkinResourcePatch: names the entry to draw
	Animations            []WireAnimation // {Type AnimatedType; Data []byte; Width, Height int}
}

func (w WireSkin) Options() (Options, error) // ready to render: set View, Size, ...
func (w WireSkin) Skin() (*Skin, error)      // the invisibility detector's view
```

A resource patch that does not parse falls back to the entry with the most cubes; malformed geometry and images whose data does not match their size are errors, named by field (`skin: ...`, `cape: ...`).

### Writing to a stream

```go
func (o Options) WritePNG(w io.Writer) error
func WriteGIF(w io.Writer, opts AnimationOptions) error // and the method AnimationOptions.WriteGIF
```

The same bytes as `RenderPNG` and `RenderGIF`, written as they are encoded.

### Method forms

Both option types render themselves, if you prefer building and rendering in one expression:

```go
img, err := bedrockskin.Options{Texture: tex}.Render()          // image.Image
raw, err := bedrockskin.Options{Texture: tex}.RenderPNG()       // []byte
raw, err := bedrockskin.BytesOptions{Texture: b}.RenderPNG()    // []byte
```

`Options.RenderPNG` is the useful hybrid: decoded image in, PNG bytes out.

### `func TextureFromRGBA(pix []byte, width, height int) (image.Image, error)`

Wraps raw non-premultiplied RGBA pixel data as an image. **This is the form Bedrock actually sends skins in** — `SkinData` decodes to `width*height*4` bytes with no header, dimensions arriving separately.

```go
raw, err := base64.StdEncoding.DecodeString(data.SkinData)
if err != nil {
	return err
}
tex, err := bedrockskin.TextureFromRGBA(raw, data.SkinImageWidth, data.SkinImageHeight)
```

The slice backs the image directly rather than being copied, so do not modify it afterwards. A length disagreeing with the dimensions is an error rather than a garbled image.

Note that `BytesOptions.Texture` expects an *encoded* image, so wire data goes through this function and the `Options` path instead.

### `func DecodeImage(data []byte) (image.Image, error)`

Decodes PNG or JPEG. Applies **no size limit** — call `ImageDimensions` first for untrusted input, since it reads only the header. See [recipes.md](recipes.md#handling-untrusted-uploads).

### `func ImageDimensions(data []byte) (width, height int, err error)`

Reads an encoded image's pixel dimensions from its header alone, without decoding the pixels.

```go
w, h, err := bedrockskin.ImageDimensions(data)
if err != nil {
	return err
}
if w > 512 || h > 512 {
	return errors.New("skin texture too large")
}
tex, err := bedrockskin.DecodeImage(data)
```

This is the check `DecodeImage` tells untrusted callers to make first, and it is cheap because decoding is where the damage happens: a few-KB PNG can declare enormous dimensions and force a multi-gigabyte allocation the moment it is decoded. `ImageDimensions` is to a texture what `Complexity` is to geometry — the measurement, with the ceiling left to you.

### `func EncodePNG(img image.Image) ([]byte, error)`

Encodes an image as PNG bytes.

---

## Views and angles

### `type View string`

```go
const (
	ViewBody   View = "body"   // full figure
	ViewChest  View = "chest"  // waist-up, arms included
	ViewHead   View = "head"   // head and everything parented under it
	ViewAvatar View = "avatar" // head, tighter framing
)
```

### `type Angle string`

```go
const (
	AngleFront Angle = "front" // straight-on
	AngleIso   Angle = "iso"   // 35° yaw, 25° pitch
)
```

### `func ParseView(raw string) (View, error)` and `func ParseAngle(raw string) (Angle, error)`

Resolve a name — as it arrives in a query string or config file — to a `View` or `Angle`. Matching ignores case and surrounding space.

```go
view, err := bedrockskin.ParseView(r.URL.Query().Get("view"))
if err != nil {
	http.Error(w, err.Error(), http.StatusBadRequest)
	return
}
```

Blank input returns `ViewBody` / the zero `Angle` (which `Options` reads as "the default for this view"). **An unrecognised name is an error**, wrapping `ErrUnknownView` or `ErrUnknownAngle`, rather than a silent fallback: `View` and `Angle` are bare string types that accept anything, so a request for `avatr` would otherwise render a full body and look like the service ignoring its caller.

### `func ParseParts(raw string) []string`

Splits a comma-separated bone list, trimming whitespace and dropping empties.

```go
bedrockskin.ParseParts(" head , leftArm ,, rightArm ") // ["head" "leftArm" "rightArm"]
```

Returns nil for blank input, which `Options.Parts` reads as "everything".

---

## Geometry

### `func ParseGeometry(raw []byte) ([]Geometry, error)`

Parses a `geometry.json` into normalized entries, accepting both Bedrock formats — the modern `minecraft:geometry` array and the pre-1.12 flat top-level-key form — and detecting which it has.

Returns **zero entries with no error** for input that is valid JSON but carries no geometry, including the literal `null` a Bedrock client sends for a built-in model. An error means genuinely malformed JSON.

```go
geos, err := bedrockskin.ParseGeometry(raw)
if err != nil {
	return err // malformed
}
// len(geos) == 0 is fine: pass nil to Render and get the default model.
```

### `func ParseResourcePatch(raw []byte) (ResourcePatch, error)`

Decodes a skin's `SkinResourcePatch` and returns the geometry identifiers it names.

```go
type ResourcePatch struct {
	Default string // body geometry, e.g. "geometry.humanoid.customSlim"
	Cape    string // cape geometry when the patch names one, else ""
}
```

```go
patch, err := bedrockskin.ParseResourcePatch(raw)
if err != nil {
	return err
}
img, err := bedrockskin.Render(bedrockskin.Options{
	Texture:    tex,
	Geometry:   geos,
	Identifier: patch.Default,
})
```

The patch is the **authoritative** wide-vs-slim selector — the login packet's `ArmSize` disagrees with it on real captures, reporting `"wide"` for a skin whose patch names `customSlim`. See [design-decisions.md](design-decisions.md#why-the-resource-patch-beats-armsize).

Empty input or the literal `"null"` returns a zero `ResourcePatch` and no error, the same "nothing was sent" case `IsEmpty` covers for geometry. A patch that parses but names no default is not an error either; check `Default != ""` if you need one, and let `SelectGeometry`'s cube-count fallback handle its absence.

### `func DefaultGeometry() []Geometry`

The vanilla humanoid bundle, captured from a real Bedrock client: `geometry.humanoid.custom` (wide), `geometry.humanoid.customSlim` (slim), and `geometry.cape`.

Parsed once at package init. **The returned slice is shared and must not be modified.**

### `func SelectGeometry(geos []Geometry, identifier string) (Geometry, bool)`

Picks one entry. Matches `identifier` exactly if given; otherwise, or if nothing matches, returns the entry with the **most cubes**.

Returns `false` only when `geos` is empty.

### `func FindCape(geos []Geometry) (Geometry, bool)`

Finds the entry containing a bone literally named `cape` that has at least one cube. Capes always live in their own entry, never merged into the body.

### `func ParseGeometryTree(raw []byte) (*GeometryTree, error)`

Reads a whole geometry file, every field kept, to pick values out of by path. `ErrNoGeometryModels` for valid JSON holding none. See [geometry-format.md](geometry-format.md#picking-values-out-of-a-file).

```go
type GeometryTree struct {
	FormatVersion string
}

func (t *GeometryTree) Select(path string) []GeometryValue  // every match, in file order
func (t *GeometryTree) Get(path string) (GeometryValue, bool) // the first match
func (t *GeometryTree) Identifiers() []string
func (t *GeometryTree) Geometries() ([]Geometry, error)     // as ParseGeometry

type GeometryValue struct {
	Path  string // canonical: "geometry.humanoid.custom/bones/rightArm/pivot"
	Value any    // map[string]any, []any, float64, string, bool or nil
}

func (v GeometryValue) Float() (float64, bool)
func (v GeometryValue) Floats() ([]float64, bool)
func (v GeometryValue) Text() (string, bool)
func (v GeometryValue) Decode(dst any) error
func (v GeometryValue) JSON() []byte
```

### `func IsEmpty(raw []byte) bool`

Reports whether raw geometry carries no mesh at all — empty, whitespace, or the literal `null` (with or without a trailing newline). Use it to separate "this skin has no custom model" from "this upload is broken" before parsing.

```go
if bedrockskin.IsEmpty(raw) {
	geos = nil // built-in model; Render will use the default
} else if geos, err = bedrockskin.ParseGeometry(raw); err != nil {
	return err
}
```

### `func Complexity(geos []Geometry) (bones, cubes int)`

Total bones and cubes across **every** entry, not just the one that will be rendered — `SelectGeometry`'s fallback and `FindCape` both walk the whole document, so the total is what bounds worst-case cost.

The library enforces no limit itself. See [design-decisions.md](design-decisions.md#why-there-are-no-built-in-limits).

```go
if bones, cubes := bedrockskin.Complexity(geos); bones > 2000 || cubes > 5000 {
	return errors.New("geometry too complex")
}
```

---

## Animation

See [animation.md](animation.md) for the motions, the file format, Molang and the conventions.

### `type Pose map[string]BonePose` and `type BonePose`

```go
type BonePose struct {
	Rotation [3]float64 // degrees, added to the bone's own
	Position [3]float64 // model units, added to its offset from its parent
	Scale    [3]float64 // multiplies the bone about its pivot, when Scaled
	Scaled   bool
}
```

A pose for each bone it moves, by name (exact, else case-insensitive). Set `Options.Pose` to render one pose as a still.

### `type Animator`

```go
type Animator interface {
	Duration() float64   // one loop, in seconds
	Pose(t float64) Pose // the pose t seconds in
}
```

What `RenderFrames` and `RenderGIF` take: a built-in `Motion` or an `*Animation`.

### `type Motion` and `func Motions() []Motion`, `func ParseMotion(s string) (Motion, error)`

The built-in motions: `MotionWalk`, `MotionIdle`, `MotionWave`, `MotionSneak`. `ParseMotion` returns `ErrUnknownMotion` for an unrecognised name.

### `func ParseAnimations(raw []byte) (map[string]*Animation, error)`

Reads a Bedrock animation file (a Blockbench export, or a resource pack's), returning its animations by name. Every Molang expression is compiled here, so a syntax error is reported now, naming where it is. `ErrNoAnimations` for a file with none.

### `type Animation`

```go
type Animation struct {
	Name            string
	Loop            bool
	HoldOnLastFrame bool
	Length          float64 // seconds
}
```

**`func (a *Animation) Pose(t float64) Pose`** and **`Duration() float64`**: it satisfies `Animator`.

**`func (a *Animation) Bones() []string`** — the bones it moves, as the file names them.

**`func (a *Animation) MissingBones(g Geometry) []string`** — the bones it moves that `g` lacks, which will do nothing on that model. See [animation.md](animation.md#which-minecraft-animations-work).

### `func ExampleAnimations() map[string]*Animation`

The 33 bundled example animations for the player model, by name (`"animation.player.dance"`). See [animation.md](animation.md#example-animations).

### `func RenderFrames(opts AnimationOptions) ([]image.Image, error)`

Every frame of the animation, sharing one camera fitted around its whole sweep.

### `func RenderGIF(opts AnimationOptions) ([]byte, error)`

The same frames as a looping GIF.

### `type AnimationOptions`

```go
type AnimationOptions struct {
	Options             // everything Render takes; its Pose is ignored
	Animation Animator  // required; ErrNoAnimation without one
	FPS       int       // frames per second; zero means 20
	Frames    int       // zero means one loop
	Workers   int       // frames rasterized at once; zero means GOMAXPROCS, 1 one at a time
}
```

Frames are rasterized in parallel by default; the images are the same at any `Workers`. See [design-decisions.md](design-decisions.md#why-animation-frames-render-in-parallel).

## Invisibility detection

Separate from rendering: given a texture (and, ideally, its geometry), answer *"is this skin invisible, or partly invisible?"* The high-level entry point is `Skin`, which bundles the two and answers the common questions in one call. The `Validate*`/`Is*` functions underneath are for callers who want the raw result or finer control.

### `func NewSkin(texture image.Image, geometry []byte) *Skin`

Bundles a decoded texture with its raw `geometry.json` bytes (`nil` if the skin sends none) for analysis.

```go
skin := bedrockskin.NewSkin(tex, geoBytes)
```

### `func NewSkinWithOptions(texture image.Image, geometry []byte, opts SkinOptions) *Skin`

The same detector, judging by your thresholds instead of the defaults.

```go
type SkinOptions struct {
	MinVisibleFraction float64 // opaque share a part needs; 0 = DefaultMinVisibleFraction
	MinGeometrySize    float64 // size a bone needs; 0 = DefaultMinGeometrySize
	MinVisibleParts    int     // parts needed to stop being suspicious; 0 = DefaultMinVisibleParts
}
```

A zero field takes its `Default*` value, so `SkinOptions{}` is exactly what `NewSkin` uses. The knobs exist for the same reason the library has no size limits: what counts as unacceptable is policy. A lobby showing only head icons might care solely about the head; a strict server might demand every part be near-opaque.

### `type Skin`

The detector. Methods cache their result: the first call runs the analysis, later calls reuse it, so asking several questions of one `Skin` costs a single pass over the texture. Safe to call concurrently from multiple goroutines. Each call returns its own copy of the slices, so you can sort or filter a report without disturbing the cached one. Don't copy a `Skin` after first use — hold the pointer `NewSkin` returns.

- **`func (s *Skin) Report() SkinReport`** — the full breakdown.
- **`func (s *Skin) Parts() []PartReport`** — just `Report().Parts`.
- **`func (s *Skin) OK() bool`** — the single question most callers have.
- **`func (s *Skin) IsInvisible() bool`** — whole skin effectively invisible.
- **`func (s *Skin) IsSuspicious() bool`** — half-invisible: several body parts missing but not fully invisible.
- **`func (s *Skin) InvisibleParts() []string`** — names of the invisible standard body parts.

### `type SkinReport`

```go
type SkinReport struct {
	Verdict      Verdict      `json:"verdict"`
	VisibleParts int          `json:"visible_parts"`
	TotalParts   int          `json:"total_parts"`
	Parts        []PartReport `json:"parts"`
}

func (r SkinReport) OK() bool                // Verdict == VerdictOK
func (r SkinReport) InvisibleParts() []string // standard parts that don't render
```

```json
{
  "verdict": "suspicious",
  "visible_parts": 3,
  "total_parts": 6,
  "parts": [
    {"name": "head", "visibility": "visible", "opaque_ratio": 1,
     "pixels": 384, "transparent_pixels": 0, "from_geometry": true}
  ]
}
```

Everything derivable is a **method, not a field**, so each fact is stated once and the JSON cannot contradict itself. `OK()` and `InvisibleParts()` are computed from `Verdict` and `Parts`.

`VisibleParts` counts only the six standard body parts (`head`, `body`, `leftArm`, `rightArm`, `leftLeg`, `rightLeg`) out of `TotalParts`. Overlay layers (`hat`, `jacket`, `leftSleeve`, …) fold into the part they cover and accessories (`cape`) are ignored, so an opaque cape never masks an invisible body — but `Parts` still lists them for inspection.

### `type Verdict`

```go
const (
	VerdictUnknown Verdict = iota // zero value: nothing analysed
	VerdictOK                     // renders normally
	VerdictSuspicious             // some standard parts missing, not all
	VerdictInvisible              // nothing renders, or only a stray limb
)
```

One value with four states, replacing three independent booleans (`Pass`, `IsInvisible`, `IsSuspicious`) that could express five combinations meaning nothing — invisible *and* suspicious, or passing *while* invisible.

`VerdictUnknown` is the zero value so an uninitialised report **fails closed**: `OK()` returns false for it. Marshals as its lowercase name.

`VerdictSuspicious` is a soft signal — worth logging or reviewing, not necessarily worth rejecting. Gate on `OK()` only if you mean to reject suspicious skins too; otherwise compare against `VerdictInvisible`.

### `type PartReport` and `type PartVisibility`

```go
type PartReport struct {
	Name         string         `json:"name"`
	Visibility   PartVisibility `json:"visibility"`
	OpaqueRatio  float64        `json:"opaque_ratio"`      // 0-1
	Pixels       int            `json:"pixels"`
	Transparent  int            `json:"transparent_pixels"`
	FromGeometry bool           `json:"from_geometry"`
}

func (p PartReport) Visible() bool // Visibility == PartVisible

const (
	PartVisible PartVisibility = iota
	PartInvisible
	PartSuspicious
	PartTiny
)
```

Both enums marshal as lowercase names (`"visible"`, `"tiny"`) and unmarshal from them.

`PartTiny` distinguishes "the geometry defines this part too small to see" from `PartInvisible`'s "the texture is transparent here". It requires geometry: without it there is nothing to measure.

`Parts` order is stable across calls: the six standard parts first in their fixed order, then every other bone sorted by name. Empty slices marshal as `[]`, not `null`.

### Geometry makes detection strict

Pass the skin's real geometry when you have it. It pins the part UV regions to the actual rendered pixels, so a transparent region can't pass just because the geometry maps there, and the tiny-geometry check can judge bones by size. Without geometry the standard vanilla humanoid UV layout is assumed, which is correct for most skins but can't apply the tiny check.

### The validation functions

These are the internals `Skin` delegates to. Most callers only need `Skin`, but they're exported for raw results and thresholds:

- **`func ValidateSkinVisibility(texture image.Image, geomData []byte, minVisibleFraction float64) SkinVisibilityResult`** — per-part visibility (alpha vs the texture). Geometry present → uses real cube UV regions; geometry provided but with zero cubes (a persona skin) → trusted visible; no geometry → standard UV layout.
- **`func ValidateGeometrySize(geomData []byte, minSize float64) GeometrySizeResult`** — flags bones whose world size is below `minSize` as too small to see. Size is the largest axis of the bounding box enclosing the bone's cubes, not a sum of their sizes: summing let a bone of a hundred separately-invisible cubes add up to a passing figure.
- **`func ValidateSkinInvisibility(texture image.Image, geomData []byte) SkinVisibilityResult`** — combines the two: a part is invisible if it's transparent *or* defined by geometry too small to see. This is what `Skin.Report()` uses.
- **`func IsSkinInvisible(texture image.Image) bool`** — texture-only check: invisible when no standard body part renders under the standard layout.
- **`func IsSkinTiny(geomData []byte) bool`** — true when any geometry bone is too small to see.

### `type SkinVisibilityResult`, `type GeometrySizeResult`

The raw results of the validators:

```go
type SkinVisibilityResult struct {
	IsInvisible    bool
	Pass           bool
	Suspicious     bool
	Parts          []SkinPartResult
	VisibleParts   int
	InvisibleParts int
}

type SkinPartResult struct {
	Name        string
	Visible     bool
	Fraction    float64
	Pixels      int
	Transparent int
	FromGeo     bool // resolved from real geometry, not the fallback layout
	Tiny        bool // geometry defines it below DefaultMinGeometrySize
}

type GeometrySizeResult struct {
	Pass       bool
	Violations []GeometryViolation // per-bone below-minimum sizes, sorted by bone name
}
```

`Suspicious` means half-invisible: at least one standard part visible but fewer than `DefaultMinVisibleParts`. Note this is a *soft* signal — `Pass` stays true unless the skin is fully invisible.

### Thresholds

| Constant | Default | Meaning |
| --- | --- | --- |
| `DefaultMinVisibleParts` | 4 | standard parts that must render to avoid being suspicious |
| `DefaultMinVisibleFraction` | 0.5 | opaque fraction required for a part to count as visible |
| `DefaultMinGeometrySize` | 0.5 | world units below which a bone is too small to see |
| `DefaultMinVisibleAlpha` | 0.5/255 | alpha at or above which a pixel counts as opaque |

Persona skins are judged by what they draw: their poly meshes are measured where their UVs point, and the parts drawn only by an animated entry (the head, on every persona skin), whose texture the detector is not given, are trusted. Geometry that parses into bones drawing nothing at all is trusted visible.

That branch tests *parsed bones*, not "the caller supplied some bytes". Geometry that fails to parse, or parses to nothing, is not a persona skin: it falls through to the texture-only standard-layout check, exactly as `nil` geometry does. Treating unreadable geometry as a persona skin let anyone defeat the detector outright by attaching a byte of garbage to `SkinGeometryData`.

A `nil` texture, or one with no pixels, is not a pass either: it reports `VerdictInvisible`, so both `OK()` and a caller gating on `if !skin.IsInvisible()` reject it rather than admitting it.

---

## Types

### `type Geometry`

```go
type Geometry struct {
	Identifier    string
	TextureWidth  float64
	TextureHeight float64
	Bones         []Bone

	VisibleBoundsWidth  float64
	VisibleBoundsHeight float64
	VisibleBoundsOffset []float64
}
```

One normalized model, regardless of which wire format it came from.

**`func (g *Geometry) BoneByName(name string) (Bone, bool)`** — look up a bone.

**`func (g *Geometry) Children(name string) []Bone`** — a bone's direct children, in file order.

**`func (g *Geometry) Locator(name string) (Locator, Bone, bool)`** — a locator on any bone, and the bone it is on.

**`func (g *Geometry) TotalCubes() int`** — cubes across every bone. A persona skin has none; its mesh is poly meshes.

**`func (g *Geometry) HasMesh() bool`** — whether any bone draws something, a cube or a poly mesh. False is when `Render` falls back to `Render2D`.

**`func (b Bone) Mesh() (PolyMesh, bool)`** — the bone's poly mesh, read from `PolyMesh`. See [geometry-format.md](geometry-format.md#poly-meshes).

**`func (m PolyMesh) Polygons() [][]PolyVertex`** — every polygon, each corner looked up:

```go
type PolyVertex struct {
	Position [3]float64 // model space
	Normal   [3]float64 // zero when the corner names no usable normal
	UV       [2]float64 // 0..1, V up, with NormalizedUVs; else texture pixels
}
```

`tri_list` and `quad_list` are resolved; a polygon whose position or UV index is out of range is skipped, as the renderer skips it.

### Persona animation textures

```go
type AnimatedTexture struct {
	Type    AnimatedType // AnimatedFace (1), AnimatedBody32 (2), AnimatedBody128 (3), as the protocol numbers them
	Texture image.Image  // the animation's image, frames stacked top to bottom
}
```

`Options.Animated` takes these. Each draws the `geometry.animated_face_…` / `_32x32_…` / `_128x128_…` entry of the geometry with its own texture, alongside the selected entry.

### `type Bone`

```go
type Bone struct {
	Name     string
	Parent   string
	Pivot    []float64
	Rotation []float64
	Inflate  float64
	Mirror   bool
	Cubes    []Cube

	BindPoseRotation []float64
	Locators         map[string]Locator
	PolyMesh         json.RawMessage // drawn; read with Mesh()
	TextureMeshes    json.RawMessage // parsed, not drawn
}

type Locator struct {
	Offset               []float64
	Rotation             []float64
	IgnoreInheritedScale bool
}
```

See [geometry-format.md](geometry-format.md#everything-else-in-a-bone). `Rotation` follows the geometry convention in [geometry-format.md](geometry-format.md#rotation), as do animation rotations.

### `type Cube`

```go
type Cube struct {
	Origin   []float64
	Size     []float64
	UV       json.RawMessage
	Inflate  *float64
	Mirror   bool
	Rotation []float64
	Pivot    []float64
}
```

`Rotation` turns the cube about `Pivot` (model space; the cube's centre when absent). A cube mirrors when its own `Mirror` or its bone's is set.

**`func (c Cube) BoxUV() (u, v float64, ok bool)`** and **`func (c Cube) FaceUVs() map[string]FaceUV`** read the two uv forms; `FaceUV` has `UV`, `UVSize`, `UVRotation` and `MaterialInstance`.

`UV` stays raw because Bedrock allows two shapes — a `[u,v]` pair or a per-face object — resolved at mesh-build time. `Inflate` is a pointer so an explicit `0` can be told apart from absent, which matters since absent means "inherit the bone's value".

See [geometry-format.md](geometry-format.md) for what these fields mean.
