package skinapi

import (
	"embed"
	"io/fs"
	"sync"
)

// The example animations ship inside the library, so they work with no
// files to hand: examples/animations holds the same files to read or load
// into Blockbench. See docs/animation.md#example-animations.
//
//go:embed examples/animations/*.json
var exampleFiles embed.FS

var (
	examplesOnce sync.Once
	examples     map[string]*Animation
)

// ExampleAnimations returns the bundled example animations by name, e.g.
// "animation.player.dance" - emotes, moves and fighting, made for the player
// model. The map is new on each call; the animations are shared and must not
// be changed.
func ExampleAnimations() map[string]*Animation {
	examplesOnce.Do(func() {
		examples = map[string]*Animation{}
		paths, _ := fs.Glob(exampleFiles, "examples/animations/*.json")
		for _, p := range paths {
			raw, _ := exampleFiles.ReadFile(p)
			anims, err := ParseAnimations(raw)
			if err != nil {
				panic("skinapi: bundled " + p + ": " + err.Error()) // TestExampleAnimations keeps them valid
			}
			for name, a := range anims {
				examples[name] = a
			}
		}
	})
	out := make(map[string]*Animation, len(examples))
	for k, v := range examples {
		out[k] = v
	}
	return out
}
