package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"
	"strconv"
	"strings"

	"alphacheck/preview"
)

func runComposite(args []string) error {
	fs := flag.NewFlagSet("composite", flag.ContinueOnError)
	colorPath := fs.String("color", "", "raw RGBA frame sequence")
	alphaPath := fs.String("alpha", "", "raw Gray alpha frame sequence; defaults to RGBA A plane")
	prefix := fs.String("out", "preview", "output PNG prefix")
	width := fs.Int("width", 0, "frame width")
	height := fs.Int("height", 0, "frame height")
	frame := fs.Int("frame", 0, "zero-based frame index in raw sequences")
	crop := fs.String("crop", "", "optional l,r,t,b crop in encoded pixels")
	asJSON := fs.Bool("json", false, "print metrics JSON")
	requireVariation := fs.Bool("require-variation", false, "treat constant alpha as a failed encoding/export assessment")
	doublePremultiply := fs.Bool("double-premultiply", false, "simulate premultiplied color multiplied by alpha again")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *colorPath == "" || *width <= 0 || *height <= 0 {
		return fmt.Errorf("composite requires --color, --width and --height")
	}
	rgbaAll, err := os.ReadFile(*colorPath)
	if err != nil {
		return err
	}
	framePixels := *width * *height
	rgbaSize := framePixels * 4
	if len(rgbaAll) < (*frame+1)*rgbaSize {
		return fmt.Errorf("color raw file too short for frame %d: need %d bytes", *frame, (*frame+1)*rgbaSize)
	}
	rgba := rgbaAll[*frame*rgbaSize : (*frame+1)*rgbaSize]

	var alpha []byte
	if *alphaPath != "" {
		alphaAll, err := os.ReadFile(*alphaPath)
		if err != nil {
			return err
		}
		alphaSize := framePixels
		if len(alphaAll) < (*frame+1)*alphaSize {
			return fmt.Errorf("alpha raw file too short for frame %d", *frame)
		}
		alpha = alphaAll[*frame*alphaSize : (*frame+1)*alphaSize]
	} else {
		alpha = make([]byte, framePixels)
		for i := 0; i < framePixels; i++ {
			alpha[i] = rgba[i*4+3]
		}
	}

	var black, white *image.RGBA
	var stats preview.Stats
	if *doublePremultiply {
		black, white, stats = preview.CompositeDoublePremultiplication(*width, *height, rgba, alpha)
	} else {
		black, white, stats = preview.Composite(*width, *height, rgba, alpha)
	}
	cropEdges, err := parseCrop(*crop)
	if err != nil {
		return err
	}
	outputs := []string{*prefix + ".black.png", *prefix + ".white.png"}
	if err := writePNG(outputs[0], black); err != nil {
		return err
	}
	if err := writePNG(outputs[1], white); err != nil {
		return err
	}
	if *crop != "" {
		cb := preview.Crop(black, cropEdges)
		cw := preview.Crop(white, cropEdges)
		outputs = append(outputs, *prefix+".black.cropped.png", *prefix+".white.cropped.png")
		if err := writePNG(outputs[2], cb); err != nil {
			return err
		}
		if err := writePNG(outputs[3], cw); err != nil {
			return err
		}
	}
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(preview.Evaluate(stats, *requireVariation)); err != nil {
			return err
		}
	} else {
		fmt.Printf("alpha min/max/mean=%d/%d/%.2f; transparent/opaque/partial=%d/%d/%d; B-W mean diff=%.2f\n",
			stats.AlphaMin, stats.AlphaMax, stats.AlphaMean,
			stats.TransparentPixels, stats.OpaquePixels, stats.PartialPixels, stats.BlackWhiteMeanDiff)
	}
	for _, p := range outputs {
		fmt.Fprintln(os.Stderr, "wrote", p)
	}
	return nil
}

func writePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		return err
	}
	return f.Close()
}

func parseCrop(s string) (preview.CropEdges, error) {
	if strings.TrimSpace(s) == "" {
		return preview.CropEdges{}, nil
	}
	parts := strings.Split(s, ",")
	if len(parts) != 4 {
		return preview.CropEdges{}, fmt.Errorf("crop must be l,r,t,b")
	}
	values := make([]int, 4)
	for i, p := range parts {
		v, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil || v < 0 {
			return preview.CropEdges{}, fmt.Errorf("invalid crop edge %q", p)
		}
		values[i] = v
	}
	return preview.CropEdges{Left: values[0], Right: values[1], Top: values[2], Bottom: values[3]}, nil
}
