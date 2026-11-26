package rangecheck

import (
	"bufio"
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// InputKind identifies the deterministic decoder/adapter to use.
type InputKind string

const (
	InputAuto  InputKind = "auto"
	InputImage InputKind = "image"
	InputRaw   InputKind = "raw"
	InputVideo InputKind = "video"
)

// ProcessOptions controls file traversal and optional output side effects.
type ProcessOptions struct {
	Kind        InputKind
	Settings    Settings
	Metadata    Metadata
	PTSFile     string
	FramePTS    []float64
	MaxFrames   int
	FFmpegPath  string
	FFprobePath string
	OnFrame     func(*FrameReport)
}

// ProcessPath analyzes an image, image directory, raw frame dump, or video.
func ProcessPath(path string, analyzer *Analyzer, options ProcessOptions) (*Report, error) {
	pts, err := ReadPTSFile(options.PTSFile)
	if err != nil {
		return nil, err
	}
	if len(pts) == 0 {
		pts = options.FramePTS
	}
	kind := options.Kind
	if kind == InputAuto {
		kind = detectInputKind(path)
	}

	switch kind {
	case InputImage:
		err = processImages(path, analyzer, options, pts)
	case InputRaw:
		err = processRaw(path, analyzer, options, pts)
	case InputVideo:
		err = processVideo(path, analyzer, options, pts)
	default:
		err = fmt.Errorf("unsupported input kind %q", kind)
	}
	if err != nil {
		return nil, err
	}
	return analyzer.Close()
}

func detectInputKind(path string) InputKind {
	info, err := os.Stat(path)
	if err != nil {
		return InputVideo
	}
	if info.IsDir() {
		return InputImage
	}
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".png", ".jpg", ".jpeg":
		return InputImage
	case ".yuv", ".raw", ".gray", ".rgb", ".rgba":
		return InputRaw
	default:
		return InputVideo
	}
}

// DetectInputKind exposes extension/stat-based input detection to the CLI.
func DetectInputKind(path string) InputKind { return detectInputKind(path) }

// ReadPTSFile reads either one PTS value per line or frame,pts CSV rows.
func ReadPTSFile(path string) ([]float64, error) {
	if path == "" {
		return nil, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var pts []float64
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if comma := strings.Index(line, ","); comma >= 0 {
			line = strings.TrimSpace(line[comma+1:])
		}
		v, err := strconv.ParseFloat(line, 64)
		if err != nil {
			return nil, fmt.Errorf("parse PTS %q: %w", line, err)
		}
		pts = append(pts, v)
	}
	return pts, scanner.Err()
}

func ptsForFrame(index int, pts []float64, fps Rational) float64 {
	if index < len(pts) {
		return pts[index]
	}
	if fps.Num != 0 && fps.Den != 0 {
		return float64(index) / fps.Float64()
	}
	return 0
}

func collectImages(path string) ([]string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return []string{path}, nil
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext == ".png" || ext == ".jpg" || ext == ".jpeg" {
			files = append(files, filepath.Join(path, entry.Name()))
		}
	}
	sort.Strings(files)
	if len(files) == 0 {
		return nil, fmt.Errorf("no PNG/JPEG images found in %s", path)
	}
	return files, nil
}

func processImages(path string, analyzer *Analyzer, options ProcessOptions, pts []float64) error {
	files, err := collectImages(path)
	if err != nil {
		return err
	}
	if options.MaxFrames > 0 && len(files) > options.MaxFrames {
		files = files[:options.MaxFrames]
	}
	for i, file := range files {
		f, err := os.Open(file)
		if err != nil {
			return err
		}
		img, _, err := image.Decode(f)
		f.Close()
		if err != nil {
			return fmt.Errorf("decode image %s: %w", file, err)
		}
		frame := DecodeImageToFrame(img, i, ptsForFrame(i, pts, options.Settings.FPS))
		frame.PTSSeconds = ptsForFrame(i, pts, options.Settings.FPS)
		if _, err := addFrame(analyzer, frame, options); err != nil {
			return fmt.Errorf("%s: %w", file, err)
		}
	}
	return nil
}

func rawFrameSize(s Settings) (int, error) {
	w, h := s.Width, s.Height
	if w <= 0 || h <= 0 {
		return 0, fmt.Errorf("raw input requires positive --width and --height")
	}
	switch s.PixelFormat {
	case FormatGray8, FormatRGB24:
		if s.PixelFormat == FormatGray8 {
			return w * h, nil
		}
		return w * h * 3, nil
	case FormatRGBA32:
		return w * h * 4, nil
	case FormatYUV444P:
		return w * h * 3, nil
	case FormatYUV420P:
		cw, ch := (w+1)/2, (h+1)/2
		return w*h + 2*cw*ch, nil
	default:
		return 0, fmt.Errorf("raw input requires a supported --pix-fmt, got %q", s.PixelFormat)
	}
}

func processRaw(path string, analyzer *Analyzer, options ProcessOptions, pts []float64) error {
	frameSize, err := rawFrameSize(options.Settings)
	if err != nil {
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	reader := bufio.NewReader(f)
	buffer := make([]byte, frameSize)
	for index := 0; options.MaxFrames == 0 || index < options.MaxFrames; index++ {
		_, err := io.ReadFull(reader, buffer)
		if err == io.EOF || err == io.ErrUnexpectedEOF && index > 0 {
			if err == io.ErrUnexpectedEOF {
				return fmt.Errorf("raw input ends with a partial frame at frame %d", index)
			}
			break
		}
		if err != nil {
			return err
		}
		frame, err := rawFrame(buffer, index, ptsForFrame(index, pts, options.Settings.FPS), options.Settings)
		if err != nil {
			return err
		}
		if _, err := addFrame(analyzer, frame, options); err != nil {
			return err
		}
	}
	return nil
}

func rawFrame(buffer []byte, index int, pts float64, s Settings) (Frame, error) {
	w, h := s.Width, s.Height
	frame := Frame{Index: index, PTSSeconds: pts, Width: w, Height: h, Format: s.PixelFormat}
	switch s.PixelFormat {
	case FormatGray8:
		frame.Y = append([]byte(nil), buffer...)
		frame.StrideY = w
	case FormatRGB24, FormatRGBA32:
		frame.RGB = append([]byte(nil), buffer...)
	case FormatYUV444P:
		plane := w * h
		frame.Y = append([]byte(nil), buffer[:plane]...)
		frame.Cb = append([]byte(nil), buffer[plane:2*plane]...)
		frame.Cr = append([]byte(nil), buffer[2*plane:3*plane]...)
		frame.StrideY, frame.StrideC = w, w
	case FormatYUV420P:
		cw, ch := (w+1)/2, (h+1)/2
		ySize := w * h
		cSize := cw * ch
		frame.Y = append([]byte(nil), buffer[:ySize]...)
		frame.Cb = append([]byte(nil), buffer[ySize:ySize+cSize]...)
		frame.Cr = append([]byte(nil), buffer[ySize+cSize:ySize+2*cSize]...)
		frame.StrideY, frame.StrideC = w, cw
	default:
		return Frame{}, fmt.Errorf("unsupported raw format %q", s.PixelFormat)
	}
	return frame, nil
}
