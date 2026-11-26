package fixture

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"

	"colorfix/internal/color"
)

// YUV420Frame builds a deliberately simple 720x576 test image. The top is
// 75% color bars; the lower third contains skin and saturated red patches.
func YUV420Frame(width, height int) []byte {
	return YUV420FrameBlend(width, height, 0)
}

func YUV420FrameBlend(width, height int, blend float64) []byte {
	ySize := width * height
	raw := make([]byte, ySize*3/2)
	uPlane := raw[ySize : ySize+ySize/4]
	vPlane := raw[ySize+ySize/4:]
	bars := color.Templates[0].Colors
	barWidth := float64(width) / float64(len(bars))
	barBottom := height * 5 / 8

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			rgb := sampleRGB(x, y, width, barBottom, barWidth, bars)
			yn, _, _ := blendedEncode(rgb, blend)
			raw[y*width+x] = limitedByte(yn, 16, 219)
		}
	}
	for y := 0; y < height; y += 2 {
		for x := 0; x < width; x += 2 {
			var sumU, sumV float64
			for yy := 0; yy < 2; yy++ {
				for xx := 0; xx < 2; xx++ {
					rgb := sampleRGB(x+xx, y+yy, width, barBottom, barWidth, bars)
					_, un, vn := blendedEncode(rgb, blend)
					sumU += un
					sumV += vn
				}
			}
			ci := (y/2)*(width/2) + x/2
			uPlane[ci] = limitedByte(sumU/4, 128, 112)
			vPlane[ci] = limitedByte(sumV/4, 128, 112)
		}
	}
	return raw
}

func YUV444Frame(width, height int) []byte {
	return YUV444FrameBlend(width, height, 0)
}

func YUV444FrameBlend(width, height int, blend float64) []byte {
	raw := make([]byte, width*height*3)
	bars := color.Templates[0].Colors
	barWidth := float64(width) / float64(len(bars))
	barBottom := height * 5 / 8
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			rgb := sampleRGB(x, y, width, barBottom, barWidth, bars)
			yn, un, vn := blendedEncode(rgb, blend)
			i := y*width + x
			raw[i] = limitedByte(yn, 16, 219)
			raw[width*height+i] = limitedByte(un, 128, 112)
			raw[2*width*height+i] = limitedByte(vn, 128, 112)
		}
	}
	return raw
}

func blendedEncode(rgb color.RGB, blend float64) (float64, float64, float64) {
	y601, u601, v601 := color.Encode(color.BT601, rgb.R, rgb.G, rgb.B)
	y709, u709, v709 := color.Encode(color.BT709, rgb.R, rgb.G, rgb.B)
	if blend < 0 {
		blend = 0
	}
	if blend > 1 {
		blend = 1
	}
	return y601 + (y709-y601)*blend,
		u601 + (u709-u601)*blend,
		v601 + (v709-v601)*blend
}

func sampleRGB(x, y, width, barBottom int, barWidth float64, bars [7]color.RGB) color.RGB {
	if y < barBottom {
		index := int(float64(x) / barWidth)
		if index > 6 {
			index = 6
		}
		return bars[index]
	}
	switch {
	case y < barBottom+10:
		return color.RGB{R: 0.08, G: 0.08, B: 0.08}
	case x >= 80 && x < 290 && y >= 445 && y < 545:
		t := math.Mod(float64(y-445), 40) / 40
		return color.RGB{R: 0.78 - 0.04*t, G: 0.56 - 0.035*t, B: 0.43 - 0.03*t}
	case x >= 340 && x < 600 && y >= 445 && y < 545:
		return color.RGB{R: 0.82, G: 0.035, B: 0.03}
	case x >= 640 && x < 700 && y >= 445 && y < 545:
		return color.RGB{R: 0.04, G: 0.22, B: 0.78}
	default:
		return color.RGB{R: 0.16, G: 0.18, B: 0.21}
	}
}

func limitedByte(normalized float64, offset, span float64) uint8 {
	v := offset + span*normalized
	if v < 0 {
		v = 0
	}
	if v > 255 {
		v = 255
	}
	return uint8(math.Round(v))
}

// WriteMislabeled601 encodes raw BT.601 YUV as H.264 while declaring BT.709.
// FFmpeg only performs compression and metadata writing here: no RGB->YUV
// conversion is allowed to influence the fixture.
func WriteMislabeled601(ffmpeg string, raw []byte, outPath string, width, height, frames, fps int) error {
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	args := []string{
		"-hide_banner", "-loglevel", "error", "-nostdin", "-y",
		"-f", "rawvideo", "-pix_fmt", "yuv420p",
		"-s", fmt.Sprintf("%dx%d", width, height), "-r", fmt.Sprint(fps),
		"-i", "pipe:0", "-frames:v", fmt.Sprint(frames),
		"-c:v", "libx264", "-preset", "medium", "-crf", "18",
		"-pix_fmt", "yuv420p", "-colorspace", "bt709",
		"-color_primaries", "bt709", "-color_trc", "bt709",
		"-movflags", "+faststart", outPath,
	}
	cmd := exec.Command(ffmpeg, args...)
	cmd.Stdin = bytes.NewReader(raw)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("encode fixture: %w: %s", err, stderr.String())
	}
	return nil
}
