package bundle

import (
	"bytes"
	"io"
)

// MediaKind classifies a file by its binary signature rather than its
// extension, so that renaming a file cannot hide a real media difference.
type MediaKind string

const (
	KindImage MediaKind = "image"
	KindAudio MediaKind = "audio"
	KindVideo MediaKind = "video"
	KindOther MediaKind = "other"
)

// magic prefixes kept short on purpose: only enough bytes are needed.
var (
	pngMagic  = []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}
	jpgMagic  = []byte{0xFF, 0xD8, 0xFF}
	gifMagic1 = []byte("GIF87a")
	gifMagic2 = []byte("GIF89a")
	webpRIFF  = []byte("RIFF")
	webpWEBP  = []byte("WEBP")
	mp4ftyp   = []byte("ftyp")
	oggMagic  = []byte("OggS")
	riffMagic = []byte("RIFF")
	riffWAVE  = []byte("WAVE")
	flacMagic = []byte("fLaC")
	id3Magic  = []byte("ID3")
	matroska  = []byte{0x1A, 0x45, 0xDF, 0xA3}
)

// DetectKind sniffs at most the first 32 bytes of r. The returned reader
// is always non-nil and yields the full stream even when empty input is
// given, so callers can keep hashing without rewinding.
func DetectKind(r io.Reader) (MediaKind, io.Reader) {
	head := make([]byte, 32)
	n, _ := io.ReadFull(r, head)
	head = head[:n]
	rest := io.MultiReader(bytes.NewReader(head), r)
	return sniff(head), rest
}

func sniff(head []byte) MediaKind {
	switch {
	case bytes.HasPrefix(head, pngMagic),
		bytes.HasPrefix(head, jpgMagic),
		bytes.HasPrefix(head, gifMagic1),
		bytes.HasPrefix(head, gifMagic2),
		isWebP(head):
		return KindImage
	case isMP4Like(head):
		// ISO-BMFF family: mp4/mov/m4v video, m4a audio. Distinguish by
		// the major brand when it is known; default to video.
		brand := string(head[8:12])
		if brand == "M4A " {
			return KindAudio
		}
		return KindVideo
	case bytes.HasPrefix(head, matroska):
		return KindVideo
	case bytes.HasPrefix(head, oggMagic),
		bytes.HasPrefix(head, flacMagic),
		bytes.HasPrefix(head, id3Magic),
		isWAV(head):
		return KindAudio
	case len(head) >= 11 && bytes.Equal(head[4:11], []byte("ftypqt")):
		return KindVideo
	}
	return KindOther
}

func isWebP(head []byte) bool {
	return len(head) >= 12 && bytes.Equal(head[0:4], webpRIFF) && bytes.Equal(head[8:12], webpWEBP)
}

func isMP4Like(head []byte) bool {
	return len(head) >= 12 && bytes.Equal(head[4:8], mp4ftyp)
}

func isWAV(head []byte) bool {
	return len(head) >= 12 && bytes.Equal(head[0:4], riffMagic) && bytes.Equal(head[8:12], riffWAVE)
}
