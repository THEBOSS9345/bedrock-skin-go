package bedrockskin

import (
	"bytes"
	"errors"
	"image"
	"image/gif"
	"math"
	"strings"
	"testing"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func evalMolang(t *testing.T, src string, queries map[string]float64) float64 {
	t.Helper()
	m, err := compileMolang(src)
	if err != nil {
		t.Fatalf("compile %q: %v", src, err)
	}
	if queries == nil {
		queries = map[string]float64{}
	}
	return m.eval(&molangEnv{queries: queries, variables: map[string]float64{}})
}

func TestMolang(t *testing.T) {
	q := map[string]float64{"anim_time": 0.25, "life_time": 2}
	for _, c := range []struct {
		src  string
		want float64
	}{
		{"1 + 2 * 3", 7},
		{"(1 + 2) * 3", 9},
		{"-2 * -3", 6},
		{"10 / 4", 2.5},
		{"1 / 0", 0},        // never NaN or infinity
		{"math.sin(90)", 1}, // degrees, as in Bedrock
		{"Math.Cos(180)", -1},
		{"math.sin(query.anim_time * 360)", 1},
		{"math.sin(q.anim_time * 360) * 30", 30},
		{"math.clamp(5, 0, 2)", 2},
		{"math.lerp(10, 20, 0.25)", 12.5},
		{"math.abs(-3) + math.pow(2, 3)", 11},
		{"math.mod(7, 3)", 1},
		{"math.pi", math.Pi},
		{"query.life_time > 1 ? 5 : 6", 5},
		{"query.life_time > 9 ? 5 : 6", 6},
		{"query.life_time > 9 ? 5", 0},
		{"1 == 1 && 2 != 3", 1},
		{"!(1 < 2) || 0", 0},
		{"query.not_modelled", 0},
		{"variable.x = 4; variable.y = variable.x * 2; return variable.y + 1;", 9},
		{"v.a = 3; return v.a;", 3},
		{"1.5f", 1.5},
		{"", 0},
	} {
		if got := evalMolang(t, c.src, q); !near(got, c.want) {
			t.Errorf("%q = %v, want %v", c.src, got, c.want)
		}
	}
	for _, bad := range []string{"1 +", "math.sin(", "math.nope(1)", "(1", "1 $ 2"} {
		if _, err := compileMolang(bad); err == nil {
			t.Errorf("%q compiled, want a syntax error", bad)
		}
	}
}

const animFile = `{
  "format_version": "1.8.0",
  "animations": {
    "animation.test.keys": {
      "loop": true,
      "animation_length": 2.0,
      "bones": {
        "RightArm": {
          "rotation": { "0.0": [0, 0, 0], "1.0": [-90, 0, 0], "2.0": [0, 0, 0] }
        },
        "head": {
          "rotation": ["math.sin(query.anim_time * 180) * 30", 0, 0],
          "position": [0, 1, 0],
          "scale": 1.5
        },
        "body": {
          "position": {
            "0.0": [0, 0, 0],
            "0.5": { "pre": [0, 0, 0], "post": [0, 4, 0] },
            "1.5": [0, 4, 0]
          }
        },
        "leftLeg": {
          "rotation": {
            "0.0": { "post": [0, 0, 0], "lerp_mode": "catmullrom" },
            "1.0": { "post": [40, 0, 0], "lerp_mode": "catmullrom" },
            "2.0": { "post": [0, 0, 0], "lerp_mode": "catmullrom" }
          }
        },
        "rightLeg": {
          "rotation": { "0.0": { "post": [10, 0, 0], "lerp_mode": "step" }, "1.0": [50, 0, 0] }
        }
      }
    },
    "animation.test.once": {
      "bones": { "head": { "rotation": { "0.0": [0, 0, 0], "1.0": [0, 90, 0] } } }
    },
    "animation.test.distance": {
      "loop": true,
      "anim_time_update": "query.modified_distance_moved",
      "bones": { "rightArm": { "rotation": ["query.anim_time", 0, 0] } }
    }
  }
}`

func TestParseAnimations(t *testing.T) {
	anims, err := ParseAnimations([]byte(animFile))
	if err != nil {
		t.Fatal(err)
	}
	keys := anims["animation.test.keys"]
	if keys == nil || !keys.Loop || keys.Length != 2 || keys.Duration() != 2 {
		t.Fatalf("keys = %+v", keys)
	}

	p := keys.Pose(0.5)
	// Linear keyframes, matched to bones case-insensitively ("RightArm").
	if got := p.of("rightArm").Rotation[0]; !near(got, -45) {
		t.Errorf("linear half way: %v, want -45", got)
	}
	// A Molang channel reads the animation's time, in degrees.
	if got := p.of("head").Rotation[0]; !near(got, 30) {
		t.Errorf("molang at 0.5s: %v, want 30", got)
	}
	if hp := p.of("head"); hp.Position[1] != 1 || !hp.Scaled || hp.Scale != [3]float64{1.5, 1.5, 1.5} {
		t.Errorf("head position/scale = %+v", hp)
	}
	// pre/post: the value jumps at 0.5s.
	if a, b := keys.Pose(0.49).of("body").Position[1], keys.Pose(0.51).of("body").Position[1]; a != 0 || b != 4 {
		t.Errorf("pre/post jump: %v then %v, want 0 then 4", a, b)
	}
	// Catmull-Rom passes through its keyframes and curves between them. With
	// 0, 40, 0 and the first keyframe standing in for the one before it, the
	// uniform spline at a quarter of the way is 9.0625, where a straight line
	// would be 10.
	cr := keys.Pose(0.25).of("leftLeg").Rotation[0]
	if !near(keys.Pose(1).of("leftLeg").Rotation[0], 40) || !near(cr, 9.0625) {
		t.Errorf("catmull-rom: %v at 0.25s (want 9.0625), %v at 1s (want 40)", cr, keys.Pose(1).of("leftLeg").Rotation[0])
	}
	// Step holds its value until the next keyframe.
	if got := keys.Pose(0.9).of("rightLeg").Rotation[0]; got != 10 {
		t.Errorf("step: %v, want 10", got)
	}
	// Looping: 2.5s is 0.5s again.
	if got := keys.Pose(2.5).of("rightArm").Rotation[0]; !near(got, -45) {
		t.Errorf("loop: %v at 2.5s, want -45", got)
	}

	// Not looping: the last pose holds, and the length is the last keyframe.
	once := anims["animation.test.once"]
	if once.Loop || once.Length != 1 || once.Pose(5).of("head").Rotation[1] != 90 {
		t.Errorf("once: loop %v, length %v, after the end %v", once.Loop, once.Length, once.Pose(5).of("head").Rotation[1])
	}

	// anim_time_update drives the clock by distance walked.
	dist := anims["animation.test.distance"]
	if got := dist.Pose(1).of("rightArm").Rotation[0]; !near(got, walkSpeed) {
		t.Errorf("distance-driven time after 1s: %v, want %v", got, walkSpeed)
	}
}

func TestParseAnimationsErrors(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"empty", `{"animations":{}}`, "no animations"},
		{"not json", `nope`, "animation file"},
		{"bad molang", `{"animations":{"animation.a":{"bones":{"head":{"rotation":["math.sin(",0,0]}}}}}`, "animation.a, bone head, rotation"},
		{"quaternion", `{"animations":{"animation.a":{"bones":{"head":{"rotation":[0,0,0,1]}}}}}`, "quaternion"},
		{"bad time", `{"animations":{"animation.a":{"bones":{"head":{"rotation":{"soon":[0,0,0]}}}}}}`, "keyframe time"},
	} {
		_, err := ParseAnimations([]byte(c.src))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v, want an error mentioning %q", c.name, err, c.want)
		}
	}
	if _, err := ParseAnimations([]byte(`{"animations":{}}`)); !errors.Is(err, ErrNoAnimations) {
		t.Errorf("empty file: %v, want ErrNoAnimations", err)
	}
}

// The built-in motions use Minecraft's conventions: a negative X swings a
// hanging limb forward, a positive Z takes the right arm out.
func TestMotionPoses(t *testing.T) {
	walk := MotionWalk.Pose(0.25 * MotionWalk.Duration())
	if r, l := walk.of("rightArm").Rotation[0], walk.of("rightLeg").Rotation[0]; r >= 0 || l <= 0 {
		t.Errorf("walk at a quarter: right arm X %v (want forward, negative), right leg X %v (want back, positive)", r, l)
	}
	if got := MotionWave.Pose(0).of("rightArm").Rotation[2]; got < 90 {
		t.Errorf("wave: right arm Z %v, want raised past 90", got)
	}
	sneak := MotionSneak.Pose(0)
	if sneak.of("root").Rotation[0] != 28 || sneak.of("rightLeg").Rotation[0] != -28 {
		t.Errorf("sneak: root %v, right leg %v; want Minecraft's 28 and -28", sneak.of("root").Rotation[0], sneak.of("rightLeg").Rotation[0])
	}
	for _, m := range Motions() {
		if got, err := ParseMotion(string(m)); err != nil || got != m {
			t.Errorf("ParseMotion(%q) = %v, %v", m, got, err)
		}
	}
	if _, err := ParseMotion("moonwalk"); !errors.Is(err, ErrUnknownMotion) {
		t.Errorf("ParseMotion(moonwalk): %v, want ErrUnknownMotion", err)
	}
}

func TestRenderAnimation(t *testing.T) {
	opts := AnimationOptions{Options: Options{Texture: testTexture(), Size: 64}, Animation: MotionWalk, FPS: 8}
	frames, err := RenderFrames(opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 8 {
		t.Fatalf("one loop at 8 fps: %d frames, want 8", len(frames))
	}
	// The pose changes from frame to frame, the camera does not: the head
	// (which walking does not move) stays put.
	a, b := encodePNG(t, frames[0]), encodePNG(t, frames[2])
	if bytes.Equal(a, b) {
		t.Error("walking frames 0 and 2 are identical")
	}
	if headTop(frames[0]) != headTop(frames[2]) {
		t.Errorf("the head moved between frames (%d, %d): the camera is not fixed", headTop(frames[0]), headTop(frames[2]))
	}

	out, err := RenderGIF(opts)
	if err != nil {
		t.Fatal(err)
	}
	g, err := gif.DecodeAll(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("not a GIF: %v", err)
	}
	if len(g.Image) != 8 || g.LoopCount != 0 || g.Delay[0] != 13 {
		t.Errorf("gif: %d frames, loop %d, delay %d; want 8 frames looping at 1/8 s", len(g.Image), g.LoopCount, g.Delay[0])
	}

	// A file animation renders too, moving bones it names in any case.
	anims, err := ParseAnimations([]byte(animFile))
	if err != nil {
		t.Fatal(err)
	}
	if fr, err := RenderFrames(AnimationOptions{Options: Options{Texture: testTexture(), Size: 64}, Animation: anims["animation.test.keys"], FPS: 4}); err != nil || len(fr) != 8 {
		t.Errorf("file animation: %d frames, %v; want 8", len(fr), err)
	}
	if _, err := RenderFrames(AnimationOptions{Options: Options{Texture: testTexture()}}); !errors.Is(err, ErrNoAnimation) {
		t.Errorf("no animation: %v, want ErrNoAnimation", err)
	}
}

// headTop is the first row with an opaque pixel.
func headTop(img image.Image) int {
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a >= 0x8000 {
				return y
			}
		}
	}
	return -1
}

// Minecraft's own sneak, verbatim from Mojang's sample resource pack
// (resource_pack/animations/player.animation.json), renders exactly as the
// built-in MotionSneak does at the start of its step: the built-in motion is
// that animation, and a real file - "this" and all - parses and plays.
func TestVanillaSneakMatchesMotion(t *testing.T) {
	const vanilla = `{"format_version":"1.8.0","animations":{
	"animation.player.sneaking": {
		"loop": true,
		"bones": {
			"body": { "position": [ 0.0, -2.0, 0.0 ] },
			"head": { "position": [ 0.0, -1.0, 0.0 ] },
			"leftarm": { "rotation": [ -5.7, 0.0, 0.0 ] },
			"leftleg": { "rotation": [ -28.0, -0.1, -0.1 ] },
			"rightarm": { "rotation": [ -5.7, 0.0, 0.0 ] },
			"rightleg": { "rotation": [ -28.0, 0.1, 0.1 ] },
			"root": { "position": [ 0.0, 1.25, 9.0 ], "rotation": [ "28.0 - this", 0.0, 0.0 ] }
		}
	}}}`
	anims, err := ParseAnimations([]byte(vanilla))
	if err != nil {
		t.Fatal(err)
	}
	opts := Options{Texture: testTexture(), Size: 96, Camera: &Camera{Yaw: 90}}
	opts.Pose = anims["animation.player.sneaking"].Pose(0)
	fromFile, err := opts.RenderPNG()
	if err != nil {
		t.Fatal(err)
	}
	opts.Pose = MotionSneak.Pose(0)
	builtIn, err := opts.RenderPNG()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(fromFile, builtIn) {
		t.Error("Minecraft's sneak animation renders differently from MotionSneak")
	}
}

func TestExampleAnimations(t *testing.T) {
	anims := ExampleAnimations()
	if len(anims) < 30 {
		t.Fatalf("got %d example animations, want 30 or more", len(anims))
	}
	geos, err := ParseGeometry(defaultGeometryJSON)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := SelectGeometry(geos, "geometry.humanoid.custom")
	tex := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for name, a := range anims {
		// Made for the player: every bone they move is on the player model.
		if missing := a.MissingBones(body); len(missing) > 0 {
			t.Errorf("%s moves bones the player model lacks: %v", name, missing)
		}
		if _, err := Render(Options{Texture: tex, Size: 32, Pose: a.Pose(a.Duration() / 3)}); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	// The map is the caller's to change.
	delete(anims, "animation.player.dance")
	if _, ok := ExampleAnimations()["animation.player.dance"]; !ok {
		t.Error("deleting from one result changed the next")
	}
}

func TestMissingBones(t *testing.T) {
	anims, err := ParseAnimations([]byte(`{"animations":{"a":{"bones":{"RightArm":{"rotation":[0,0,10]},"wing_left":{"rotation":[0,0,10]}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	a := anims["a"]
	if got := a.Bones(); len(got) != 2 || got[0] != "RightArm" || got[1] != "wing_left" {
		t.Errorf("Bones = %v", got)
	}
	g := Geometry{Bones: []Bone{{Name: "rightArm"}}}
	if got := a.MissingBones(g); len(got) != 1 || got[0] != "wing_left" {
		t.Errorf("MissingBones = %v", got)
	}
}
