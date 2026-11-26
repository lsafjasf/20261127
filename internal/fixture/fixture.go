package fixture

import (
	"fmt"
	"os"
	"path/filepath"

	"proalpha/internal/imagex"
	"proalpha/internal/mediaio"
	"proalpha/internal/synthetic"
)

type Paths struct {
	Root             string
	StraightPNG      string
	PremultipliedPNG string
	SourceProRes     string
	DroppedProxy     string
	PremulProxy      string
}

func Generate(root string, frameCount, width, height int) (*Paths, error) {
	if frameCount <= 0 {
		frameCount = 4
	}
	if width <= 0 {
		width = 96
	}
	if height <= 0 {
		height = 54
	}
	straightDir := filepath.Join(root, "png-straight")
	premulDir := filepath.Join(root, "png-premultiplied")
	for _, dir := range []string{root, straightDir, premulDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}

	for i := 0; i < frameCount; i++ {
		frame := synthetic.Frame(width, height, i, frameCount)
		name := fmt.Sprintf("frame_%04d.png", i)
		if err := imagex.SavePNG(filepath.Join(straightDir, name), frame); err != nil {
			return nil, err
		}
		premultiplied := imagex.Premultiply(frame)
		if err := imagex.SavePNG(filepath.Join(premulDir, name), premultiplied); err != nil {
			return nil, err
		}
	}

	paths := &Paths{
		Root:             root,
		StraightPNG:      filepath.Join(straightDir, "frame_%04d.png"),
		PremultipliedPNG: filepath.Join(premulDir, "frame_%04d.png"),
		SourceProRes:     filepath.Join(root, "source-prores4444-alpha.mov"),
		DroppedProxy:     filepath.Join(root, "proxy-prores-dropped-alpha-wrong-color.mov"),
		PremulProxy:      filepath.Join(root, "proxy-prores-premultiplied-rgb.mov"),
	}
	if err := encodeSource(paths); err != nil {
		return nil, err
	}
	if err := encodeDropped(paths); err != nil {
		return nil, err
	}
	if err := encodePremul(paths); err != nil {
		return nil, err
	}
	return paths, nil
}

func encodeSource(paths *Paths) error {
	return mediaio.RunFFmpeg(
		"-y", "-hide_banner", "-loglevel", "error",
		"-framerate", "24", "-start_number", "0", "-i", paths.StraightPNG,
		"-an", "-c:v", "prores_ks", "-profile:v", "4444",
		"-pix_fmt", "yuva444p10le", "-alpha_bits", "16",
		"-color_primaries", "bt709", "-color_trc", "bt709",
		"-colorspace", "bt709", "-color_range", "tv",
		paths.SourceProRes,
	)
}

func encodeDropped(paths *Paths) error {
	return mediaio.RunFFmpeg(
		"-y", "-hide_banner", "-loglevel", "error",
		"-framerate", "24", "-start_number", "0", "-i", paths.StraightPNG,
		"-an", "-c:v", "prores_ks", "-profile:v", "proxy",
		"-pix_fmt", "yuv422p10le",
		"-color_primaries", "smpte170m", "-color_trc", "smpte170m",
		"-colorspace", "smpte170m", "-color_range", "tv",
		paths.DroppedProxy,
	)
}

func encodePremul(paths *Paths) error {
	return mediaio.RunFFmpeg(
		"-y", "-hide_banner", "-loglevel", "error",
		"-framerate", "24", "-start_number", "0", "-i", paths.PremultipliedPNG,
		"-an", "-c:v", "prores_ks", "-profile:v", "4444",
		"-pix_fmt", "yuva444p10le", "-alpha_bits", "16",
		"-color_primaries", "bt709", "-color_trc", "bt709",
		"-colorspace", "bt709", "-color_range", "tv",
		paths.PremulProxy,
	)
}
