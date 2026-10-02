package main

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"

	bedrockskin "github.com/THEBOSS9345/bedrock-skin-go"
)

func TestRun(t *testing.T) {
	dir := t.TempDir()
	img := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for i := range img.Pix {
		img.Pix[i] = 200
	}
	img.Set(0, 0, color.NRGBA{A: 255})
	png, err := bedrockskin.EncodePNG(img)
	if err != nil {
		t.Fatal(err)
	}
	skin := filepath.Join(dir, "skin.png")
	if err := os.WriteFile(skin, png, 0o644); err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct {
		args  []string
		magic string
	}{
		{[]string{skin, filepath.Join(dir, "a.png"), "avatar", "iso", "64"}, "\x89PNG"},
		{[]string{skin, filepath.Join(dir, "w.gif"), "walk", "", "48"}, "GIF89a"},
		{[]string{skin, filepath.Join(dir, "d.gif"), "dance", "front", "32"}, "GIF89a"},
	} {
		if err := run(c.args, "", skin); err != nil {
			t.Fatalf("%v: %v", c.args, err)
		}
		out, err := os.ReadFile(c.args[1])
		if err != nil || !strings.HasPrefix(string(out), c.magic) {
			t.Fatalf("%s: not a %q file (%v)", c.args[1], c.magic, err)
		}
	}

	// A mistake fails before the output file exists.
	bad := filepath.Join(dir, "bad.png")
	if err := run([]string{skin, bad, "nope"}, "", ""); err == nil {
		t.Fatal("an unknown view should fail")
	}
	if _, err := os.Stat(bad); !os.IsNotExist(err) {
		t.Fatal("a failed run left its output file behind")
	}
	if err := run([]string{skin, bad, "body", "", "big"}, "", ""); err == nil {
		t.Fatal("a size that is not a number should fail")
	}
	if err := run([]string{skin, bad}, filepath.Join(dir, "missing.json"), ""); err == nil {
		t.Fatal("a missing geometry file should fail")
	}
}
