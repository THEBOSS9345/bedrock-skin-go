# Changelog

Versions match bedrock-skin-rs: the same version renders the same images
with the same features in both.

## v0.3.0

- Armor: `Options.Armor` wears a helmet, chestplate, leggings and boots, one
  texture per piece as a resource pack lays them out, on vanilla's armor
  model. `ArmorSet(layer1, layer2)` wears a full set; `BytesOptions.Armor`
  takes them encoded (`ArmorBytes`, `ArmorSetBytes`).
- Held items: `Options.HeldItem` puts an item sprite in the right hand,
  extruded one pixel deep as the game draws it, gripped at the model's
  `rightItem`. `BytesOptions.HeldItem` takes it encoded.
- Both move with every pose and animation, and head and avatar views show
  only the helmet. Renders without them are unchanged.

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
