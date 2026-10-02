package bedrockskin_test

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"os"

	bedrockskin "github.com/THEBOSS9345/bedrock-skin-go"
)

// skinTexture stands in for a decoded skin file: 64x64, every pixel opaque.
func skinTexture() *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			img.Set(x, y, color.NRGBA{R: uint8(x * 4), G: uint8(y * 4), B: 128, A: 255})
		}
	}
	return img
}

// A skin file in, a 3D head icon out. With no geometry the default model is
// drawn, which is right for most skins.
func Example() {
	raw, err := os.ReadFile("skin.png")
	if err != nil {
		return // a skin file of your own
	}
	tex, err := bedrockskin.DecodeImage(raw)
	if err != nil {
		panic(err)
	}
	f, err := os.Create("avatar.png")
	if err != nil {
		panic(err)
	}
	defer f.Close()
	opts := bedrockskin.Options{Texture: tex, View: bedrockskin.ViewAvatar, Angle: bedrockskin.AngleIso, Size: 256}
	if err := opts.WritePNG(f); err != nil {
		panic(err)
	}
}

func ExampleRender() {
	img, err := bedrockskin.Render(bedrockskin.Options{
		Texture: skinTexture(),
		View:    bedrockskin.ViewHead,
		Size:    128,
	})
	if err != nil {
		panic(err)
	}
	fmt.Println(img.Bounds().Dx(), "x", img.Bounds().Dy())
	// Output: 128 x 128
}

// A skin straight from a Bedrock packet: raw RGBA, "null" geometry for a
// built-in model, and the resource patch naming the slim arms.
func ExampleWireSkin_Options() {
	pix := skinTexture().Pix
	opts, err := bedrockskin.WireSkin{
		SkinData:      pix,
		SkinWidth:     64,
		SkinHeight:    64,
		Geometry:      []byte("null"),
		ResourcePatch: []byte(`{"geometry":{"default":"geometry.humanoid.customSlim"}}`),
	}.Options()
	if err != nil {
		panic(err)
	}
	opts.View = bedrockskin.ViewChest
	opts.Size = 96
	img, err := opts.Render()
	if err != nil {
		panic(err)
	}
	fmt.Println(opts.Identifier, img.Bounds().Dx())
	// Output: geometry.humanoid.customSlim 96
}

// A looping GIF, written as it is encoded - to a file here, to an HTTP
// response just the same.
func ExampleWriteGIF() {
	var out bytes.Buffer
	err := bedrockskin.WriteGIF(&out, bedrockskin.AnimationOptions{
		Options:   bedrockskin.Options{Texture: skinTexture(), Size: 64},
		Animation: bedrockskin.MotionWalk,
		FPS:       10,
	})
	if err != nil {
		panic(err)
	}
	g, err := gif.DecodeAll(&out)
	if err != nil {
		panic(err)
	}
	fmt.Println(len(g.Image), "frames")
	// Output: 10 frames
}

// One of the 33 bundled animations.
func ExampleExampleAnimations() {
	dance := bedrockskin.ExampleAnimations()["animation.player.dance"]
	frames, err := bedrockskin.RenderFrames(bedrockskin.AnimationOptions{
		Options:   bedrockskin.Options{Texture: skinTexture(), Size: 48},
		Animation: dance,
		FPS:       5,
	})
	if err != nil {
		panic(err)
	}
	fmt.Println(len(frames) > 1)
	// Output: true
}

// Blocking the invisible-skin trick: a fully transparent skin is caught.
func ExampleNewSkin() {
	clear := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	fmt.Println(bedrockskin.NewSkin(clear, nil).IsInvisible())
	fmt.Println(bedrockskin.NewSkin(skinTexture(), nil).IsInvisible())
	// Output:
	// true
	// false
}

// Reading a persona skin's poly mesh: every polygon, each corner looked up.
func ExamplePolyMesh_Polygons() {
	geos, err := bedrockskin.ParseGeometry([]byte(`{"minecraft:geometry":[{"description":{"identifier":"geometry.persona_x"},
		"bones":[{"name":"body","poly_mesh":{"normalized_uvs":true,
			"positions":[[-4,12,-2],[4,12,-2],[4,24,-2],[-4,24,-2]],
			"normals":[[0,0,-1]],
			"uvs":[[0.25,0.5],[0.375,0.5],[0.375,0.6875],[0.25,0.6875]],
			"polys":[[[0,0,0],[1,0,1],[2,0,2],[3,0,3]]]}}]}]}`))
	if err != nil {
		panic(err)
	}
	mesh, _ := geos[0].Bones[0].Mesh()
	for _, poly := range mesh.Polygons() {
		fmt.Println(len(poly), "corners, first at", poly[0].Position, "facing", poly[0].Normal)
	}
	// Output: 4 corners, first at [-4 12 -2] facing [0 0 -1]
}
