package bedrockskin

import "fmt"

// WireSkin is a skin as a Bedrock client sends it - in the login packet, a
// PlayerList entry or a PlayerSkin packet: images as raw RGBA with their
// sizes alongside, and the model as JSON. The field names follow the
// protocol's, so a proxy or bot can fill one straight from the packet.
//
//	opts, err := bedrockskin.WireSkin{
//		SkinData: s.SkinData, SkinWidth: int(s.SkinImageWidth), SkinHeight: int(s.SkinImageHeight),
//		Geometry: s.SkinGeometry, ResourcePatch: s.SkinResourcePatch,
//	}.Options()
//
// See docs/skin-data.md for what each field holds.
type WireSkin struct {
	// SkinData is the skin's pixels, raw non-premultiplied RGBA,
	// SkinWidth*SkinHeight*4 bytes. Required.
	SkinData              []byte
	SkinWidth, SkinHeight int

	// CapeData is the cape's pixels in the same form; empty for no cape.
	CapeData              []byte
	CapeWidth, CapeHeight int

	// Geometry is SkinGeometryData. Empty or the literal "null" - what a
	// client sends for a built-in model - draws the default model.
	Geometry []byte

	// ResourcePatch is SkinResourcePatch: it names which entry of Geometry
	// the skin uses, wide or slim among them.
	ResourcePatch []byte

	// Animations is the skin's animation list. A persona skin's face, and
	// some of its body, are textured by these.
	Animations []WireAnimation
}

// WireAnimation is one entry of a skin's animation list: its image as raw
// RGBA, and its type as the protocol numbers it.
type WireAnimation struct {
	Type          AnimatedType
	Data          []byte
	Width, Height int
}

// Options turns the wire fields into render Options: the images wrapped
// without copying, the geometry parsed, the entry the resource patch names
// selected, and the animation images attached so persona heads draw. Set
// View, Size and the rest on the result.
//
// A resource patch that does not parse is not an error - the patch only
// picks an entry, and without it the entry with the most cubes is used, as
// for an empty patch. Malformed geometry and images whose data does not match
// their size are errors. An animation of a type the renderer does not draw
// is kept and ignored.
func (w WireSkin) Options() (Options, error) {
	tex, err := TextureFromRGBA(w.SkinData, w.SkinWidth, w.SkinHeight)
	if err != nil {
		return Options{}, fmt.Errorf("skin: %w", err)
	}
	o := Options{Texture: tex}
	if len(w.CapeData) > 0 {
		if o.Cape, err = TextureFromRGBA(w.CapeData, w.CapeWidth, w.CapeHeight); err != nil {
			return Options{}, fmt.Errorf("cape: %w", err)
		}
	}
	if !IsEmpty(w.Geometry) {
		if o.Geometry, err = ParseGeometry(w.Geometry); err != nil {
			return Options{}, fmt.Errorf("geometry: %w", err)
		}
	}
	if len(w.ResourcePatch) > 0 {
		if p, err := ParseResourcePatch(w.ResourcePatch); err == nil {
			o.Identifier = p.Default
		}
	}
	for _, a := range w.Animations {
		img, err := TextureFromRGBA(a.Data, a.Width, a.Height)
		if err != nil {
			return Options{}, fmt.Errorf("animation %d: %w", a.Type, err)
		}
		o.Animated = append(o.Animated, AnimatedTexture{Type: a.Type, Texture: img})
	}
	return o, nil
}

// Skin is the invisibility detector's view of the same wire fields: the skin
// texture and its geometry. It does not need the cape or the animations.
func (w WireSkin) Skin() (*Skin, error) {
	tex, err := TextureFromRGBA(w.SkinData, w.SkinWidth, w.SkinHeight)
	if err != nil {
		return nil, fmt.Errorf("skin: %w", err)
	}
	var geo []byte
	if !IsEmpty(w.Geometry) {
		geo = w.Geometry
	}
	return NewSkin(tex, geo), nil
}
