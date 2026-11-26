package render

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	cv "colorfix/internal/color"
	"colorfix/internal/video"
)

var glyphs = map[rune][7]string{
	'A': {"01110", "10001", "10001", "11111", "10001", "10001", "01110"},
	'B': {"11110", "10001", "10001", "11110", "10001", "10001", "11110"},
	'C': {"01110", "10001", "10000", "10000", "10000", "10001", "01110"},
	'D': {"11110", "10001", "10001", "10001", "10001", "10001", "01110"},
	'E': {"11111", "10000", "10000", "11110", "10000", "10000", "11111"},
	'F': {"11111", "10000", "10000", "11110", "10000", "10000", "10000"},
	'G': {"01110", "10001", "10000", "10111", "10001", "10001", "01110"},
	'H': {"10001", "10001", "10001", "11111", "10001", "10001", "10001"},
	'I': {"11111", "00100", "00100", "00100", "00100", "00100", "11111"},
	'L': {"10000", "10000", "10000", "10000", "10000", "10000", "11111"},
	'M': {"10001", "11011", "10101", "10101", "10001", "10001", "10001"},
	'N': {"10001", "11001", "10101", "10101", "10011", "10001", "10001"},
	'O': {"01110", "10001", "10001", "10001", "10001", "10001", "01110"},
	'P': {"11110", "10001", "10001", "11110", "10000", "10000", "10000"},
	'R': {"11110", "10001", "10001", "11110", "10100", "10010", "10001"},
	'S': {"01111", "10000", "10000", "01110", "00001", "00001", "11110"},
	'T': {"11111", "00100", "00100", "00100", "00100", "00100", "00100"},
	'U': {"10001", "10001", "10001", "10001", "10001", "10001", "01110"},
	'V': {"10001", "10001", "10001", "10001", "10001", "01010", "00100"},
	'X': {"10001", "10001", "01010", "00100", "01010", "10001", "10001"},
	'0': {"01110", "10001", "10011", "10101", "11001", "10001", "01110"},
	'1': {"00100", "01100", "00100", "00100", "00100", "00100", "01110"},
	'2': {"01110", "10001", "00001", "00110", "01000", "10000", "11111"},
	'3': {"11110", "00001", "00001", "01110", "00001", "00001", "11110"},
	'4': {"00010", "00110", "01010", "10010", "11111", "00010", "00010"},
	'5': {"11111", "10000", "11110", "00001", "00001", "00001", "11110"},
	'6': {"01110", "10000", "10000", "11110", "10001", "10001", "01110"},
	'7': {"11111", "00001", "00010", "00100", "01000", "10000", "10000"},
	'8': {"01110", "10001", "10001", "01110", "10001", "10001", "01110"},
	'9': {"01110", "10001", "10001", "01110", "10000", "10000", "01111"},
	' ': {"00000", "00000", "00000", "00000", "00000", "00000", "00000"},
	'.': {"00000", "00000", "00000", "00000", "00000", "00110", "00110"},
	':': {"00000", "00110", "00110", "00000", "00110", "00110", "00000"},
	'-': {"00000", "00000", "00000", "11111", "00000", "00000", "00000"},
}

func Decode(frame *video.Frame, matrix cv.Matrix) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, frame.Width, frame.Height))
	for y := 0; y < frame.Height; y++ {
		for x := 0; x < frame.Width; x++ {
			rgb := frame.RGB(matrix, x, y)
			img.SetRGBA(x, y, color.RGBA{
				R: cv.ClampByte(rgb.R * 255), G: cv.ClampByte(rgb.G * 255),
				B: cv.ClampByte(rgb.B * 255), A: 255,
			})
		}
	}
	return img
}

func WritePNG(path string, img image.Image) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return png.Encode(file, img)
}

func WritePair(path string, frame *video.Frame, left, right cv.Matrix, leftText, rightText string) error {
	leftImg := Decode(frame, left)
	rightImg := Decode(frame, right)
	const gap = 10
	const header = 26
	w := frame.Width*2 + gap
	h := frame.Height + header
	canvas := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(canvas, canvas.Bounds(), &image.Uniform{C: color.RGBA{15, 17, 20, 255}}, image.Point{}, draw.Src)
	draw.Draw(canvas, image.Rect(0, header, frame.Width, h), leftImg, image.Point{}, draw.Src)
	draw.Draw(canvas, image.Rect(frame.Width+gap, header, w, h), rightImg, image.Point{}, draw.Src)
	draw.Draw(canvas, image.Rect(0, 0, frame.Width, 5), &image.Uniform{C: matrixColor(left)}, image.Point{}, draw.Src)
	draw.Draw(canvas, image.Rect(frame.Width+gap, 0, w, 5), &image.Uniform{C: matrixColor(right)}, image.Point{}, draw.Src)
	drawLabel(canvas, 12, 6, strings.ToUpper(leftText), 2)
	drawLabel(canvas, frame.Width+gap+12, 6, strings.ToUpper(rightText), 2)
	draw.Draw(canvas, image.Rect(0, header-1, w, header), &image.Uniform{C: color.RGBA{50, 55, 62, 255}}, image.Point{}, draw.Src)
	return WritePNG(path, canvas)
}

func matrixColor(m cv.Matrix) color.RGBA {
	if m == cv.BT601 {
		return color.RGBA{11, 132, 78, 255}
	}
	return color.RGBA{196, 99, 10, 255}
}

func drawLabel(dst *image.RGBA, x, y int, text string, scale int) {
	cursor := x
	for _, ch := range text {
		rows, ok := glyphs[ch]
		if ok {
			for row, bits := range rows {
				for col, bit := range bits {
					if bit == '1' {
						rect := image.Rect(
							cursor+col*scale, y+row*scale,
							cursor+(col+1)*scale, y+(row+1)*scale,
						)
						draw.Draw(dst, rect, &image.Uniform{C: color.RGBA{238, 241, 244, 255}}, image.Point{}, draw.Src)
					}
				}
			}
		}
		cursor += 6 * scale
	}
}
