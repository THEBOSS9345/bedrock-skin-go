package bedrockskin

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"io"
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

// of returns the pose for a bone: the exact name, else the bone's name
// lower-cased, else any name equal to it ignoring ASCII case - the built-in
// motions say "leftArm" where persona models name the bone "leftarm". When
// several names match that way the smallest wins, so the result never
// depends on map order. See docs/geometry-format.md#bone-names-ignore-case.
func (p Pose) of(bone string) BonePose {
	if bp, ok := p[bone]; ok {
		return bp
	}
	if bp, ok := p[strings.ToLower(bone)]; ok {
		return bp
	}
	best, found := "", false
	for name := range p {
		if sameBone(name, bone) && (!found || name < best) {
			best, found = name, true
		}
	}
	if found {
		return p[best]
	}
	return BonePose{}
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
// still frame rather than the frame chasing it. For a viewer that draws frames
// as its own camera moves, PrepareFrames keeps the same scenes and camera and
// draws them one at a time.
func RenderFrames(opts AnimationOptions) ([]image.Image, error) {
	f, err := PrepareFrames(opts)
	if err != nil {
		return nil, err
	}
	return f.all(opts.Workers), nil
}

// Frames is an animation prepared once: its per-frame scenes and the bounding
// box they share, kept so a viewer can draw frames one at a time as its camera
// moves. RenderFrames builds the same thing and discards it after drawing every
// frame; Frames keeps it, so one frame costs one rasterization and every frame
// of an angle is framed by one camera - root and whole-body motion stay on
// screen instead of the camera chasing each pose. See docs/animation.md.
type Frames struct {
	scenes []scene
	min    fauxgl.Vector
	max    fauxgl.Vector
	scale  float64     // Options.Scale.Model applied to a camera's margin
	flat   image.Image // non-nil for a persona skin's flat crop
}

// PrepareFrames builds every frame of an animation, and the one camera they
// share, without rasterizing anything. It is RenderFrames split in two; draw
// the result with Frames.Draw.
func PrepareFrames(opts AnimationOptions) (*Frames, error) {
	if opts.Animation == nil {
		return nil, ErrNoAnimation
	}
	if m, ok := opts.Animation.(Motion); ok {
		if _, err := ParseMotion(string(m)); err != nil {
			return nil, err
		}
	}
	fps, frames := opts.timing()
	f := &Frames{scenes: make([]scene, frames), scale: opts.Scale.Model}
	var sweep []*fauxgl.Triangle
	for i := range f.scenes {
		sc, err := opts.Options.scene(opts.Animation.Pose(float64(i) / float64(fps)))
		if err != nil {
			return nil, err
		}
		if sc.flat != nil {
			// Geometry that draws nothing has nothing to move: every frame
			// is the flat crop.
			f.flat = sc.flat
			return f, nil
		}
		f.scenes[i] = sc
		sweep = append(sweep, sc.framing()...)
	}
	f.min, f.max = boundingBoxOf(sweep)
	return f, nil
}

// Len is how many frames the animation has.
func (f *Frames) Len() int { return len(f.scenes) }

// Draw rasterizes frame i at size, using the camera the frames were prepared
// with unless cam is set, when it refits the shared framing to cam. Refitting
// is what turns a draw into an orbit: every frame at one camera still shares a
// single framing. i wraps into range. A persona skin's flat crop is returned as
// prepared, whatever size was asked for.
func (f *Frames) Draw(i, size int, cam *Camera) image.Image {
	if f.flat != nil {
		return f.flat
	}
	sc := f.scenes[0]
	fov, margin, yaw, pitch := sc.fov, sc.margin, sc.yaw, sc.pitch
	if cam != nil {
		if cam.FOV > 0 {
			fov = cam.FOV
		}
		if cam.Margin > 0 {
			margin = cam.Margin
		}
		if f.scale > 0 {
			// scene() divides a camera's margin by Scale.Model, so a refit
			// has to as well or a scaled model frames differently.
			margin /= f.scale
		}
		yaw, pitch = cam.Yaw, cam.Pitch
	}
	if size <= 0 {
		size = sc.size
	}
	i %= len(f.scenes)
	if i < 0 {
		i += len(f.scenes)
	}
	eye, center := cameraForBounds(f.min, f.max, fov, margin, yaw, pitch)
	return rasterize(f.scenes[i].layers, eye, center, fov, size)
}

// all rasterizes every frame with the shared camera - the batch RenderFrames
// uses.
func (f *Frames) all(workers int) []image.Image {
	frames := len(f.scenes)
	out := make([]image.Image, frames)
	if f.flat != nil {
		for i := range out {
			out[i] = f.flat
		}
		return out
	}
	sc := f.scenes[0]
	eye, center := cameraForBounds(f.min, f.max, sc.fov, sc.margin, sc.yaw, sc.pitch)
	if workers <= 0 {
		workers = runtime.GOMAXPROCS(0)
	}
	workers = min(workers, frames)
	if workers == 1 {
		for i := range f.scenes {
			out[i] = rasterize(f.scenes[i].layers, eye, center, sc.fov, sc.size)
		}
		return out
	}
	// Each frame has its own buffers and its own slot in out, so the
	// workers share nothing but the read-only scenes and textures.
	next := make(chan int, frames)
	for i := range f.scenes {
		next <- i
	}
	close(next)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				out[i] = rasterize(f.scenes[i].layers, eye, center, sc.fov, sc.size)
			}
		}()
	}
	wg.Wait()
	return out
}

// RenderGIF renders the motion as a looping animated GIF. GIF holds 256
// colours a frame, so the frames share a palette of the colours they use most
// (exact for most skins, which use fewer), and transparency is on or off per
// pixel, as the renderer's alpha test already makes it.
func RenderGIF(opts AnimationOptions) ([]byte, error) {
	var buf bytes.Buffer
	if err := WriteGIF(&buf, opts); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// WriteGIF renders the animation and writes the GIF to w - an HTTP response,
// a file - without holding the encoded bytes first. It writes the same bytes
// RenderGIF returns.
func WriteGIF(w io.Writer, opts AnimationOptions) error {
	frames, err := RenderFrames(opts)
	if err != nil {
		return err
	}
	fps, _ := opts.timing()
	return encodeGIF(w, frames, fps)
}

// encodeGIF writes frames as a looping GIF at fps frames a second.
func encodeGIF(w io.Writer, frames []image.Image, fps int) error {
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
	return gif.EncodeAll(w, anim)
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

// WriteGIF is the method form of WriteGIF.
func (o AnimationOptions) WriteGIF(w io.Writer) error { return WriteGIF(w, o) }
