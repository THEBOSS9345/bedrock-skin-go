package bedrockskin

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// Animator is anything that poses a model over time: a built-in Motion, or an
// Animation loaded from a Bedrock animation file.
type Animator interface {
	// Duration is the length of one loop, in seconds.
	Duration() float64
	// Pose is the pose t seconds in.
	Pose(t float64) Pose
}

// Animation is one animation from a Bedrock animation file - the format
// Blockbench exports and Minecraft's own resource packs use. See
// docs/animation.md#animation-files.
type Animation struct {
	// Name is its key in the file, e.g. "animation.player.wave".
	Name string
	// Loop is whether it starts over at the end; HoldOnLastFrame keeps the
	// last pose instead (as does not looping, once it has finished).
	Loop            bool
	HoldOnLastFrame bool
	// Length is how long it runs, in seconds: animation_length, or the last
	// keyframe's time when the file leaves it out.
	Length float64

	timeUpdate *molang // anim_time_update: what drives the animation's clock
	bones      map[string]*boneAnimation
	boneNames  []string // as the file spells them, sorted
}

// Bones lists the bones the animation moves, as the file names them.
func (a *Animation) Bones() []string {
	return append([]string(nil), a.boneNames...)
}

// MissingBones lists the bones the animation moves that g does not have, so
// those parts of it will do nothing on that model - typically an animation
// made for another entity (a wing, a tail). Names match case-insensitively,
// as in game. Empty means every part of it applies.
func (a *Animation) MissingBones(g Geometry) []string {
	have := make(map[string]bool, len(g.Bones))
	for _, b := range g.Bones {
		have[strings.ToLower(b.Name)] = true
	}
	var out []string
	for _, n := range a.boneNames {
		if !have[strings.ToLower(n)] {
			out = append(out, n)
		}
	}
	return out
}

type boneAnimation struct {
	rotation, position, scale *channel
}

// channel is one of a bone's rotation, position or scale: a value for all
// time (an expression per axis), or keyframes.
type channel struct {
	always  *[3]*molang
	keys    []keyframe
	uniform bool // a single value for all three axes (scale's 2.0)
}

type keyframe struct {
	t         float64
	pre, post [3]*molang
	lerp      string // "linear", "catmullrom" or "step"
}

// ErrNoAnimations is returned by ParseAnimations for a file with no
// animations in it.
var ErrNoAnimations = errors.New("bedrockskin: no animations in the file")

// ParseAnimations reads a Bedrock animation file, returning its animations by
// name. Every Molang expression in it is compiled, so a syntax error is
// reported here, naming the animation, bone and channel it is in; names an
// expression reads that scout does not model are simply 0 when it runs.
func ParseAnimations(raw []byte) (map[string]*Animation, error) {
	var doc struct {
		Animations map[string]json.RawMessage `json:"animations"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("bedrockskin: animation file: %w", err)
	}
	if len(doc.Animations) == 0 {
		return nil, ErrNoAnimations
	}
	out := map[string]*Animation{}
	for name, rawAnim := range doc.Animations {
		a, err := parseAnimation(name, rawAnim)
		if err != nil {
			return nil, err
		}
		out[name] = a
	}
	return out, nil
}

func parseAnimation(name string, raw json.RawMessage) (*Animation, error) {
	var doc struct {
		Loop       json.RawMessage            `json:"loop"`
		Length     *float64                   `json:"animation_length"`
		TimeUpdate json.RawMessage            `json:"anim_time_update"`
		Bones      map[string]json.RawMessage `json:"bones"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("bedrockskin: animation %s: %w", name, err)
	}
	a := &Animation{Name: name, bones: map[string]*boneAnimation{}}
	switch strings.TrimSpace(string(doc.Loop)) {
	case "true":
		a.Loop = true
	case `"hold_on_last_frame"`:
		a.HoldOnLastFrame = true
	}
	if len(doc.TimeUpdate) > 0 {
		m, err := molangValue(doc.TimeUpdate)
		if err != nil {
			return nil, fmt.Errorf("bedrockskin: animation %s: anim_time_update: %w", name, err)
		}
		a.timeUpdate = m
	}
	last := 0.0
	for bone, rawBone := range doc.Bones {
		var chans map[string]json.RawMessage
		if err := json.Unmarshal(rawBone, &chans); err != nil {
			return nil, fmt.Errorf("bedrockskin: animation %s, bone %s: %w", name, bone, err)
		}
		ba := &boneAnimation{}
		for chName, rawCh := range chans {
			ch, err := parseChannel(rawCh)
			if err != nil {
				return nil, fmt.Errorf("bedrockskin: animation %s, bone %s, %s: %w", name, bone, chName, err)
			}
			if n := len(ch.keys); n > 0 {
				last = math.Max(last, ch.keys[n-1].t)
			}
			switch strings.ToLower(chName) {
			case "rotation":
				ba.rotation = ch
			case "position":
				ba.position = ch
			case "scale":
				ba.scale = ch
			}
		}
		// Bone names match the geometry's case-insensitively, as in game.
		a.bones[strings.ToLower(bone)] = ba
		a.boneNames = append(a.boneNames, bone)
	}
	sort.Strings(a.boneNames)
	a.Length = last
	if doc.Length != nil && *doc.Length > 0 {
		a.Length = *doc.Length
	}
	return a, nil
}

// parseChannel reads a channel's value: a number, a string expression, an
// array of them, or an object of keyframes by time.
func parseChannel(raw json.RawMessage) (*channel, error) {
	trimmed := strings.TrimSpace(string(raw))
	if strings.HasPrefix(trimmed, "{") {
		var frames map[string]json.RawMessage
		if err := json.Unmarshal(raw, &frames); err != nil {
			return nil, err
		}
		ch := &channel{}
		for ts, rawKey := range frames {
			t, err := strconv.ParseFloat(strings.TrimSpace(ts), 64)
			if err != nil {
				return nil, fmt.Errorf("keyframe time %q is not a number", ts)
			}
			k, uniform, err := parseKeyframe(t, rawKey)
			if err != nil {
				return nil, fmt.Errorf("keyframe %s: %w", ts, err)
			}
			ch.uniform = ch.uniform || uniform
			ch.keys = append(ch.keys, k)
		}
		sort.Slice(ch.keys, func(i, j int) bool { return ch.keys[i].t < ch.keys[j].t })
		return ch, nil
	}
	v, uniform, err := parseVector(raw)
	if err != nil {
		return nil, err
	}
	return &channel{always: &v, uniform: uniform}, nil
}

func parseKeyframe(t float64, raw json.RawMessage) (keyframe, bool, error) {
	k := keyframe{t: t, lerp: "linear"}
	if !strings.HasPrefix(strings.TrimSpace(string(raw)), "{") {
		v, uniform, err := parseVector(raw)
		k.pre, k.post = v, v
		return k, uniform, err
	}
	var obj struct {
		Pre  json.RawMessage `json:"pre"`
		Post json.RawMessage `json:"post"`
		Lerp string          `json:"lerp_mode"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return k, false, err
	}
	if obj.Lerp != "" {
		k.lerp = strings.ToLower(obj.Lerp)
	}
	var uniform bool
	switch {
	case len(obj.Pre) > 0 && len(obj.Post) > 0:
		pre, u1, err := parseVector(obj.Pre)
		if err != nil {
			return k, false, err
		}
		post, u2, err := parseVector(obj.Post)
		if err != nil {
			return k, false, err
		}
		k.pre, k.post, uniform = pre, post, u1 || u2
	case len(obj.Post) > 0:
		v, u, err := parseVector(obj.Post)
		if err != nil {
			return k, false, err
		}
		k.pre, k.post, uniform = v, v, u
	case len(obj.Pre) > 0:
		v, u, err := parseVector(obj.Pre)
		if err != nil {
			return k, false, err
		}
		k.pre, k.post, uniform = v, v, u
	default:
		return k, false, errors.New("a keyframe needs pre or post")
	}
	return k, uniform, nil
}

// parseVector reads a value for three axes: an array of three numbers or
// expressions, or one value for all three (a number, a string, or a
// one-element array). It reports whether it was one value.
func parseVector(raw json.RawMessage) ([3]*molang, bool, error) {
	var out [3]*molang
	trimmed := strings.TrimSpace(string(raw))
	if strings.HasPrefix(trimmed, "[") {
		var items []json.RawMessage
		if err := json.Unmarshal(raw, &items); err != nil {
			return out, false, err
		}
		switch len(items) {
		case 1:
			m, err := molangValue(items[0])
			return [3]*molang{m, m, m}, true, err
		case 3:
			for i := range out {
				m, err := molangValue(items[i])
				if err != nil {
					return out, false, err
				}
				out[i] = m
			}
			return out, false, nil
		case 4:
			return out, false, errors.New("quaternion rotations are not supported; export Euler rotations from Blockbench")
		}
		return out, false, fmt.Errorf("a value has %d parts; want 1 or 3", len(items))
	}
	m, err := molangValue(raw)
	return [3]*molang{m, m, m}, true, err
}

// molangValue compiles a JSON number or string as an expression.
func molangValue(raw json.RawMessage) (*molang, error) {
	var n float64
	if err := json.Unmarshal(raw, &n); err == nil {
		return &molang{stmts: []mStmt{{kind: mExprStmt, expr: mNumber(n)}}}, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("a value must be a number or a Molang string, not %s", raw)
	}
	if strings.TrimSpace(s) == "" {
		return &molang{stmts: []mStmt{{kind: mExprStmt, expr: mNumber(0)}}}, nil
	}
	return compileMolang(s)
}

// Duration is how long one loop lasts: Length, or a second for an animation
// with no keyframes (all expressions), which has no natural end.
func (a *Animation) Duration() float64 {
	if a.Length > 0 {
		return a.Length
	}
	return 1
}

// walkSpeed is the blocks a second Animation's movement queries report: about
// a player walking, so a walk cycle driven by distance moved plays at its
// in-game pace.
const walkSpeed = 4.3

// Pose is the animation's pose t seconds in. Its clock is t, or what
// anim_time_update makes of it, looped or held at the end as the file says.
func (a *Animation) Pose(t float64) Pose {
	env := &molangEnv{variables: map[string]float64{}, queries: map[string]float64{
		"life_time":               t,
		"delta_time":              1.0 / 20,
		"modified_distance_moved": t * walkSpeed,
		"distance_moved":          t * walkSpeed,
		"walk_distance":           t * walkSpeed,
		"ground_speed":            walkSpeed,
		"modified_move_speed":     1,
		"anim_speed":              1,
		"is_on_ground":            1,
		"is_alive":                1,
		"health":                  20,
		"max_health":              20,
	}}
	at := t
	if a.timeUpdate != nil {
		env.queries["anim_time"] = t
		at = a.timeUpdate.eval(env)
	}
	if a.Length > 0 {
		switch {
		case a.Loop:
			at = math.Mod(at, a.Length)
			if at < 0 {
				at += a.Length
			}
		default: // played once, or held: the end pose stays
			at = math.Min(at, a.Length)
		}
	}
	env.queries["anim_time"] = at
	env.queries["anim_pos"] = at

	pose := Pose{}
	for bone, ba := range a.bones {
		var bp BonePose
		if ba.rotation != nil {
			bp.Rotation = ba.rotation.value(at, env)
		}
		if ba.position != nil {
			bp.Position = ba.position.value(at, env)
		}
		if ba.scale != nil {
			bp.Scale, bp.Scaled = ba.scale.value(at, env), true
		}
		pose[bone] = bp
	}
	return pose
}

// value is the channel at time at: its expressions, or its keyframes
// interpolated. Before the first keyframe its value holds, as after the last.
func (c *channel) value(at float64, env *molangEnv) [3]float64 {
	if c.always != nil {
		return evalVec(*c.always, env)
	}
	keys := c.keys
	if len(keys) == 0 {
		return [3]float64{}
	}
	if at <= keys[0].t {
		return evalVec(keys[0].pre, env)
	}
	last := keys[len(keys)-1]
	if at >= last.t {
		return evalVec(last.post, env)
	}
	i := sort.Search(len(keys), func(i int) bool { return keys[i].t > at }) - 1
	k1, k2 := keys[i], keys[i+1]
	from, to := evalVec(k1.post, env), evalVec(k2.pre, env)
	if k1.lerp == "step" {
		return from
	}
	f := (at - k1.t) / (k2.t - k1.t)
	if k1.lerp == "catmullrom" || k2.lerp == "catmullrom" {
		// Through the neighbouring keyframes; a missing one is its
		// neighbour repeated.
		before, after := from, to
		if i > 0 {
			before = evalVec(keys[i-1].post, env)
		}
		if i+2 < len(keys) {
			after = evalVec(keys[i+2].pre, env)
		}
		var out [3]float64
		for ax := range out {
			out[ax] = catmullRom(before[ax], from[ax], to[ax], after[ax], f)
		}
		return out
	}
	var out [3]float64
	for ax := range out {
		out[ax] = from[ax] + (to[ax]-from[ax])*f
	}
	return out
}

func evalVec(v [3]*molang, env *molangEnv) [3]float64 {
	return [3]float64{v[0].eval(env), v[1].eval(env), v[2].eval(env)}
}

// catmullRom is the uniform Catmull-Rom spline through p1 and p2 at t in
// 0..1, shaped by p0 and p3.
func catmullRom(p0, p1, p2, p3, t float64) float64 {
	t2, t3 := t*t, t*t*t
	return 0.5 * (2*p1 + (-p0+p2)*t + (2*p0-5*p1+4*p2-p3)*t2 + (-p0+3*p1-3*p2+p3)*t3)
}
