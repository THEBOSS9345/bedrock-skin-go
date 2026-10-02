# Changelog

Versions match bedrock-skin-rs: the same version renders the same images
with the same features in both.

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
