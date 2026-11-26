package observermap

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"strings"
)

// ProfileInfo is the small subset of an ICC profile relevant to observer and
// white-point provenance. It is not a full ICC parser.
type ProfileInfo struct {
	Path             string   `json:"path,omitempty"`
	OnDiskSize       int64    `json:"on_disk_size"`
	HeaderSize       uint32   `json:"header_size_field"`
	Version          string   `json:"version"`
	DeviceClass      string   `json:"device_class"`
	ColorSpace       string   `json:"color_space"`
	PCS              string   `json:"pcs"`
	PCSIlluminant    XYZ      `json:"pcs_illuminant_d50"`
	MediaWhitePoint  *XYZ     `json:"media_white_point,omitempty"`
	TagSignatures    []string `json:"tag_signatures,omitempty"`
	HasObserverField bool     `json:"has_observer_field"`
}

var deviceClassNames = map[string]string{
	"scnr": "input device (scanner/camera)",
	"mntr": "display (monitor)",
	"prtr": "output device (printer)",
	"link": "device link",
	"spac": "color space conversion",
	"abst": "abstract",
	"nmcl": "named color",
}

func sig(b []byte) string {
	return strings.TrimRight(strings.TrimSpace(string(b)), "\x00")
}

func s15Fixed16(b []byte, off int) float64 {
	raw := int32(binary.BigEndian.Uint32(b[off : off+4]))
	return float64(raw) / 65536.0
}

// ParseProfileBytes parses the ICC header and tag table from raw bytes.
func ParseProfileBytes(data []byte) (*ProfileInfo, error) {
	if len(data) < 132 {
		return nil, fmt.Errorf("icc profile too short: %d bytes (need >=132)", len(data))
	}
	if s := sig(data[36:40]); s != "acsp" {
		return nil, fmt.Errorf("not an ICC profile: signature at offset 36 is %q, want \"acsp\"", s)
	}

	headerSize := binary.BigEndian.Uint32(data[0:4])
	major := data[8]
	minor := data[9] >> 4
	bugfix := data[9] & 0x0f
	classSig := sig(data[12:16])
	pi := &ProfileInfo{
		HeaderSize: headerSize,
		Version:    fmt.Sprintf("%d.%d.%d", major, minor, bugfix),
		DeviceClass: func() string {
			if name, ok := deviceClassNames[classSig]; ok {
				return classSig + " (" + name + ")"
			}
			return classSig
		}(),
		ColorSpace: sig(data[16:20]),
		PCS:        sig(data[20:24]),
		// Header PCS illuminant is encoded as three s15Fixed16 values,
		// conventionally D50 with Y=1.0; rescale to the Y=100 convention.
		PCSIlluminant: XYZ{
			X: s15Fixed16(data, 68) * 100,
			Y: s15Fixed16(data, 72) * 100,
			Z: s15Fixed16(data, 76) * 100,
		},
	}

	tagCount := binary.BigEndian.Uint32(data[128:132])
	// Defend against absurd counts on truncated files.
	if int(tagCount) > (len(data)-132)/12 {
		return nil, fmt.Errorf("icc tag count %d exceeds file capacity", tagCount)
	}
	for i := uint32(0); i < tagCount; i++ {
		ent := 132 + int(i)*12
		tagSig := sig(data[ent : ent+4])
		off := int(binary.BigEndian.Uint32(data[ent+4 : ent+8]))
		size := int(binary.BigEndian.Uint32(data[ent+8 : ent+12]))
		pi.TagSignatures = append(pi.TagSignatures, tagSig)

		// ICC.1 defines no standard observer tag; this check exists only to
		// make the absence explicit if a future/private profile carries one.
		if strings.EqualFold(tagSig, "obsv") {
			pi.HasObserverField = true
		}
		if tagSig != "wtpt" {
			continue
		}
		if off <= 0 || size < 20 || off+size > len(data) {
			return nil, errors.New("icc wtpt tag has out-of-range offset/size")
		}
		// Compare raw bytes: the ICC type signature is "XYZ " with a
		// significant trailing space, which sig() would trim.
		if s := string(data[off : off+4]); s != "XYZ " {
			return nil, fmt.Errorf("wtpt tag type is %q, want \"XYZ \"", s)
		}
		pi.MediaWhitePoint = &XYZ{
			X: s15Fixed16(data, off+8) * 100,
			Y: s15Fixed16(data, off+12) * 100,
			Z: s15Fixed16(data, off+16) * 100,
		}
	}
	return pi, nil
}

// ParseProfileFile reads and parses an ICC profile at path.
func ParseProfileFile(path string) (*ProfileInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	pi, err := ParseProfileBytes(data)
	if err != nil {
		return nil, err
	}
	pi.Path = path
	if st, err := os.Stat(path); err == nil {
		pi.OnDiskSize = st.Size()
	}
	return pi, nil
}
