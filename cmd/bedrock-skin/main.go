// Command bedrock-skin renders a skin file from the command line, to try the
// library before writing any code:
//
//	go run github.com/THEBOSS9345/bedrock-skin-go/cmd/bedrock-skin@latest skin.png out.png [view] [angle] [size]
//	go run github.com/THEBOSS9345/bedrock-skin-go/cmd/bedrock-skin@latest skin.png out.gif walk
//	go run github.com/THEBOSS9345/bedrock-skin-go/cmd/bedrock-skin@latest -geometry geometry.json skin.png out.png
//
// view is body, chest, head or avatar; a motion name (walk, idle, wave,
// sneak) or an example animation (dance, backflip, ...) makes a GIF. The
// arguments are those of bedrock-skin-rs's render example.
package main

import (
	"flag"
	"fmt"
	"image"
	"os"
	"strconv"

	bedrockskin "github.com/THEBOSS9345/bedrock-skin-go"
)

func main() {
	geometry := flag.String("geometry", "", "a geometry.json for the skin's model; none draws the default model")
	cape := flag.String("cape", "", "a cape texture to wear")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: bedrock-skin [-geometry geometry.json] [-cape cape.png] <skin.png> <out.png|out.gif> [view|motion] [angle] [size]")
		flag.PrintDefaults()
	}
	flag.Parse()
	args := flag.Args()
	if len(args) < 2 {
		flag.Usage()
		os.Exit(2)
	}
	if err := run(args, *geometry, *cape); err != nil {
		fmt.Fprintln(os.Stderr, "bedrock-skin:", err)
		os.Exit(1)
	}
}

func run(args []string, geometryPath, capePath string) error {
	input, output := args[0], args[1]
	what, angleName, size := "body", "", 256
	if len(args) > 2 {
		what = args[2]
	}
	if len(args) > 3 {
		angleName = args[3]
	}
	if len(args) > 4 {
		n, err := strconv.Atoi(args[4])
		if err != nil {
			return fmt.Errorf("size %q: %w", args[4], err)
		}
		size = n
	}

	tex, err := readImage(input)
	if err != nil {
		return err
	}
	angle, err := bedrockskin.ParseAngle(angleName)
	if err != nil {
		return err
	}
	opts := bedrockskin.Options{Texture: tex, Angle: angle, Size: size}
	if geometryPath != "" {
		raw, err := os.ReadFile(geometryPath)
		if err != nil {
			return err
		}
		if opts.Geometry, err = bedrockskin.ParseGeometry(raw); err != nil {
			return fmt.Errorf("%s: %w", geometryPath, err)
		}
	}
	if capePath != "" {
		if opts.Cape, err = readImage(capePath); err != nil {
			return err
		}
	}

	// Everything is checked before the output file exists, so a mistake
	// leaves no empty file behind.
	var animation bedrockskin.Animator
	if m, err := bedrockskin.ParseMotion(what); err == nil {
		animation = m
	} else if a, ok := bedrockskin.ExampleAnimations()["animation.player."+what]; ok {
		animation = a
	} else if opts.View, err = bedrockskin.ParseView(what); err != nil {
		return fmt.Errorf("%q is not a view, a motion or an example animation", what)
	}

	f, err := os.Create(output)
	if err != nil {
		return err
	}
	defer f.Close()
	if animation != nil {
		err = bedrockskin.WriteGIF(f, bedrockskin.AnimationOptions{Options: opts, Animation: animation})
	} else {
		err = opts.WritePNG(f)
	}
	if err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	fmt.Println("wrote", output)
	return nil
}

func readImage(path string) (image.Image, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	img, err := bedrockskin.DecodeImage(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return img, nil
}
