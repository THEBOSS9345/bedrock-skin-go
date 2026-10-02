package bedrockskin

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"testing"
)

var sprintf = fmt.Sprintf

func encodePNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// armsTexture is a procedural skin with the right arm's texture (40,16) red,
// the left arm's (32,48) blue, the rest gray, and the outer layer clear so it
// covers nothing.
func armsTexture() *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	fill := func(x, y, w, h int, c color.NRGBA) {
		draw.Draw(img, image.Rect(x, y, x+w, y+h), &image.Uniform{c}, image.Point{}, draw.Src)
	}
	fill(0, 0, 64, 64, color.NRGBA{128, 128, 128, 255})
	fill(40, 16, 16, 16, color.NRGBA{230, 30, 30, 255})
	fill(32, 48, 16, 16, color.NRGBA{30, 60, 230, 255})
	for _, r := range [][4]int{{32, 0, 32, 16}, {16, 32, 24, 16}, {40, 32, 16, 16}, {0, 32, 16, 16}, {48, 48, 16, 16}, {0, 48, 16, 16}} {
		fill(r[0], r[1], r[2], r[3], color.NRGBA{})
	}
	return img
}

// centroidX is the mean x of the pixels matching want, or -1 if none do.
func centroidX(img image.Image, want func(r, g, b uint32) bool) int {
	sum, n := 0, 0
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, a := img.At(x, y).RGBA()
			if a >= 0x8000 && want(r, g, bl) {
				sum += x
				n++
			}
		}
	}
	if n == 0 {
		return -1
	}
	return sum / n
}

var (
	isRed  = func(r, g, b uint32) bool { return r > 2*g && r > 2*b }
	isBlue = func(r, g, b uint32) bool { return b > 2*r && b > g }
)

// Facing a player, their right arm is on the viewer's left - the convention
// every Minecraft renderer and the game itself follow. Bedrock model space is
// X-mirrored against the world, so a renderer that draws it as written puts
// the arms the wrong way round. See docs/rendering-pipeline.md#model-space-is-x-mirrored.
func TestRightArmOnViewersLeft(t *testing.T) {
	tex := armsTexture()
	for _, c := range []struct {
		name      string
		opts      Options
		redOnLeft bool
	}{
		{"front", Options{Texture: tex, Size: 128}, true},
		{"back", Options{Texture: tex, Size: 128, Camera: &Camera{Yaw: 180}}, false},
	} {
		img, err := Render(c.opts)
		if err != nil {
			t.Fatal(err)
		}
		red, blue := centroidX(img, isRed), centroidX(img, isBlue)
		if red < 0 || blue < 0 {
			t.Fatalf("%s: an arm is missing (red at %d, blue at %d)", c.name, red, blue)
		}
		if (red < blue) != c.redOnLeft {
			t.Errorf("%s: right arm (red) at x=%d, left arm (blue) at x=%d; want the right arm on the viewer's %s",
				c.name, red, blue, map[bool]string{true: "left", false: "right"}[c.redOnLeft])
		}
	}

	flat := Render2D(tex, ViewBody, 128)
	if red, blue := centroidX(flat, isRed), centroidX(flat, isBlue); red < 0 || blue < 0 || red > blue {
		t.Errorf("Render2D: right arm (red) at x=%d, left arm (blue) at x=%d; want it on the viewer's left", red, blue)
	}
}

// Real captured geometry sometimes leaves texture_width/height out. Minecraft
// reads that as 64x64; dividing by the missing zero drew nothing at all.
func TestMissingTextureSizeIs64(t *testing.T) {
	modern := `{"format_version":"1.12.0","minecraft:geometry":[{"description":{"identifier":"geometry.t"%s},
		"bones":[{"name":"body","pivot":[0,0,0],"cubes":[{"origin":[-4,0,-2],"size":[8,12,4],"uv":[16,16]}]}]}]}`
	legacy := `{"format_version":"1.8.0","geometry.t":{%s"bones":[{"name":"body","pivot":[0,0,0],"cubes":[{"origin":[-4,0,-2],"size":[8,12,4],"uv":[16,16]}]}]}}`
	for _, c := range []struct{ name, without, with string }{
		{"modern", sprintf(modern, ""), sprintf(modern, `,"texture_width":64,"texture_height":64`)},
		{"legacy", sprintf(legacy, ""), sprintf(legacy, `"texturewidth":64,"textureheight":64,`)},
	} {
		got, err := RenderBytes(BytesOptions{Texture: encodePNG(t, testTexture()), Geometry: []byte(c.without), Size: 64})
		if err != nil {
			t.Fatalf("%s without a size: %v", c.name, err)
		}
		want, err := RenderBytes(BytesOptions{Texture: encodePNG(t, testTexture()), Geometry: []byte(c.with), Size: 64})
		if err != nil {
			t.Fatalf("%s with a size: %v", c.name, err)
		}
		if string(got) != string(want) {
			t.Errorf("%s: geometry without a texture size renders differently from one declaring 64x64", c.name)
		}
	}
}

// A cube's rotation turns it about its pivot: a flat square turned 45 degrees
// about the view axis is a diamond, with its bounding box's corners empty.
func TestCubeRotation(t *testing.T) {
	geo := func(rotation string) []Geometry {
		geos, err := ParseGeometry([]byte(`{"format_version":"1.12.0","minecraft:geometry":[{"description":{"identifier":"geometry.t","texture_width":64,"texture_height":64},
			"bones":[{"name":"body","pivot":[0,0,0],"cubes":[{"origin":[-4,0,-0.5],"size":[8,8,1],"uv":[0,0]` + rotation + `}]}]}]}`))
		if err != nil {
			t.Fatal(err)
		}
		return geos
	}
	corner := func(geos []Geometry) (cornerOpaque, centreOpaque bool) {
		img, err := Render(Options{Texture: testTexture(), Geometry: geos, Size: 128})
		if err != nil {
			t.Fatal(err)
		}
		// The model's on-screen bounding box.
		b := img.Bounds()
		minX, minY, maxX, maxY := b.Max.X, b.Max.Y, b.Min.X, b.Min.Y
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				if _, _, _, a := img.At(x, y).RGBA(); a >= 0x8000 {
					minX, minY, maxX, maxY = min(minX, x), min(minY, y), max(maxX, x), max(maxY, y)
				}
			}
		}
		inset := (maxX - minX) / 10
		_, _, _, ca := img.At(minX+inset, minY+inset).RGBA()
		_, _, _, ma := img.At((minX+maxX)/2, (minY+maxY)/2).RGBA()
		return ca >= 0x8000, ma >= 0x8000
	}
	if c, m := corner(geo(``)); !c || !m {
		t.Fatalf("unrotated square: corner opaque %v, centre opaque %v; want both", c, m)
	}
	if c, m := corner(geo(`,"rotation":[0,0,45],"pivot":[0,4,0]`)); c || !m {
		t.Errorf("square turned 45 degrees: corner opaque %v, centre opaque %v; want an empty corner, a filled centre", c, m)
	}
}

// A bone's mirror flag applies to its cubes, as Bedrock's own models use it.
func TestBoneMirrorAppliesToCubes(t *testing.T) {
	render := func(boneMirror, cubeMirror string) string {
		geos, err := ParseGeometry([]byte(`{"format_version":"1.12.0","minecraft:geometry":[{"description":{"identifier":"geometry.t","texture_width":64,"texture_height":64},
			"bones":[{"name":"body","pivot":[0,0,0]` + boneMirror + `,"cubes":[{"origin":[-4,0,-2],"size":[8,12,4],"uv":[16,16]` + cubeMirror + `}]}]}]}`))
		if err != nil {
			t.Fatal(err)
		}
		out, err := Options{Texture: testTexture(), Geometry: geos, Size: 64}.RenderPNG()
		if err != nil {
			t.Fatal(err)
		}
		return string(out)
	}
	plain, onBone, onCube := render(``, ``), render(`,"mirror":true`, ``), render(``, `,"mirror":true`)
	if onBone == plain {
		t.Error("a bone's mirror flag changed nothing")
	}
	if onBone != onCube {
		t.Error("mirror on the bone renders differently from mirror on its cube")
	}
}

// Positive yaw swings the camera toward the model's left (see
// views-and-cameras.md), so it brings the head's left side (texture 16,8)
// into view, not its right (0,8).
func TestPositiveYawShowsModelsLeft(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	fill := func(x, y, w, h int, c color.NRGBA) {
		draw.Draw(img, image.Rect(x, y, x+w, y+h), &image.Uniform{c}, image.Point{}, draw.Src)
	}
	fill(0, 0, 64, 64, color.NRGBA{128, 128, 128, 255})
	fill(32, 0, 32, 16, color.NRGBA{})               // no hat
	fill(0, 8, 8, 8, color.NRGBA{230, 30, 30, 255})  // the head's right side
	fill(16, 8, 8, 8, color.NRGBA{30, 200, 30, 255}) // the head's left side
	isGreen := func(r, g, b uint32) bool { return g > 2*r && g > 2*b }
	for _, c := range []struct {
		yaw         float64
		left, right bool
	}{{60, true, false}, {-60, false, true}} {
		out, err := Render(Options{Texture: img, View: ViewHead, Camera: &Camera{Yaw: c.yaw}, Size: 96})
		if err != nil {
			t.Fatal(err)
		}
		left, right := centroidX(out, isGreen) >= 0, centroidX(out, isRed) >= 0
		if left != c.left || right != c.right {
			t.Errorf("yaw %v: left side visible %v, right side visible %v; want %v, %v", c.yaw, left, right, c.left, c.right)
		}
	}
}
