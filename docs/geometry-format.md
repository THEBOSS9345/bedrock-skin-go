# The Bedrock geometry.json format

A model is a flat list of **bones**. Each bone has a name, an optional parent, a pivot point, an optional rotation, and a list of axis-aligned **cubes**. There are no arbitrary meshes — a Minecraft model is boxes, all the way down.

## Two wire formats

Bedrock has shipped two shapes for the same data, and both still appear in live traffic. `ParseGeometry` detects which it has and normalizes both to `[]Geometry`.

### Modern (`format_version` 1.12.0 and later)

Entries live in a `minecraft:geometry` array, each with a `description` block:

```json
{
  "format_version": "1.12.0",
  "minecraft:geometry": [
    {
      "description": {
        "identifier": "geometry.humanoid.custom",
        "texture_width": 64,
        "texture_height": 64
      },
      "bones": [ ... ]
    }
  ]
}
```

### Legacy (pre-1.12)

The identifier is a **top-level key**, and the texture dimensions lose their underscores:

```json
{
  "format_version": "1.8.0",
  "geometry.humanoid.custom": {
    "texturewidth": 64,
    "textureheight": 64,
    "bones": [ ... ]
  }
}
```

Note `texturewidth` / `textureheight` here versus `texture_width` / `texture_height` above. This form was confirmed against a real capture of `2ndBirthday.PartyPlasticCreeper`.

Bone and cube fields are **identical** between the two. Only the outer wrapper differs, which is why normalizing is a small job.

## One file, several models

A geometry file almost always holds more than one entry. A real capture contains three:

| Identifier | Bones | Cubes | What it is |
| --- | --- | --- | --- |
| `geometry.cape` | 3 | 1 | The cape, self-contained |
| `geometry.humanoid.custom` | 17 | 12 | Wide-armed body |
| `geometry.humanoid.customSlim` | 17 | 12 | Slim-armed body |

`SelectGeometry` picks between them. See [design-decisions.md](design-decisions.md#why-select-by-cube-count).

## Bones

```json
{
  "name": "leftArm",
  "parent": "body",
  "pivot": [5, 22, 0],
  "rotation": [0, 0, 0],
  "inflate": 0,
  "mirror": false,
  "cubes": [ ... ]
}
```

| Field | Meaning |
| --- | --- |
| `name` | Unique within the entry. Referenced by parents and by bone scoping. |
| `parent` | Name of the parent bone, or absent for a root. |
| `pivot` | The point this bone rotates around, in model space. |
| `rotation` | Degrees around X, Y, Z, about `pivot`. See [Rotation](#rotation). |
| `inflate` | Grows every cube in the bone outward by this much on all sides. |
| `mirror` | Mirrors every cube in the bone (a cube's own `mirror: true` does too). |
| `cubes` | May be absent or empty — plenty of bones are pure structure. |

Bones with no cubes are completely normal. In the vanilla model, `root`, `waist`, `cape`, `leftItem` and `rightItem` all carry no geometry; they exist to position other things.

The vanilla humanoid bone set is:

```
root, body, waist, head, cape, hat,
leftArm, leftSleeve, leftItem,
rightArm, rightSleeve, rightItem,
leftLeg, leftPants, rightLeg, rightPants,
jacket
```

Custom skins add their own bones to this — ears, tails, wings, hats — parented somewhere in the standard tree. Nothing in this library hardcodes that list; see [views-and-cameras.md](views-and-cameras.md).

### Everything else in a bone

The schema has more than the renderer uses. These are read into `Bone` too, so a whole file can be inspected:

| Field | `Bone` field | Meaning |
| --- | --- | --- |
| `locators` | `Locators map[string]Locator` | Named points: where an item is held, a lead ties, particles start. Written as `[x, y, z]` or `{"offset": [...], "rotation": [...]}`; both read into `Locator`. A malformed one reads as empty instead of failing the model. |
| `bind_pose_rotation` | `BindPoseRotation` | The rest rotation an animation is relative to. |
| `poly_mesh` | `PolyMesh` (raw JSON) | Free-form polygons instead of cubes. **Not drawn.** |
| `texture_meshes` | `TextureMeshes` (raw JSON) | A texture drawn as a flat mesh. **Not drawn.** |

The renderer draws cubes only. Anything else in the file — fields no type covers — is still reachable through a [geometry tree](#picking-values-out-of-a-file).

`Geometry.Children(name)` lists a bone's direct children; `Geometry.Locator(name)` finds a locator on any bone and says which. A model's `visible_bounds_width`, `visible_bounds_height` and `visible_bounds_offset` read into `Geometry.VisibleBounds*`.

## Cubes

```json
{
  "origin": [-4, 24, -4],
  "size": [8, 8, 8],
  "uv": [0, 0],
  "inflate": 0.25,
  "mirror": false,
  "rotation": [0, 0, 45],
  "pivot": [0, 28, 0]
}
```

| Field | Meaning |
| --- | --- |
| `origin` | The cube's minimum corner in model space. |
| `size` | Extent along X, Y, Z. |
| `uv` | Where the cube's faces live in the texture. Two possible forms — below. |
| `inflate` | Overrides the bone's inflate for this cube. |
| `mirror` | Mirrors the cube's texture left to right: every face flips, and east and west trade places. Also set by the bone's `mirror`. |
| `rotation` | Degrees around X, Y, Z, turning the cube about `pivot`. See [Rotation](#rotation). |
| `pivot` | The point the cube turns around, in model space. Defaults to the cube's centre. |

### Inflate and the layer system

`inflate` is how Minecraft does its second skin layer. The overlay bones — `hat`, `jacket`, `leftSleeve`, `rightSleeve`, `leftPants`, `rightPants` — are geometrically *identical* to the body parts underneath, but with `inflate: 0.25`, so they sit just outside and never z-fight.

Their textures are mostly transparent, and that transparency is what makes them work. See the alpha-test discussion in [rendering-pipeline.md](rendering-pipeline.md#alpha-testing).

### Wide vs slim, concretely

The only real difference between the two humanoid variants:

| | Wide (`custom`) | Slim (`customSlim`) |
| --- | --- | --- |
| `leftArm` origin | `[4, 12, -2]` | `[4, 11.5, -2]` |
| `leftArm` size | `[4, 12, 4]` | `[3, 12, 4]` |
| `rightArm` origin | `[-8, 12, -2]` | `[-7, 11.5, -2]` |

One unit narrower, and half a unit lower to keep the shoulder line right.

## UV mapping

A cube's `uv` field takes one of two forms, which is why `Cube.UV` is kept as `json.RawMessage` and resolved at mesh-build time.

### Box UV (the common case)

A single `[u, v]` origin. All six faces are laid out around it in Minecraft's standard unwrapped-box arrangement, derived from the cube's own size `(w, h, d)`:

```
              ┌─────┬─────┐
              │ up  │down │            up:    (u+d,     v,     w, d)
              │ w×d │ w×d │            down:  (u+d+w,   v,     w, d)
        ┌─────┼─────┼─────┼─────┐      west:  (u,       v+d,   d, h)
        │west │north│east │south│      north: (u+d,     v+d,   w, h)
        │ d×h │ w×h │ d×h │ w×h │      east:  (u+d+w,   v+d,   d, h)
        └─────┴─────┴─────┴─────┘      south: (u+d+w+d, v+d,   w, h)
```

`north` is the **front** — the face with the eyes on it. That was not assumed; it was confirmed by decoding a real skin's pixels and checking where the face is painted.

### Per-face UV

Each face gets its own explicit rectangle:

```json
"uv": {
  "north": { "uv": [0, 0],  "uv_size": [8, 8] },
  "south": { "uv": [16, 0], "uv_size": [8, 8] }
}
```

Faces that are absent are simply not drawn. This form is used by custom models whose parts do not fit the box layout.

`Cube.BoxUV()` returns the `[u, v]` origin of a box-UV cube, and `Cube.FaceUVs()` the per-face areas (`uv`, `uv_size`, `uv_rotation`, `material_instance`) by face name, so neither form needs decoding by hand.

### Coordinates are in texture pixels

Both forms give coordinates in **texture pixels**, not normalized 0–1. Conversion to normalized coordinates uses the entry's declared `texture_width` / `texture_height`, which is why those fields matter and why a mismatch between declared and actual texture size skews the whole model.

Real captured geometry sometimes leaves those fields out entirely (seen in format `1.21.0` and `1.8.0` files). Minecraft reads that as 64×64, and so does `ParseGeometry`. Left at zero, every UV divides by zero and the model renders blank, with no error.

## Rotation

Bones and cubes rotate the same way: in degrees, around X, then Y, then Z, about their pivot. A bone's local transform is that rotation about its own origin, then a translation by `ownPivot - parentPivot`; a cube turns about its own `pivot` (its centre if absent) before its bone's transform.

Model space is X-mirrored against the world it is drawn in, so in model space the X and Z angles are negated (see [rendering-pipeline.md](rendering-pipeline.md#model-space-is-x-mirrored)): in standard right-handed terms the rotation is `Rz(-z)·Ry(y)·Rx(-x)`. That is the convention of Blockbench, the reference Bedrock model editor, which loads `rotation: [x, y, z]` as `(-x, -y, z)` in its mirrored world.

In plain terms: a positive X tips a bone's top forward (so a negative X swings a hanging limb forward), and a positive Z takes the right arm out from the body. `TestRotationDirections` pins all three axes to Minecraft's own player animations: riding lifts the legs forward with X `-81` and splays them with Y `±18`, and the idle bob takes the right arm out with a positive Z.

One trap in the code: fauxgl's `Rotate` turns the opposite way to the standard rotation (its matrix is the transpose of the usual one), so `rotationMatrix` passes `+x, -y, +z` to it to produce the rotation above. An earlier version passed the standard signs straight through and rendered every rotation backwards.

This was long the one unverified corner: the first captures all had `rotation: [0, 0, 0]`. It is now checked against Minecraft's own animations as above, and against real captured geometry with rotated bones and cubes (a CubeCraft galaxy costume with tilted rings, planets and stars; Hive and Galaxite cosmetics), which renders identically to an independent three.js implementation of the same convention. `TestCubeRotation` pins the cube case with a procedural model.

Everything else in this document was verified against captures.

## Picking values out of a file

`ParseGeometryTree` reads a whole geometry file — every field, including any no type here covers — and picks values out of it by path, the way bones are picked by name:

```go
tree, err := bedrockskin.ParseGeometryTree(raw)

// One value.
v, ok := tree.Get("geometry.humanoid.custom/bones/rightArm/pivot")
pivot, _ := v.Floats() // [-5 22 0]

// One number in it.
v, _ = tree.Get("geometry.humanoid.custom/bones/rightArm/cubes/0/size/1")
height, _ := v.Float() // 12

// Everything matching, across every model.
for _, v := range tree.Select("*/bones/*/cubes/*/size") {
	fmt.Println(v.Path, v.JSON()) // geometry.cape/bones/cape/cubes/0/size [10,16,1] ...
}

// A whole bone, typed.
var arm bedrockskin.Bone
v, _ = tree.Get("geometry.humanoid.custom/bones/rightArm")
err = v.Decode(&arm)
```

A path is segments separated by `/`. The first picks a model by identifier; the rest walk into it:

| Segment | Picks |
| --- | --- |
| a field name | that field of an object: `description`, `bones`, `pivot`, `locators`, `uv` |
| a number | that element of a list, from 0; `-1` is the last |
| a name | for a list of named things (bones), the one with that `name`: `bones/rightArm` |
| `*` | every model, field or element at that level |

Names match exactly, else case-insensitively, as the game matches bones. An empty path returns the models themselves.

Both wire formats read into the modern shape, so one path works on either: a legacy model's `texturewidth`, `textureheight` and visible bounds move into a `description`, alongside its `identifier`. A legacy model that inherits from another — `"geometry.hat:geometry.humanoid"` — keeps that full name and is also picked by the part before the colon.

Each result is a `GeometryValue`: its canonical `Path` (bones by name, so results read clearly) and its `Value` as `encoding/json` reads JSON — `map[string]any`, `[]any`, `float64`, `string`, `bool` or `nil`. `Float`, `Floats` and `Text` convert; `Decode` fills any type (a `Bone`, `Cube`, `Locator`); `JSON` re-encodes it.

`tree.Identifiers()` lists the models and `tree.Geometries()` returns the typed ones, both in the same order as `ParseGeometry`. A tree is for reading; rendering takes `[]Geometry`.
