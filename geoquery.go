package skinapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// GeometryTree is a whole geometry file, every field kept - including ones
// this library has no type for - so any value in it can be picked out by a
// path. ParseGeometry gives the typed models the renderer uses; a tree is for
// reading a file. See docs/geometry-format.md#picking-values-out-of-a-file.
//
//	tree, err := skinapi.ParseGeometryTree(raw)
//	pivot, ok := tree.Get("geometry.humanoid.custom/bones/rightArm/pivot")
//	for _, v := range tree.Select("*/bones/*/cubes/*/size") { ... }
//
// Both of Bedrock's wire formats read into the same shape, the modern one:
// each model is an object with a "description" (identifier, texture_width,
// texture_height, visible_bounds_*) and "bones", so one path works on
// either.
type GeometryTree struct {
	// FormatVersion is the file's format_version, e.g. "1.12.0".
	FormatVersion string

	models []treeModel
	raw    []byte
}

type treeModel struct {
	id   string
	node map[string]any
}

// GeometryValue is one value a path picked out: its canonical path, with
// bones and other named entries by name, and the value itself - a
// map[string]any, []any, float64, string, bool or nil, as encoding/json
// reads JSON.
type GeometryValue struct {
	Path  string
	Value any
}

// ErrNoGeometryModels is returned by ParseGeometryTree for valid JSON that
// holds no geometry.
var ErrNoGeometryModels = errors.New("skinapi: no geometry models in the file")

// ParseGeometryTree reads a geometry file of either format into a tree.
func ParseGeometryTree(raw []byte) (*GeometryTree, error) {
	var top map[string]any
	if err := json.Unmarshal(raw, &top); err != nil {
		return nil, fmt.Errorf("skinapi: geometry: %w", err)
	}
	t := &GeometryTree{raw: append([]byte(nil), raw...)}
	t.FormatVersion, _ = top["format_version"].(string)

	if list, ok := top["minecraft:geometry"].([]any); ok {
		for i, m := range list {
			node, ok := m.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("skinapi: geometry: model %d is not an object", i)
			}
			desc, _ := node["description"].(map[string]any)
			id, _ := desc["identifier"].(string)
			t.models = append(t.models, treeModel{id: id, node: node})
		}
	} else {
		// Legacy: each model is a top-level key. Its texture size and bounds
		// move into a description, so paths match the modern format.
		for key, v := range top {
			node, ok := v.(map[string]any)
			if key == "format_version" || !ok {
				continue
			}
			if bones, _ := node["bones"].([]any); len(bones) == 0 {
				continue // as ParseGeometry skips it
			}
			desc := map[string]any{"identifier": key}
			out := map[string]any{"description": desc}
			for k, val := range node {
				switch k {
				case "texturewidth":
					desc["texture_width"] = val
				case "textureheight":
					desc["texture_height"] = val
				case "visible_bounds_width", "visible_bounds_height", "visible_bounds_offset":
					desc[k] = val
				default:
					out[k] = val
				}
			}
			t.models = append(t.models, treeModel{id: key, node: out})
		}
		// The same order as ParseGeometry.
		sort.Slice(t.models, func(i, j int) bool { return t.models[i].id < t.models[j].id })
	}
	if len(t.models) == 0 {
		return nil, ErrNoGeometryModels
	}
	return t, nil
}

// Identifiers lists the models' identifiers, in the same order as
// ParseGeometry.
func (t *GeometryTree) Identifiers() []string {
	out := make([]string, len(t.models))
	for i, m := range t.models {
		out[i] = m.id
	}
	return out
}

// Select returns every value the path picks out, in file order; none when
// nothing matches.
//
// A path is segments separated by "/". The first picks the model by
// identifier, the rest walk into it:
//
//   - an object's field by name: "description", "bones", "locators", "pivot"
//   - an array element by index, from 0 ("-1" is the last), or - for a list
//     of named things, such as bones - by its name: "bones/rightArm"
//   - "*" for every model, field or element at that level
//
// Names match exactly, else case-insensitively, as the game matches bones.
// A legacy model named "geometry.a:geometry.b" (one inheriting from another)
// is also picked by "geometry.a". An empty path returns every model.
func (t *GeometryTree) Select(path string) []GeometryValue {
	segs := splitPath(path)
	var out []GeometryValue
	var models []treeModel
	if len(segs) == 0 || segs[0] == "*" {
		models = t.models
	} else {
		models = t.pickModels(segs[0])
	}
	if len(segs) > 0 {
		segs = segs[1:]
	}
	for _, m := range models {
		walk(m.node, segs, m.id, &out)
	}
	return out
}

// Get returns the first value the path picks out, and whether there was one.
func (t *GeometryTree) Get(path string) (GeometryValue, bool) {
	vs := t.Select(path)
	if len(vs) == 0 {
		return GeometryValue{}, false
	}
	return vs[0], true
}

// Geometries returns the typed models: the same as ParseGeometry on the
// file.
func (t *GeometryTree) Geometries() ([]Geometry, error) {
	return ParseGeometry(t.raw)
}

func (t *GeometryTree) pickModels(seg string) []treeModel {
	for _, match := range []func(id string) bool{
		func(id string) bool { return id == seg },
		func(id string) bool { return strings.EqualFold(id, seg) },
		func(id string) bool {
			base, _, found := strings.Cut(id, ":")
			return found && strings.EqualFold(base, seg)
		},
	} {
		var out []treeModel
		for _, m := range t.models {
			if match(m.id) {
				out = append(out, m)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return nil
}

func splitPath(path string) []string {
	path = strings.Trim(strings.TrimSpace(path), "/")
	if path == "" {
		return nil
	}
	return strings.Split(path, "/")
}

func walk(v any, segs []string, path string, out *[]GeometryValue) {
	if len(segs) == 0 {
		*out = append(*out, GeometryValue{Path: path, Value: v})
		return
	}
	seg, rest := segs[0], segs[1:]
	switch node := v.(type) {
	case map[string]any:
		if seg == "*" {
			keys := make([]string, 0, len(node))
			for k := range node {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				walk(node[k], rest, path+"/"+k, out)
			}
			return
		}
		if child, ok := node[seg]; ok {
			walk(child, rest, path+"/"+seg, out)
			return
		}
		for k, child := range node {
			if strings.EqualFold(k, seg) {
				walk(child, rest, path+"/"+k, out)
				return
			}
		}
	case []any:
		if seg == "*" {
			for i, child := range node {
				walk(child, rest, path+"/"+elementName(child, i), out)
			}
			return
		}
		if i, err := strconv.Atoi(seg); err == nil {
			if i < 0 {
				i += len(node)
			}
			if i >= 0 && i < len(node) {
				walk(node[i], rest, path+"/"+elementName(node[i], i), out)
			}
			return
		}
		for _, fold := range []bool{false, true} {
			for i, child := range node {
				name := namedElement(child)
				if name != "" && (name == seg || fold && strings.EqualFold(name, seg)) {
					walk(child, rest, path+"/"+elementName(child, i), out)
					return
				}
			}
		}
	}
}

func namedElement(v any) string {
	if m, ok := v.(map[string]any); ok {
		if s, ok := m["name"].(string); ok {
			return s
		}
	}
	return ""
}

// elementName is how an array element appears in a canonical path: its
// name when it has one (bones do), else its index.
func elementName(v any, i int) string {
	if name := namedElement(v); name != "" && !strings.Contains(name, "/") {
		if _, err := strconv.Atoi(name); err != nil && name != "*" {
			return name
		}
	}
	return strconv.Itoa(i)
}

// Float returns the value as a number.
func (v GeometryValue) Float() (float64, bool) {
	f, ok := v.Value.(float64)
	return f, ok
}

// Floats returns the value as a list of numbers - a pivot, an origin, a
// size.
func (v GeometryValue) Floats() ([]float64, bool) {
	list, ok := v.Value.([]any)
	if !ok {
		return nil, false
	}
	out := make([]float64, len(list))
	for i, e := range list {
		f, ok := e.(float64)
		if !ok {
			return nil, false
		}
		out[i] = f
	}
	return out, true
}

// Text returns the value as a string - a name, a parent, an identifier.
func (v GeometryValue) Text() (string, bool) {
	s, ok := v.Value.(string)
	return s, ok
}

// Decode stores the value in dst as encoding/json would, so a bone decodes
// into a Bone, a cube into a Cube, a locator into a Locator.
func (v GeometryValue) Decode(dst any) error {
	raw, err := json.Marshal(v.Value)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, dst)
}

// JSON returns the value as JSON.
func (v GeometryValue) JSON() []byte {
	raw, _ := json.Marshal(v.Value)
	return raw
}
