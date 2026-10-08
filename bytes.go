package bedrockskin

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"

	_ "image/jpeg" // registered so DecodeImage accepts JPEG as well as PNG
)

// BytesOptions is Options with encoded byte slices in place of decoded
// images, for callers who already hold file or wire bytes and want PNG bytes
// back. It is the same render with the decode and encode steps folded in.
//
// Every field behaves exactly as its Options counterpart; see Options for
// what each one means.
type BytesOptions struct {
	// Texture is an encoded PNG or JPEG. Required.
	//
	// Bedrock sends skins as raw RGBA rather than an encoded image; use
	// TextureFromRGBA for those and the Options/Render path instead.
	Texture []byte

	// Geometry is a raw geometry.json. Nil, empty, or the literal "null" a
	// Bedrock client sends for a built-in model all fall back to
	// DefaultGeometry, so a skin's field can be passed straight through.
	Geometry []byte

	// Cape is an encoded PNG or JPEG cape texture, or nil.
	Cape []byte

	// Animated holds a persona skin's animation images, encoded; see
	// Options.Animated.
	Animated []AnimatedTextureBytes
	// Armor is Options.Armor with each piece's texture encoded.
	Armor ArmorBytes
	// RightHand and LeftHand are Options' hands with the item encoded.
	RightHand HeldBytes
	LeftHand  HeldBytes

	Identifier string
	View       View
	Angle      Angle
	Parts      []string
	Camera     *Camera
	Size       int
	// Scale and HideSkin are Options' fields. With HideSkin, Texture may be
	// empty.
	Scale    Scale
	HideSkin bool
}

// RenderBytes renders from encoded bytes and returns encoded PNG bytes.
//
//	out, err := bedrockskin.RenderBytes(bedrockskin.BytesOptions{
//		Texture: textureBytes,
//		View:    bedrockskin.ViewAvatar,
//	})
//
// It is exactly Render with decoding and PNG encoding folded in, so it
// behaves identically for geometry defaults, persona fallback and identifier
// selection.
//
// Note that this decodes whatever it is given. A service accepting untrusted
// uploads should bound image dimensions first — see
// docs/recipes.md#handling-untrusted-uploads.
func RenderBytes(opts BytesOptions) ([]byte, error) {
	o, err := opts.decode()
	if err != nil {
		return nil, err
	}
	return o.RenderPNG()
}

// AnimatedTextureBytes is AnimatedTexture with the image encoded.
type AnimatedTextureBytes struct {
	Type    AnimatedType
	Texture []byte // an encoded PNG or JPEG
}

// HeldBytes is Held with the item's sprite an encoded PNG or JPEG; nil
// holds nothing.
type HeldBytes struct {
	Item   []byte
	Flat   bool
	Adjust ItemAdjust
}

func (h HeldBytes) decode(name string) (Held, error) {
	if len(h.Item) == 0 {
		return Held{}, nil
	}
	img, err := DecodeImage(h.Item)
	if err != nil {
		return Held{}, fmt.Errorf("%s item: %w", name, err)
	}
	return Held{Item: img, Flat: h.Flat, Adjust: h.Adjust}, nil
}

// ItemBytesOptions is ItemOptions with the item's sprite an encoded PNG or
// JPEG.
type ItemBytesOptions struct {
	Item   []byte
	Angle  Angle
	Camera *Camera
	Size   int
	Adjust ItemAdjust
}

// RenderItemBytes renders an item on its own from encoded bytes and returns
// PNG bytes: RenderItem with decoding and encoding folded in.
func RenderItemBytes(opts ItemBytesOptions) ([]byte, error) {
	if len(opts.Item) == 0 {
		return nil, ErrNoTexture
	}
	item, err := DecodeImage(opts.Item)
	if err != nil {
		return nil, fmt.Errorf("item: %w", err)
	}
	img, err := RenderItem(ItemOptions{Item: item, Angle: opts.Angle, Camera: opts.Camera, Size: opts.Size, Adjust: opts.Adjust})
	if err != nil {
		return nil, err
	}
	return EncodePNG(img)
}

// ArmorBytes is Armor with each piece's texture encoded as PNG or JPEG; a
// nil piece is not worn.
type ArmorBytes struct {
	Helmet     []byte
	Chestplate []byte
	Leggings   []byte
	Boots      []byte
	Elytra     []byte
}

// ArmorSetBytes is ArmorSet for encoded textures.
func ArmorSetBytes(layer1, layer2 []byte) ArmorBytes {
	return ArmorBytes{Helmet: layer1, Chestplate: layer1, Leggings: layer2, Boots: layer1}
}

// decode decodes every worn piece. A set shares one texture between
// several pieces, so each distinct encoding is decoded once.
func (a ArmorBytes) decode() (Armor, error) {
	names := [5]string{"helmet", "chestplate", "leggings", "boots", "elytra"}
	raw := [5][]byte{a.Helmet, a.Chestplate, a.Leggings, a.Boots, a.Elytra}
	var out [5]image.Image
	for i, b := range raw {
		if len(b) == 0 {
			continue
		}
		for j := 0; j < i; j++ {
			if out[j] != nil && bytes.Equal(raw[j], b) {
				out[i] = out[j]
				break
			}
		}
		if out[i] != nil {
			continue
		}
		img, err := DecodeImage(b)
		if err != nil {
			return Armor{}, fmt.Errorf("armor %s: %w", names[i], err)
		}
		out[i] = img
	}
	return Armor{Helmet: out[0], Chestplate: out[1], Leggings: out[2], Boots: out[3], Elytra: out[4]}, nil
}

// decode turns encoded options into Options: every image decoded, the
// geometry parsed.
func (opts BytesOptions) decode() (Options, error) {
	if len(opts.Texture) == 0 && !opts.HideSkin {
		return Options{}, ErrNoTexture
	}

	var texture image.Image
	var err error
	if len(opts.Texture) > 0 {
		if texture, err = DecodeImage(opts.Texture); err != nil {
			return Options{}, fmt.Errorf("texture: %w", err)
		}
	}

	var geos []Geometry
	if !IsEmpty(opts.Geometry) {
		if geos, err = ParseGeometry(opts.Geometry); err != nil {
			return Options{}, fmt.Errorf("geometry: %w", err)
		}
	}

	var cape image.Image
	if len(opts.Cape) > 0 {
		if cape, err = DecodeImage(opts.Cape); err != nil {
			return Options{}, fmt.Errorf("cape: %w", err)
		}
	}

	var animated []AnimatedTexture
	for _, a := range opts.Animated {
		img, err := DecodeImage(a.Texture)
		if err != nil {
			return Options{}, fmt.Errorf("animation %d: %w", a.Type, err)
		}
		animated = append(animated, AnimatedTexture{Type: a.Type, Texture: img})
	}

	armor, err := opts.Armor.decode()
	if err != nil {
		return Options{}, err
	}

	right, err := opts.RightHand.decode("right hand")
	if err != nil {
		return Options{}, err
	}
	left, err := opts.LeftHand.decode("left hand")
	if err != nil {
		return Options{}, err
	}

	return Options{
		Texture:    texture,
		Geometry:   geos,
		Identifier: opts.Identifier,
		Cape:       cape,
		View:       opts.View,
		Angle:      opts.Angle,
		Parts:      opts.Parts,
		Camera:     opts.Camera,
		Size:       opts.Size,
		Animated:   animated,
		Armor:      armor,
		Scale:      opts.Scale,
		HideSkin:   opts.HideSkin,
		RightHand:  right,
		LeftHand:   left,
	}, nil
}

// AnimationBytesOptions is AnimationOptions with encoded bytes in place of
// images: BytesOptions plus the animation fields, which behave as their
// AnimationOptions counterparts.
type AnimationBytesOptions struct {
	BytesOptions

	Animation Animator // required; ErrNoAnimation without one
	FPS       int
	Frames    int
	Workers   int
}

func (opts AnimationBytesOptions) decode() (AnimationOptions, error) {
	if opts.Animation == nil {
		return AnimationOptions{}, ErrNoAnimation
	}
	o, err := opts.BytesOptions.decode()
	if err != nil {
		return AnimationOptions{}, err
	}
	return AnimationOptions{Options: o, Animation: opts.Animation, FPS: opts.FPS, Frames: opts.Frames, Workers: opts.Workers}, nil
}

// RenderGIFBytes renders an animation from encoded bytes and returns GIF
// bytes: RenderGIF with decoding folded in.
//
//	gif, err := bedrockskin.RenderGIFBytes(bedrockskin.AnimationBytesOptions{
//		BytesOptions: bedrockskin.BytesOptions{Texture: textureBytes, Size: 256},
//		Animation:    bedrockskin.MotionWalk,
//	})
func RenderGIFBytes(opts AnimationBytesOptions) ([]byte, error) {
	o, err := opts.decode()
	if err != nil {
		return nil, err
	}
	return RenderGIF(o)
}

// RenderFramesPNG renders an animation from encoded bytes and returns every
// frame as PNG bytes, in order: RenderFrames with decoding and encoding folded
// in, for callers that build their own animation format.
func RenderFramesPNG(opts AnimationBytesOptions) ([][]byte, error) {
	o, err := opts.decode()
	if err != nil {
		return nil, err
	}
	frames, err := RenderFrames(o)
	if err != nil {
		return nil, err
	}
	out := make([][]byte, len(frames))
	for i, f := range frames {
		if out[i], err = EncodePNG(f); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// RenderGIF is the method form of RenderGIFBytes.
func (o AnimationBytesOptions) RenderGIF() ([]byte, error) { return RenderGIFBytes(o) }

// RenderFramesPNG is the method form of RenderFramesPNG.
func (o AnimationBytesOptions) RenderFramesPNG() ([][]byte, error) { return RenderFramesPNG(o) }

// Render renders these options, as the package-level Render function does.
// It is the method form, for callers who prefer to build options and render
// them in one expression.
func (o Options) Render() (image.Image, error) {
	return Render(o)
}

// RenderPNG renders these options and encodes the result as PNG bytes, for
// callers holding decoded images who nonetheless want bytes back.
func (o Options) RenderPNG() ([]byte, error) {
	img, err := Render(o)
	if err != nil {
		return nil, err
	}
	return EncodePNG(img)
}

// RenderPNG renders these options and returns encoded PNG bytes. It is the
// method form of RenderBytes.
func (o BytesOptions) RenderPNG() ([]byte, error) {
	return RenderBytes(o)
}

// DecodeImage decodes PNG or JPEG bytes into an image.
//
// The result is always an *image.NRGBA: straight-alpha RGBA, whatever kind
// of PNG it was.
//
// It applies no size limit. Decoding is where a malicious image does its
// damage: a few-KB file can declare enormous dimensions and force a huge
// allocation. Callers handling untrusted input should call ImageDimensions
// first, which reads only the header.
func DecodeImage(data []byte) (image.Image, error) {
	if len(data) == 0 {
		return nil, errors.New("no image data")
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("not a valid image: %w", err)
	}
	return toNRGBA(img), nil
}

// toNRGBA returns img as straight-alpha 8-bit RGBA, the form a skin texture
// is everywhere in the library. PNGs decode to other types too - palette
// PNGs, which is how most stored skins are saved, to *image.Paletted - and
// converting each pixel's own colour keeps half-transparent pixels exact,
// where going through premultiplied RGBA() would round them.
func toNRGBA(img image.Image) image.Image {
	if n, ok := img.(*image.NRGBA); ok {
		return n
	}
	b := img.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			out.SetNRGBA(x, y, color.NRGBAModel.Convert(img.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA))
		}
	}
	return out
}

// EncodePNG encodes an image as PNG bytes.
func EncodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// WritePNG renders these options and writes the PNG to w - an HTTP response,
// a file - without holding the encoded bytes first. It writes the same bytes
// RenderPNG returns.
func (o Options) WritePNG(w io.Writer) error {
	img, err := Render(o)
	if err != nil {
		return err
	}
	return png.Encode(w, img)
}

// TextureFromRGBA wraps raw non-premultiplied RGBA pixel data as an image,
// which is the form Bedrock actually sends a skin in: SkinData decodes to
// width*height*4 bytes with no header, and the dimensions arrive separately
// in SkinImageWidth and SkinImageHeight.
//
//	raw, err := base64.StdEncoding.DecodeString(data.SkinData)
//	if err != nil {
//		return err
//	}
//	tex, err := bedrockskin.TextureFromRGBA(raw, data.SkinImageWidth, data.SkinImageHeight)
//
// The byte slice backs the image directly rather than being copied, so it
// must not be modified afterwards. A length that disagrees with the declared
// dimensions is an error rather than a silently garbled image.
//
// See docs/skin-data.md#the-texture-is-not-an-image-file.
func TextureFromRGBA(pix []byte, width, height int) (image.Image, error) {
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("invalid dimensions %dx%d", width, height)
	}
	if want := width * height * 4; len(pix) != want {
		return nil, fmt.Errorf("got %d bytes of pixel data, expected %d for %dx%d", len(pix), want, width, height)
	}
	return &image.NRGBA{
		Pix:    pix,
		Stride: width * 4,
		Rect:   image.Rect(0, 0, width, height),
	}, nil
}

// ImageDimensions reports an encoded image's pixel dimensions by reading only
// its header, without decoding the pixels.
//
// This is the check DecodeImage's documentation asks callers handling
// untrusted uploads to make first, and the reason it matters: decoding is
// where a malicious image does its damage. A few-KB PNG can declare enormous
// dimensions and force a multi-gigabyte allocation the moment it is decoded.
// Bounding the header first costs nothing.
//
//	w, h, err := bedrockskin.ImageDimensions(data)
//	if err != nil {
//		return err
//	}
//	if w > 512 || h > 512 {
//		return errors.New("skin texture too large")
//	}
//	tex, err := bedrockskin.DecodeImage(data)
//
// It is to a texture what Complexity is to geometry: the measurement, with the
// ceiling left to the caller. See docs/recipes.md#handling-untrusted-uploads.
func ImageDimensions(data []byte) (width, height int, err error) {
	if len(data) == 0 {
		return 0, 0, errors.New("no image data")
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return 0, 0, fmt.Errorf("not a valid image: %w", err)
	}
	return cfg.Width, cfg.Height, nil
}
