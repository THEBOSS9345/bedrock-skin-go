package bedrockskin

import (
	"testing"

	"github.com/fogleman/fauxgl"
)

// limbEnd is where a 1x10x1 limb hanging from a pivot at the origin ends up,
// in world space, after rotating its bone by r: the centre of its lowest
// face.
func limbEnd(t *testing.T, pivotX float64, r [3]float64) fauxgl.Vector {
	t.Helper()
	geo := Geometry{TextureWidth: 64, TextureHeight: 64, Bones: []Bone{{
		Name: "limb", Pivot: []float64{pivotX, 10, 0}, Rotation: r[:],
		Cubes: []Cube{{Origin: []float64{pivotX - 0.5, 0, -0.5}, Size: []float64{1, 10, 1}, UV: []byte(`[0,0]`)}},
	}}}
	tris := buildTriangles(geo, nil, nil)
	if len(tris) == 0 {
		t.Fatal("no triangles")
	}
	// The vertex furthest from the pivot is the limb's end.
	pivot := fauxgl.Vector{X: -pivotX, Y: 10} // world space: X is mirrored
	var end fauxgl.Vector
	best := -1.0
	for _, tr := range tris {
		for _, v := range []fauxgl.Vertex{tr.V1, tr.V2, tr.V3} {
			if d := v.Position.Distance(pivot); d > best {
				best, end = d, v.Position
			}
		}
	}
	return end
}

// Each axis turns the way Minecraft's own player animations need it to. A
// model faces -Z; the right arm and leg hang on the model's negative X, the
// viewer's left once mirrored into the world (positive world X).
func TestRotationDirections(t *testing.T) {
	// animation.player.riding.legs lifts the legs forward with X = -81, so a
	// negative X swings a hanging limb forward (toward -Z).
	if end := limbEnd(t, -2, [3]float64{-81, 0, 0}); end.Z > -5 {
		t.Errorf("X -81: limb end at z=%.1f, want well in front (negative z)", end.Z)
	}
	// animation.player.bob drifts the right arm out from the body with a
	// positive Z (and the left with a negative one).
	if end := limbEnd(t, -5, [3]float64{0, 0, 30}); end.X <= 5 {
		t.Errorf("right arm Z +30: end at world x=%.1f, want outward of its shoulder at x=5", end.X)
	}
	if end := limbEnd(t, 5, [3]float64{0, 0, -30}); end.X >= -5 {
		t.Errorf("left arm Z -30: end at world x=%.1f, want outward of its shoulder at x=-5", end.X)
	}
	// riding.legs then splays the raised right leg outward with Y +18 (the
	// left with -18).
	if end := limbEnd(t, -2, [3]float64{-81, 18, 0}); end.X <= 2 {
		t.Errorf("right leg X -81 Y +18: end at world x=%.1f, want splayed outward of x=2", end.X)
	}
}
