# Animation

The library poses a model and renders it over time, two ways:

- **Built-in motions** — Minecraft's own player movements (walk, idle, wave, sneak), recreated in code. Nothing to load.
- **Animation files** — Bedrock animation JSON, the format Blockbench exports and Minecraft's resource packs use. Animations someone already made in Blockbench play here as they do in game.

Both pose bones by name, so they work on any model using the standard bone names — `root`, `body`, `head`, `rightArm`, `leftArm`, `rightLeg`, `leftLeg` — custom models included. Whatever is parented under a bone moves with it: a sleeve with its arm, a hat with the head, a backpack with the body.

```go
// A built-in motion, as an animated GIF.
gifBytes, err := skinapi.RenderGIF(skinapi.AnimationOptions{
	Options:   skinapi.Options{Texture: tex, Geometry: geos, Size: 256},
	Animation: skinapi.MotionWalk,
})

// An animation from a Blockbench export.
anims, err := skinapi.ParseAnimations(fileBytes)
frames, err := skinapi.RenderFrames(skinapi.AnimationOptions{
	Options:   skinapi.Options{Texture: tex, Size: 256},
	Animation: anims["animation.player.wave"],
	FPS:       30,
})

// One pose, as a still.
img, err := skinapi.Render(skinapi.Options{Texture: tex, Pose: skinapi.MotionSneak.Pose(0)})
```

`RenderFrames` builds every frame, then fits one camera around the whole sweep, so the model moves inside a still frame instead of the frame zooming to chase it. `RenderGIF` encodes those frames as a looping GIF: 256 colours a frame, shared across frames (exact for most skins, which use fewer), with on/off transparency as the renderer's alpha test already produces.

## Poses

A `Pose` is a `BonePose` per bone name: a `Rotation` added to the bone's own (degrees, the geometry's convention — see [geometry-format.md](geometry-format.md#rotation)), a `Position` added to its offset from its parent (model units, 1/16 of a block), and, when `Scaled`, a `Scale` multiplying it about its pivot (carrying its children; `0` hides the bone). Names match bones exactly, or else case-insensitively, as animation files are matched in game.

The conventions, checked against Minecraft's own player animations (`TestRotationDirections`):

- A **negative X** swings a hanging limb **forward**: riding lifts the legs with `-81`.
- A **positive X** tips a bone's top forward: sneaking leans the whole model with `root` at `+28`.
- A **positive Z** takes the **right** arm out from the body, a negative one the left: the idle bob.

## Built-in motions

| Motion | Loop | What moves |
| --- | --- | --- |
| `walk` | 1 s | Arms swing ±40°, legs ±56°, each leg opposite its arm, as `animation.player.move.arms`/`legs`. |
| `idle` | 4 s | The arms drift out from the body and back, up to 5.7°, as `animation.player.bob`. |
| `wave` | 1 s | The right arm raised past the head, waving ±20°. |
| `sneak` | 1.6 s | Exactly `animation.player.sneaking` — the model leans forward from the feet, legs upright, body and head lowered — with a short creeping step. |

`TestVanillaSneakMatchesMotion` renders Mojang's sneak file and the built-in motion and requires the two images to be identical.

## Animation files

```json
{
  "format_version": "1.8.0",
  "animations": {
    "animation.player.wave": {
      "loop": true,
      "animation_length": 1.0,
      "bones": {
        "rightArm": {
          "rotation": {
            "0.0": [0, 0, 140],
            "0.5": { "post": [0, 0, 170], "lerp_mode": "catmullrom" },
            "1.0": [0, 0, 140]
          }
        },
        "head": { "rotation": ["math.sin(query.anim_time * 360) * 10", 0, 0] }
      }
    }
  }
}
```

`ParseAnimations` returns every animation in the file by name. An `*Animation` satisfies `Animator`, as `Motion` does, so either renders.

What is read:

| Field | Meaning |
| --- | --- |
| `loop` | `true` starts over at the end; `false` or `"hold_on_last_frame"` keeps the last pose. |
| `animation_length` | Seconds. Absent, the last keyframe's time. |
| `anim_time_update` | An expression giving the animation's clock, e.g. `query.modified_distance_moved` for a walk cycle paced by distance. |
| `bones.<name>.rotation` / `position` / `scale` | A value for all time, or keyframes by time in seconds. |

A value is a number, a Molang string, or an array of either: three for X, Y, Z, or one for all three (`"scale": 2.0` and `"scale": [2.0]` are uniform). A keyframe is such a value, or `{ "pre": …, "post": … , "lerp_mode": … }`: the value approaching the keyframe and the value leaving it, so a channel can jump. Before the first keyframe the first value holds; after the last, the last.

Interpolation, chosen per keyframe by `lerp_mode`:

- `linear` (the default) — a straight line to the next keyframe.
- `catmullrom` — a smooth uniform Catmull-Rom curve through the keyframes, shaped by the ones either side (an end keyframe stands in for its missing neighbour). Used when either keyframe of a segment asks for it, as Blockbench does.
- `step` — the value holds until the next keyframe.

Not supported: four-number quaternion rotations (export Euler rotations from Blockbench, its default), animation controllers, blending several animations, and `particle_effects`/`sound_effects` (ignored — they don't move bones). Why some Minecraft animations do nothing on a given model: [Which Minecraft animations work](#which-minecraft-animations-work).

A syntax error anywhere in the file is reported by `ParseAnimations`, naming the animation, bone and channel.

### Values in Blockbench exports

Blockbench shows a model mirrored in X, and flips values on export — position X, rotation X and Y — into the game's convention. Exported files therefore use exactly the convention the game reads and this library applies; nothing needs undoing. (From Blockbench's `keyframe.js`, which negates those axes when compiling Bedrock keyframes.)

## Molang

Expressions are Bedrock's Molang, the subset animations use:

- Numbers (`1.5`, `1.5f`), `+ - * /`, comparisons (`< > <= >= == !=`), `&& || !`, `a ? b : c` and `a ? b`, `??`, parentheses.
- `math.` functions, case-insensitive: `sin cos asin acos atan atan2` (in **degrees**, as in Bedrock), `abs ceil floor round trunc sqrt exp ln pow mod min max clamp lerp lerprotate hermite_blend min_angle`, `math.pi`. `random`, `random_integer` and `die_roll` return the middle of their range, so a render is repeatable.
- Scripts: `variable.x = …; return variable.x * 2;`. Short prefixes work: `q.`, `v.`, `t.`, `c.`.
- `this` (the channel's value before the animation) is 0: a pose is added on top of the geometry's rest pose.

Queries, as a player walking would report them: `query.anim_time` and `query.anim_pos` (the animation's clock), `query.life_time` (seconds since the start), `query.delta_time` (1/20), `query.modified_distance_moved`, `distance_moved` and `walk_distance` (4.3 blocks a second), `query.ground_speed` (4.3), `query.modified_move_speed` and `query.anim_speed` (1), `query.is_on_ground` and `query.is_alive` (1), `query.health` and `query.max_health` (20). Anything else, including variables an entity file would have set, is 0.

A division by zero gives 0, and no expression's value is ever NaN or infinite, which would make vertices vanish.

## Example animations

`ExampleAnimations()` returns 33 ready-made animations for the player model, bundled inside the library. The same files are in [`examples/animations`](../examples/animations), to read, copy or open in Blockbench:

| File | Animations |
| --- | --- |
| `emotes.animation.json` | dance, wave_both, clap, headbang, robot, bow, cheer, facepalm, shrug, point, salute, dab, yes, no, t_pose |
| `moves.animation.json` | jumping_jacks, zombie_walk, spin, sit, flap, look_around, run, ninja_run, march, swim, push_ups, airplane, levitate, backflip |
| `fighting.animation.json` | punch, kick, sword_swing, block |

Each is named `animation.player.<name>`:

```go
gifBytes, err := skinapi.RenderGIF(skinapi.AnimationOptions{
	Options:   skinapi.Options{Texture: tex, Size: 256},
	Animation: skinapi.ExampleAnimations()["animation.player.backflip"],
})
```

Between them they use every feature above: linear, smooth and stepped keyframes (`robot` moves only in steps), Molang expressions and scripts with temporary variables (`backflip` turns the model about its middle by moving `root` as it rotates), `hold_on_last_frame` (`sit`, `salute`, `dab`), and position as well as rotation. `TestExampleAnimations` checks they all parse, all render, and move only bones the player model has.

## Which Minecraft animations work

Not every animation made for Minecraft plays here, or plays the way it does in game. An animation is a list of bone names with values; this library applies it to the model it is given and nothing else. What decides whether one works:

**The bones must exist.** An animation moves bones by name. On a model without a bone it names, that part of the animation does nothing — no error. So:

- Player animations — emotes, custom walk cycles, the game's own `player.animation.json` — work on the standard player model.
- Animations for other human-shaped mobs mostly work: zombies, skeletons, villagers and the like use `head`, `body`, `rightArm`, `leftArm`, `rightLeg`, `leftLeg`.
- Animations for other bodies — a spider's legs, a dragon's wings, a fish's tail — name bones a player doesn't have, and do nothing.
- The reverse applies to custom skins: a captured model with its own bones (a costume with no `root` or `waist`, say) ignores whatever names it lacks.

`(*Animation).MissingBones(geometry)` lists the bones an animation moves that a model doesn't have; empty means all of it applies. `Bones()` lists every bone it moves.

**The game's state isn't there.** Many of Minecraft's own animations are driven by what the entity is doing: where it is looking (`query.target_x_rotation`), whether it is swimming, attacking or holding an item, its age, variables its entity file sets. Here only the clock and walking are modelled (see [Molang](#molang)); everything else reads as 0, so those parts hold still. Anything that runs on time — which is everything made in Blockbench as a standalone animation — plays in full.

**Controllers decide what plays in game.** Entity files use animation controllers (`animation_controllers/*.json`) to switch animations — walking to attacking — and blend several at once. Controllers aren't read; you pick one animation and it plays.

**Some file features aren't supported.** Four-number quaternion rotations (Blockbench's default export is Euler angles, which work), `particle_effects` and `sound_effects`.
