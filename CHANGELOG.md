# Changelog

Versions match bedrock-skin-rs: the same version renders the same images
with the same features in both.

## v0.3.0

- Armor: `Options.Armor` wears a helmet, chestplate, leggings and boots, one
  texture per piece as a resource pack lays them out, on vanilla's armor
  model. `ArmorSet(layer1, layer2)` wears a full set; `BytesOptions.Armor`
  takes them encoded (`ArmorBytes`, `ArmorSetBytes`).
- Elytra: `Armor.Elytra`, on vanilla's elytra model in its resting pose,
  in the chestplate's slot.
- Held items: `Options.RightHand` and `LeftHand` each hold an item sprite
  (`Held`), extruded one texel deep and placed where the game places it,
  from the model's `rightItem` or `leftItem`, with the arm held forward as
  vanilla's holding animation holds it. Tools and weapons are held upright;
  `Held.Flat` holds anything else flat. `Held.Adjust` (`ItemAdjust`) moves,
  turns or resizes an item the game's placement does not suit.
  `BytesOptions` takes them encoded (`HeldBytes`).
- `Options.Scale`: the figure's size in the image, and per-bone scales that
  carry armor and items with them.
- `Options.HideSkin` draws the equipment alone, so a helmet, the elytra or a
  held item renders by itself; `Texture` is then optional. `RenderItem` and
  `RenderItemBytes` render an item sprite on its own, and `RenderItemGIF`,
  `RenderItemFrames` and `RenderItemGIFBytes` spin it.
- Equipment moves with every pose and animation, and head and avatar views
  show only the helmet.
- Fixed: one-pixel slivers along the edges of thin, edge-on faces, and a
  gap down the diagonal of a face larger than the image, both inherited
  from fauxgl's rasterizer; pixels off the image's side wrapping into the
  next row; and every cube's bottom face mapped the wrong way round. Every
  render can differ by a few edge pixels from v0.2.2.

## v0.2.2

- Fixed: poses now find bones case-insensitively both ways. The built-in
  motions and most example animations say `leftArm`, persona models name the
  bone `leftarm`, and persona skins stood still in walk, idle, wave, sneak and
  11 animations in all.

## v0.2.1

- `WireSkin`: a skin as a Bedrock packet carries it - raw RGBA, geometry,
  resource patch, animation list - to ready Options (`Options()`) or a
  detector `Skin` (`Skin()`), the patch's model picked and persona faces
  attached.
- `Options.WritePNG` and `WriteGIF`: render straight into an `io.Writer`.
- `cmd/bedrock-skin`: try the library from the command line.
- `Example` functions for pkg.go.dev.
- `PolyMesh.Polygons()` and `PolyVertex`: a poly mesh's polygons with each
  corner's position, normal and UV looked up.

## v0.2.0

- Animations from bytes: `RenderGIFBytes` and `RenderFramesPNG`, with
  `AnimationBytesOptions` - encoded images in, GIF bytes or one PNG per frame
  out, the same as decoding first and rendering.
- `BytesOptions.Animated` takes a persona skin's encoded animation images.

## v0.1.0

The first tagged release, as module github.com/THEBOSS9345/bedrock-skin-go
(formerly mcpe-skinapi): persona skins drawn from their poly meshes, our own
rasterizer (4.7x faster than drawing through fauxgl's general one), animation
frames rasterized in parallel (`AnimationOptions.Workers`), and the
invisible-skin detector.
