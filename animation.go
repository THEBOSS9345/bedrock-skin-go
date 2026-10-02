package bedrockskin

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"math"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/fogleman/fauxgl"
)

// Motion is one of the built-in player animations: Minecraft's own movements,
// recreated as poses over time. They move bones by their standard names
// (head, body, rightArm, leftArm, rightLeg, leftLeg), so they work on any
// model that uses them, custom ones included; whatever is parented under a
// bone - a sleeve, a hat, a backpack - moves with it. See
// docs/animation.md.
type Motion string

// The built-in motions.
const (
	MotionWalk  Motion = "walk"  // arms and legs swing, each leg opposite its arm
	MotionIdle  Motion = "idle"  // the gentle breathing sway of an idle player
	MotionWave  Motion = "wave"  // the right arm raised, waving
	MotionSneak Motion = "sneak" // leaning forward, creeping
)

// ErrUnknownMotion is returned by ParseMotion for an unrecognised name.
var ErrUnknownMotion = errors.New("bedrockskin: unknown motion")

// Motions returns every built-in motion.
func Motions() []Motion { return []Motion{MotionWalk, MotionIdle, MotionWave, MotionSneak} }

// ParseMotion returns the Motion named s.
func ParseMotion(s string) (Motion, error) {
	for _, m := range Motions() {
		if string(m) == s {
			return m, nil
		}
	}
	return "", ErrUnknownMotion
}

// BonePose moves one bone from where its geometry puts it: Rotation is added
// to the bone's own rotation (degrees, the geometry's convention), Position
// to its offset from its parent (model units), and, when Scaled, Scale
// multiplies it (about its pivot, carrying its children; 0 hides it).
type BonePose struct {
	Rotation [3]float64
	Position [3]float64
	Scale    [3]float64
	Scaled   bool
}

// Pose is a BonePose for each bone it moves, by name. Bones not in it stay at
// rest. A name matches a bone exactly, or failing that case-insensitively, as
// animation files are matched in game.
type Pose map[string]BonePose

// of returns the pose for a bone.
func (p Pose) of(bone string) BonePose {
	if bp, ok := p[bone]; ok {
		return bp
	}
	return p[strings.ToLower(bone)]
}

// Duration is how long one loop of the motion lasts, in seconds.
func (m Motion) Duration() float64 {
	switch m {
	case MotionIdle:
		return 4
	case MotionSneak:
		return 1.6
	}
	return 1
}

// Pose is the motion's pose t seconds in; it loops every Duration.
//
// Rotations follow the geometry's convention, in which a positive X turn tips
// a bone's top toward the front: a limb hanging from its shoulder or hip then
// swings backward, so swinging it forward is a negative X. Raising an arm out
// to its side is a positive Z for the right arm, negative for the left.
func (m Motion) Pose(t float64) Pose {
	phase := 2 * math.Pi * t / m.Duration()
	forward := func(deg float64) [3]float64 { return [3]float64{-deg, 0, 0} }
	switch m {
	case MotionWalk:
		// As animation.player.move.arms/legs: each arm opposite its leg, the
		// legs swinging 1.4 times as far.
		arm, leg := 40*math.Sin(phase), 56*math.Sin(phase)
		return Pose{
			"rightArm": {Rotation: forward(arm)},
			"leftArm":  {Rotation: forward(-arm)},
			"rightLeg": {Rotation: forward(-leg)},
			"leftLeg":  {Rotation: forward(leg)},
		}
	case MotionIdle:
		// Minecraft's idle bob (animation.player.bob): the arms drift out
		// from the body and back, up to 5.7 degrees.
		out := 2.865 + 2.865*math.Cos(phase)
		return Pose{
			"rightArm": {Rotation: [3]float64{0, 0, out}},
			"leftArm":  {Rotation: [3]float64{0, 0, -out}},
		}
	case MotionWave:
		return Pose{
			"rightArm": {Rotation: [3]float64{-10, 0, 150 + 20*math.Sin(phase)}},
		}
	case MotionSneak:
		// Minecraft's own sneak (animation.player.sneaking): the whole model
		// leans forward from the feet (root), set back to keep it balanced;
		// the legs turn back against the lean so they stay upright, and the
		// body and head drop. Here the legs also creep a short step.
		step := 12 * math.Sin(phase)
		return Pose{
			"root":     {Rotation: [3]float64{28, 0, 0}, Position: [3]float64{0, 1.25, 9}},
			"body":     {Position: [3]float64{0, -2, 0}},
			"head":     {Position: [3]float64{0, -1, 0}},
			"rightArm": {Rotation: [3]float64{-5.7, 0, 0}},
			"leftArm":  {Rotation: [3]float64{-5.7, 0, 0}},
			"rightLeg": {Rotation: [3]float64{-28 - step, 0.1, 0.1}},
			"leftLeg":  {Rotation: [3]float64{-28 + step, -0.1, -0.1}},
		}
	}
	return nil
}

// AnimationOptions configures RenderFrames and RenderGIF: every field of
// Options (its Pose is ignored), plus the animation and its timing.
type AnimationOptions struct {
	Options

	// Animation is what moves the model: a built-in Motion, or an Animation
	// from ParseAnimations. Required.
	Animation Animator
	// FPS is frames per second; zero means 20.
	FPS int
	// Frames is how many frames to render; zero means one loop.
	Frames int
	// Workers is how many frames are rasterized at once; zero means
	// GOMAXPROCS, one means one at a time. Frames are independent, so the
	// images are the same either way; a server already rendering on every
	// core may want 1. See docs/design-decisions.md#why-animation-frames-render-in-parallel.
	Workers int
}

// ErrNoAnimation is returned by RenderFrames and RenderGIF without an
// Animation.
var ErrNoAnimation = errors.New("bedrockskin: no animation to render")

func (o AnimationOptions) timing() (fps, frames int) {
	fps, frames = o.FPS, o.Frames
	if fps <= 0 {
		fps = 20
	}
	if frames <= 0 {
		frames = max(1, int(math.Round(o.Animation.Duration()*float64(fps))))
	}
	return fps, frames
}

// RenderFrames renders the motion frame by frame. Every frame shares one
// camera, fitted around the motion's whole sweep, so the model moves within a
// still frame rather than the frame chasing it.
func RenderFrames(opts AnimationOptions) ([]image.Image, error) {
	if opts.Animation == nil {
		return nil, ErrNoAnimation
	}
	if m, ok := opts.Animation.(Motion); ok {
		if _, err := ParseMotion(string(m)); err != nil {
			return nil, err
		}
	}
	fps, frames := opts.timing()
	scenes := make([]scene, frames)
	var sweep []*fauxgl.Triangle
	for i := range scenes {
		sc, err := opts.Options.scene(opts.Animation.Pose(float64(i) / float64(fps)))
		if err != nil {
			return nil, err
		}
		if sc.flat != nil {
			// Geometry that draws nothing has nothing to move: every frame
			// is the flat crop.
			out := make([]image.Image, frames)
			for j := range out {
				out[j] = sc.flat
			}
			return out, nil
		}
		scenes[i] = sc
		sweep = append(sweep, sc.framing()...)
	}
	first := scenes[0]
	eye, center := cameraForYawPitch(sweep, first.fov, first.margin, first.yaw, first.pitch)
	out := make([]image.Image, frames)
	workers := opts.Workers
	if workers <= 0 {
		workers = runtime.GOMAXPROCS(0)
	}
	workers = min(workers, frames)
	if workers == 1 {
		for i, sc := range scenes {
			out[i] = rasterize(sc.layers, eye, center, sc.fov, sc.size)
		}
		return out, nil
	}
	// Each frame has its own buffers and its own slot in out, so the
	// workers share nothing but the read-only scenes and textures.
	next := make(chan int, frames)
	for i := range scenes {
		next <- i
	}
	close(next)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				sc := scenes[i]
				out[i] = rasterize(sc.layers, eye, center, sc.fov, sc.size)
			}
		}()
	}
	wg.Wait()
	return out, nil
}

// RenderGIF renders the motion as a looping animated GIF. GIF holds 256
// colours a frame, so the frames share a palette of the colours they use most
// (exact for most skins, which use fewer), and transparency is on or off per
// pixel, as the renderer's alpha test already makes it.
func RenderGIF(opts AnimationOptions) ([]byte, error) {
	frames, err := RenderFrames(opts)
	if err != nil {
		return nil, err
	}
	fps, _ := opts.timing()
	pal := gifPalette(frames)
	delay := max(2, int(math.Round(100/float64(fps)))) // in hundredths of a second
	anim := &gif.GIF{LoopCount: 0}
	for _, f := range frames {
		b := f.Bounds()
		p := image.NewPaletted(b, pal)
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				c := color.NRGBAModel.Convert(f.At(x, y)).(color.NRGBA)
				if c.A < 128 {
					p.SetColorIndex(x, y, 0)
					continue
				}
				p.SetColorIndex(x, y, uint8(pal.Index(color.NRGBA{c.R, c.G, c.B, 255})))
			}
		}
		anim.Image = append(anim.Image, p)
		anim.Delay = append(anim.Delay, delay)
		anim.Disposal = append(anim.Disposal, gif.DisposalBackground)
	}
	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, anim); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// gifPalette is index 0 transparent, then up to 255 opaque colours: every
// colour the frames use when they fit, otherwise the most used, each of the
// rest drawn as its nearest.
func gifPalette(frames []image.Image) color.Palette {
	count := map[color.NRGBA]int{}
	for _, f := range frames {
		b := f.Bounds()
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				c := color.NRGBAModel.Convert(f.At(x, y)).(color.NRGBA)
				if c.A >= 128 {
					count[color.NRGBA{c.R, c.G, c.B, 255}]++
				}
			}
		}
	}
	cols := make([]color.NRGBA, 0, len(count))
	for c := range count {
		cols = append(cols, c)
	}
	sort.Slice(cols, func(i, j int) bool {
		if count[cols[i]] != count[cols[j]] {
			return count[cols[i]] > count[cols[j]]
		}
		a, b := cols[i], cols[j]
		return uint32(a.R)<<16|uint32(a.G)<<8|uint32(a.B) < uint32(b.R)<<16|uint32(b.G)<<8|uint32(b.B)
	})
	pal := color.Palette{color.NRGBA{}}
	for _, c := range cols {
		if len(pal) == 256 {
			break
		}
		pal = append(pal, c)
	}
	if len(pal) == 1 {
		pal = append(pal, color.NRGBA{A: 255})
	}
	return pal
}
