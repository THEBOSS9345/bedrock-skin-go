package bedrockskin

import (
	"encoding/json"
	"math"

	"github.com/fogleman/fauxgl"
)

// PolyMesh is a bone's free-form mesh: the shape Bedrock sends for persona
// (character creator) skins instead of cubes. Positions are in model space,
// like a cube's origin. See docs/geometry-format.md#poly-meshes.
type PolyMesh struct {
	// NormalizedUVs means UVs are 0..1 across the texture; otherwise they
	// are texture pixels against the entry's declared texture size.
	NormalizedUVs bool        `json:"normalized_uvs"`
	Positions     [][]float64 `json:"positions"`
	Normals       [][]float64 `json:"normals"`
	UVs           [][]float64 `json:"uvs"`
	// Polys is a list of polygons, each a list of [position, normal, uv]
	// index triples, or the string "tri_list" / "quad_list" for vertices
	// taken in order.
	Polys json.RawMessage `json:"polys"`
}

// Mesh returns the bone's poly mesh, and false when it has none or it does
// not read.
func (b Bone) Mesh() (PolyMesh, bool) {
	if len(b.PolyMesh) == 0 {
		return PolyMesh{}, false
	}
	var m PolyMesh
	if json.Unmarshal(b.PolyMesh, &m) != nil {
		return PolyMesh{}, false
	}
	return m, true
}

// polyVertex is one corner of a polygon: its position and texture coordinate.
type polyVertex struct {
	pos [3]float64
	uv  [2]float64
}

// polygons resolves every polygon to its corners. A polygon that points
// outside the mesh's lists, or has fewer than three corners, is skipped.
func (m PolyMesh) polygons() [][]polyVertex {
	var idx [][][]float64
	var mode string
	if json.Unmarshal(m.Polys, &mode) == nil {
		per := 0
		switch mode {
		case "tri_list":
			per = 3
		case "quad_list":
			per = 4
		default:
			return nil
		}
		for i := 0; i+per <= len(m.Positions); i += per {
			poly := make([][]float64, per)
			for j := range poly {
				k := float64(i + j)
				poly[j] = []float64{k, k, k}
			}
			idx = append(idx, poly)
		}
	} else if json.Unmarshal(m.Polys, &idx) != nil {
		return nil
	}

	var out [][]polyVertex
	for _, poly := range idx {
		if len(poly) < 3 {
			continue
		}
		verts := make([]polyVertex, 0, len(poly))
		for _, c := range poly {
			p, okP := index(c, 0, m.Positions, 3)
			t, okT := index(c, 2, m.UVs, 2)
			if !okP || !okT {
				break
			}
			verts = append(verts, polyVertex{
				pos: [3]float64{p[0], p[1], p[2]},
				uv:  [2]float64{t[0], t[1]},
			})
		}
		if len(verts) == len(poly) {
			out = append(out, verts)
		}
	}
	return out
}

// index looks up corner[slot] in list, wanting at least n components.
func index(corner []float64, slot int, list [][]float64, n int) ([]float64, bool) {
	if slot >= len(corner) {
		return nil, false
	}
	f := corner[slot]
	if f < 0 || f >= float64(len(list)) || f != math.Trunc(f) {
		return nil, false
	}
	v := list[int(f)]
	if len(v) < n {
		return nil, false
	}
	return v, true
}

// HasMesh reports whether any bone draws something: a cube or a poly mesh.
func (g *Geometry) HasMesh() bool {
	for _, b := range g.Bones {
		if len(b.Cubes) > 0 {
			return true
		}
		if m, ok := b.Mesh(); ok && len(m.polygons()) > 0 {
			return true
		}
	}
	return false
}

// addPolyMesh appends a poly mesh's polygons, fanned into triangles, placed
// the same way addCube places a cube: through the bone's world transform from
// its pivot, then X mirrored. The UVs ride on the vertices, so the mirror
// needs no U flip of its own. See docs/rendering-pipeline.md#poly-meshes.
func addPolyMesh(triangles *[]*fauxgl.Triangle, m PolyMesh, b Bone, worldMatrix fauxgl.Matrix, texW, texH float64) {
	pivot := fauxgl.Vector{X: at(b.Pivot, 0), Y: at(b.Pivot, 1), Z: at(b.Pivot, 2)}
	for _, poly := range m.polygons() {
		verts := make([]fauxgl.Vertex, len(poly))
		for i, c := range poly {
			p := worldMatrix.MulPosition(fauxgl.Vector{X: c.pos[0], Y: c.pos[1], Z: c.pos[2]}.Sub(pivot))
			p.X = -p.X
			// Normalized UVs count V up from the bottom, as fauxgl samples;
			// pixel UVs count down from the top, as a cube's do.
			u, v := c.uv[0], c.uv[1]
			if !m.NormalizedUVs {
				u, v = u/texW, 1-v/texH
			}
			verts[i] = fauxgl.Vertex{Position: p, Texture: fauxgl.Vector{X: u, Y: v}}
		}
		for i := 1; i+1 < len(verts); i++ {
			*triangles = append(*triangles, fauxgl.NewTriangle(verts[0], verts[i], verts[i+1]))
		}
	}
}
