package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "rangecheck-demo:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("rangecheck-demo", flag.ContinueOnError)
	out := fs.String("out", "demo.yuv", "output raw YUV420P path")
	frames := fs.Int("frames", 90, "number of 30 fps frames")
	width := fs.Int("width", 640, "frame width")
	height := fs.Int("height", 360, "frame height")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *width%2 != 0 || *height%2 != 0 {
		return fmt.Errorf("YUV420P demo dimensions must be even")
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil && filepath.Dir(*out) != "." {
		return err
	}
	f, err := os.Create(*out)
	if err != nil {
		return err
	}
	defer f.Close()

	for i := 0; i < *frames; i++ {
		y, cb, cr := demoFrame(i, *frames, *width, *height)
		if _, err := f.Write(y); err != nil {
			return err
		}
		if _, err := f.Write(cb); err != nil {
			return err
		}
		if _, err := f.Write(cr); err != nil {
			return err
		}
	}
	fmt.Printf("wrote %d frames %dx%d yuv420p limited-range demo to %s\n", *frames, *width, *height, *out)
	return nil
}

func demoFrame(index, total, w, h int) (y, cb, cr []byte) {
	y = make([]byte, w*h)
	cw, ch := w/2, h/2
	cb = make([]byte, cw*ch)
	cr = make([]byte, cw*ch)
	for yy := 0; yy < h; yy++ {
		for xx := 0; xx < w; xx++ {
			base := 92 + int(28*float64(xx)/float64(w)) + int(10*float64(yy)/float64(h))
			y[yy*w+xx] = byte(clamp(base, 16, 235))
		}
	}
	for i := range cb {
		cb[i] = 128
		cr[i] = 128
	}

	switch {
	case index >= total/3 && index < 2*total/3:
		drawSubtitleGlyphs(y, w, h)
		drawHighlight(y, w, h)
	case index >= 2*total/3:
		drawLegalStylizedGrade(y, w, h)
	}
	return y, cb, cr
}

func drawSubtitleGlyphs(y []byte, w, h int) {
	glyphW, glyphH, gap := w/40, h/16, w/80
	startX := w / 5
	top := h * 82 / 100
	for g := 0; g < 14; g++ {
		x0 := startX + g*(glyphW+gap)
		for yy := top; yy < top+glyphH; yy++ {
			for xx := x0; xx < x0+glyphW; xx++ {
				if (g%3 == 0 && xx > x0+glyphW/3) || (g%4 == 0 && yy > top+glyphH/2) {
					continue
				}
				y[yy*w+xx] = 0
			}
		}
	}
}

func drawHighlight(y []byte, w, h int) {
	cx, cy := w*68/100, h*28/100
	rx, ry := w/12, h/9
	for yy := cy - ry; yy <= cy+ry; yy++ {
		for xx := cx - rx; xx <= cx+rx; xx++ {
			dx := float64(xx-cx) / float64(rx)
			dy := float64(yy-cy) / float64(ry)
			if dx*dx+dy*dy <= 1 {
				y[yy*w+xx] = 255
			}
		}
	}
}

func drawLegalStylizedGrade(y []byte, w, h int) {
	for yy := 0; yy < h; yy++ {
		for xx := 0; xx < w; xx++ {
			v := int(y[yy*w+xx])
			v = 16 + (v-16)/3
			if xx > w*72/100 && yy < h*45/100 {
				v = 235
			}
			y[yy*w+xx] = byte(clamp(v, 16, 235))
		}
	}
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
