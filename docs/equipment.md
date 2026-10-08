# Armor and held items

A skin can be rendered wearing armor and holding an item: `Options.Armor` and `Options.HeldItem`. Both are drawn as the game draws them, both move with any pose or animation, and both are left out of views that would not show them.

```go
diamond1, _ := bedrockskin.DecodeImage(diamond1PNG) // textures/models/armor/diamond_1.png
diamond2, _ := bedrockskin.DecodeImage(diamond2PNG) // textures/models/armor/diamond_2.png
sword, _ := bedrockskin.DecodeImage(swordPNG)       // textures/items/diamond_sword.png

img, err := bedrockskin.Render(bedrockskin.Options{
	Texture:  skin,
	Armor:    bedrockskin.ArmorSet(diamond1, diamond2),
	HeldItem: sword,
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

A nil piece is not worn. `ArmorSet(layer1, layer2)` wears all four from one material; pieces from different materials mix by setting them one by one. `ArmorSetBytes` and `ArmorBytes` are the same for `BytesOptions`, and a texture shared by several pieces is decoded once.

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

## Held items

`HeldItem` is an item sprite, held in the right hand. It is drawn the way the game draws a flat item: every pixel that passes the alpha test becomes a cube one pixel deep, so the sprite has thickness and edges seen from the side.

- **Grip.** The sprite hangs from the model's `rightItem` bone, where the game puts a held item. A model with no `rightItem` grips it where the standard arm's would be: one unit out, seven down and one forward of the right arm's pivot. A model with no right arm holds nothing; that is not an error.
- **Size and angle.** The sprite is 10 model units across, whatever its resolution (an arm is 12). Its handle is taken to be near the bottom-left corner, where tool sprites put it, and it is tipped 20 degrees forward so a sword points ahead of the fist.
- **Views.** It goes where its arm goes: shown in body and chest views and with `Parts` naming the right arm, left out of head and avatar views.
- **Texels.** Each cube samples the middle of its own texel, so nearest-neighbour sampling never strays into a neighbour at the cube's edges.

A pixel needs alpha of at least 128 to become a cube, the byte form of the shader's 0.5 alpha test. The rest would be discarded when drawn anyway.

## Order and framing

A scene draws the body, any animated persona parts, the armor (helmet, chestplate, leggings, boots), the held item, then the cape. The camera frames everything drawn, so armor and a held item can widen the shot a little.

Equipment never decides whether a view is empty: `ErrEmptyView` and `ErrNoMatchingParts` are about the skin alone. Without equipment, renders are exactly as they were before it existed.
