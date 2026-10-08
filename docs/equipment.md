# Armor, elytra and held items

A skin can be rendered wearing armor or an elytra and holding an item in either hand: `Options.Armor`, `Options.RightHand` and `Options.LeftHand`. All of it is drawn as the game draws it, moves with any pose or animation, and is left out of views that would not show it. `Options.Scale` resizes the figure or any of its parts.

```go
diamond1, _ := bedrockskin.DecodeImage(diamond1PNG) // textures/models/armor/diamond_1.png
diamond2, _ := bedrockskin.DecodeImage(diamond2PNG) // textures/models/armor/diamond_2.png
sword, _ := bedrockskin.DecodeImage(swordPNG)       // textures/items/diamond_sword.png

img, err := bedrockskin.Render(bedrockskin.Options{
	Texture:   skin,
	Armor:     bedrockskin.ArmorSet(diamond1, diamond2),
	RightHand: bedrockskin.Held{Item: sword},
})
```

The library ships no Minecraft textures. The caller passes them in, from a resource pack or the vanilla one.

## Armor

`Armor` has one texture per piece. An armor set has two texture layers, and the pieces split between them the way the game splits them:

| Piece | Texture | Drawn on |
| --- | --- | --- |
| `Helmet` | layer 1 (`*_1.png`) | head, plus the head's overlay box |
| `Chestplate` | layer 1 | body and both arms |
| `Leggings` | layer 2 (`*_2.png`) | body and both legs |
| `Boots` | layer 1 | both legs |
| `Elytra` | `elytra.png` | the back; see [Elytra](#elytra) |

A nil piece is not worn. `ArmorSet(layer1, layer2)` wears the four armor pieces of one material; pieces from different materials mix by setting them one by one. `ArmorSetBytes` and `ArmorBytes` are the same for `BytesOptions`, and a texture shared by several pieces is decoded once.

Armor textures are usually 64x32, but any resolution works: the model's UVs are laid out on 64x32 and scale to the texture.

### The armor model

`armor_geometry.json` holds one entry per piece. The boxes and inflates are vanilla's `geometry.humanoid.armor1` (helmet, chestplate, boots) and `geometry.humanoid.armor2` (leggings) from the game's `models/mobs.json`, written out in full rather than through the file's inheritance:

| Bone | armor1 inflate | armor2 inflate |
| --- | --- | --- |
| `head` | 1.0 | - |
| `hat` | 1.5 | - |
| `body` | 1.01 | 0.5 |
| arms | 1.0 | - |
| legs | 1.0 | 0.5 |

The body's 1.01 keeps the chestplate just outside the leggings, so the two never z-fight.

The `hat` box is vanilla's overlay for the helmet. The game's file sets 1.5 on the bone while the inherited cube carries its own 0.5, and which wins is not documented; this library uses 1.5, so anything a pack draws there shows outside the helmet. Vanilla's own armor textures leave that region empty, so they render the same either way.

The bones hang on the player model's own skeleton (`root` > `waist` > `body` > head and arms, `root` > legs), not the zombie skeleton vanilla's armor geometry inherits. A pose moves bones by name relative to their parents, so a skeleton that differs from the skin's would let a pose move the armor and the body apart: in the sneak motion, which drops the body and head separately, a helmet parented as the zombie's is would sink a unit further than the head under it.

Armor is drawn on its own model, not fitted to the skin's: a custom model with its arms moved keeps vanilla-placed armor, as it does in game. Slim skins wear the same armor as wide ones, as in game.

### Elytra

`Armor.Elytra` is vanilla's `geometry.elytra` - two 10x20x2 wings hung from the body - in `armor_geometry.json` on the same player skeleton. On top of the skin's pose it takes the elytra's resting pose, vanilla's `animation.elytra.default`: the body bone scaled by 1.067, and each wing moved 4.5 out, 4 up and 2 back, turned 15 degrees back and 13 out, and doubled in depth.

The elytra takes the chestplate's slot, as in game: with both set, only the elytra is worn.

The game also has gliding, sneaking, sleeping and swimming poses for the wings, picked by what the player is doing. An animation here does not say that, so the wings always rest as they do standing.

## Held items

`RightHand` and `LeftHand` are each a `Held`: an item sprite (`Item`), whether it is held flat (`Flat`), and an adjustment (`Adjust`). The zero value holds nothing, and each hand holds its own item. The sprite is drawn the way the game draws a flat item: a front and a back face over the whole sprite, one texel deep, with an edge strip along every side of an opaque texel that has no opaque neighbour there. So it has thickness and edges seen from the side.

- **Grip.** The sprite hangs from the model's `rightItem` or `leftItem` bone, where the game puts a held item. A model without one grips it where the standard arm's would be: one unit out, seven down and one forward of the arm's pivot. A model without the arm holds nothing; that is not an error.
- **Placement.** Item space is in blocks, the sprite's longer side one block: column `c` at `X = -c`, row `r` at `Y = height - r`, the slab running back from `Z = 0`. Fixed transforms take it into the hand's frame, the ones the game uses. Tools and weapons (`Flat` false, the default) are held upright: scaled to 0.94 of a block, 15 model units, with the blade pointing ahead of the fist and a little up and the flat of the blade facing sideways. Anything else (`Flat` true) is held flat, at 0.56 of a block. The left hand is the game's off hand, with its own offset rather than a mirror of the right. The result is scaled by 16 and its X mirrored into geometry space, so it moves with the arm as any bone under it does.
- **The arm.** Holding an item swings that arm forward, as vanilla's `animation.player.holding` does: the arm's X turn becomes `this * 0.5 - 18`, half of whatever swing a pose gives it, 18 degrees ahead. It applies to the skin, its armor and the item alike, at rest and in every animation.
- **Views.** It goes where its arm goes: shown in body and chest views and with `Parts` naming the arm, left out of head and avatar views.

A texel is opaque when its alpha is at least 128, the byte form of the shader's 0.5 alpha test. A texel below that would be discarded when drawn, so it gets no edges either.

### Adjusting an item

The game places items by kind, and not every item suits the two placements here: a trident, a shield or a pack's oddly drawn tool may sit wrong. `Held.Adjust` moves the item from the game's placement, so a caller can set it right:

```go
bedrockskin.Held{Item: trident, Adjust: bedrockskin.ItemAdjust{
	Offset:   [3]float64{0, 2, -1}, // model units
	Rotation: [3]float64{90, 0, 0}, // degrees
	Scale:    1.2,
}}
```

It works in the hand's frame, about the grip, so the item still follows the arm through every pose. `Offset` and `Rotation` mean what a bone's `Position` and `Rotation` mean: model units along the model's axes, and degrees in the geometry's convention, a positive X tipping the item's top forward. `Scale` resizes it about the grip; zero means 1. The order is scale, then rotation, then offset.

## Scale

`Options.Scale` resizes the figure or its parts. The zero value changes nothing.

- **`Model`** is the figure's size in the image: 2 draws it twice as large and crops what no longer fits, 0.5 half as large. The camera frames the model whatever its size, so this is the only way to change how big it looks; it works by dividing the camera's margin.
- **`Parts`** scales bones by name, ignoring case, each about its own pivot and carrying everything parented under it - the armor on it, an arm's held item, a hat or hair. `{"head": 1.6}` draws a big head. 0 hides a bone. It multiplies with any scale the pose already gives the bone.

A held item's own size is `ItemAdjust.Scale`.

## Order and framing

A scene draws the body, any animated persona parts, the armor (helmet, chestplate, leggings, boots, elytra), the right hand's item, the left hand's, then the cape. The camera frames everything drawn, so equipment can widen the shot a little.

Equipment never decides whether a view is empty: `ErrEmptyView` and `ErrNoMatchingParts` are about the skin alone. Without equipment, renders are exactly as they were before it existed.
