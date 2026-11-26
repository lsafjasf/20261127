package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"

	"colorfix/internal/fixture"
)

func main() {
	out := flag.String("out", "demo/mislabeled-601-as-709.mp4", "output MP4")
	ffmpeg := flag.String("ffmpeg", "ffmpeg", "ffmpeg executable")
	frames := flag.Int("frames", 50, "number of frames")
	fps := flag.Int("fps", 25, "frame rate")
	blend := flag.Float64("blend", 0, "mix BT.601 and BT.709 encodings, 0..1")
	flag.Parse()
	rawFrame := fixture.YUV420FrameBlend(720, 576, *blend)
	raw := bytes.Repeat(rawFrame, *frames)
	if err := fixture.WriteMislabeled601(*ffmpeg, raw, *out, 720, 576, *frames, *fps); err != nil {
		fmt.Fprintln(os.Stderr, "colorfixture:", err)
		os.Exit(1)
	}
	fmt.Println(*out)
}
