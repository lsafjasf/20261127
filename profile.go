package iccproof

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
)

var (
	// ErrProfileRequired means no explicit ICC path/bytes/descriptor was supplied.
	// Callers must not respond by substituting sRGB.
	ErrProfileRequired = errors.New("corresponding ICC profile is required")
	ErrProfileNotFound = errors.New("ICC profile file not found")
	ErrInvalidProfile  = errors.New("invalid ICC profile")
	ErrBackend         = errors.New("ICC backend error")
)

// ProfileRole identifies where a profile belongs in the proofing chain.
type ProfileRole string

const (
	RoleSource  ProfileRole = "source"
	RoleMedia   ProfileRole = "media"
	RoleDisplay ProfileRole = "display"
)

// ProfileRef is an explicitly supplied profile. Exactly one source should be
// present: Path, Bytes, or Descriptor. An empty ProfileRef is an error.
type ProfileRef struct {
	Role       ProfileRole   `json:"role,omitempty"`
	Path       string        `json:"path,omitempty"`
	Bytes      []byte        `json:"-"`
	Descriptor *ColorProfile `json:"descriptor,omitempty"`
}

// ColorProfile is an explicit, non-ICC fallback descriptor. It is never
// populated implicitly with sRGB values.
type ColorProfile struct {
	ID          string  `json:"id,omitempty"`
	Description string  `json:"description,omitempty"`
	ColorSpace  string  `json:"color_space"`
	Red         XYY     `json:"red"`
	Green       XYY     `json:"green"`
	Blue        XYY     `json:"blue"`
	White       XYY     `json:"white"`
	Gamma       float64 `json:"gamma,omitempty"`
}

// ProfileInfo contains provenance and basic ICC header facts.
type ProfileInfo struct {
	Path        string   `json:"path,omitempty"`
	SHA256      string   `json:"sha256,omitempty"`
	Size        int      `json:"size"`
	Version     string   `json:"version,omitempty"`
	DeviceClass string   `json:"device_class,omitempty"`
	ColorSpace  string   `json:"color_space,omitempty"`
	PCS         string   `json:"pcs,omitempty"`
	TagCount    int      `json:"tag_count,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Backend     string   `json:"backend"`
}

// LoadedProfile contains bytes and parsed metadata needed by an engine.
type LoadedProfile struct {
	Ref  ProfileRef  `json:"ref"`
	Data []byte      `json:"-"`
	Info ProfileInfo `json:"info"`
}

type ProfileError struct {
	Role   ProfileRole
	Path   string
	Reason string
	Err    error
}

func (e *ProfileError) Error() string {
	parts := []string{string(e.Role) + " profile"}
	if e.Path != "" {
		parts = append(parts, fmt.Sprintf("%q", e.Path))
	}
	if e.Reason != "" {
		parts = append(parts, e.Reason)
	}
	msg := strings.Join(parts, " ")
	if e.Err != nil {
		msg += ": " + e.Err.Error()
	}
	return msg
}

func (e *ProfileError) Unwrap() error { return e.Err }

func newProfileRequired(role ProfileRole) error {
	return &ProfileError{
		Role:   role,
		Reason: "is missing; refusing to use a default sRGB profile",
		Err:    ErrProfileRequired,
	}
}

// LoadProfile resolves an explicit profile. It never returns a built-in sRGB
// profile when the reference is empty.
func LoadProfile(role ProfileRole, ref ProfileRef) (*LoadedProfile, error) {
	ref.Role = role
	if ref.Descriptor != nil {
		if err := ref.Descriptor.Validate(); err != nil {
			return nil, &ProfileError{Role: role, Reason: "invalid descriptor", Err: err}
		}
		digest := sha256.Sum256([]byte("descriptor:" + ref.Descriptor.ID + ":" + ref.Descriptor.Description))
		return &LoadedProfile{
			Ref: ref,
			Info: ProfileInfo{
				Backend: "reference-descriptor", SHA256: hex.EncodeToString(digest[:]),
				ColorSpace: ref.Descriptor.ColorSpace, Version: "explicit-descriptor",
			},
		}, nil
	}

	var data []byte
	var path string
	switch {
	case len(ref.Bytes) > 0:
		data = append([]byte(nil), ref.Bytes...)
		path = ref.Path
	case strings.TrimSpace(ref.Path) != "":
		path = strings.TrimSpace(ref.Path)
		var err error
		data, err = os.ReadFile(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil, &ProfileError{Role: role, Path: path, Reason: "does not exist", Err: ErrProfileNotFound}
			}
			return nil, &ProfileError{Role: role, Path: path, Reason: "cannot read", Err: err}
		}
	default:
		return nil, newProfileRequired(role)
	}

	info, err := ParseICCBytes(data)
	if err != nil {
		return nil, &ProfileError{Role: role, Path: path, Reason: "not a usable ICC profile", Err: err}
	}
	sum := sha256.Sum256(data)
	info.Path = path
	info.SHA256 = hex.EncodeToString(sum[:])
	info.Size = len(data)
	info.Backend = "littlecms2"
	return &LoadedProfile{Ref: ref, Data: data, Info: *info}, nil
}

func (p *ColorProfile) Validate() error {
	if p == nil {
		return ErrProfileRequired
	}
	if strings.ToUpper(strings.TrimSpace(p.ColorSpace)) != "RGB" {
		return fmt.Errorf("%w: reference descriptor currently supports RGB, got %q", ErrInvalidProfile, p.ColorSpace)
	}
	if p.Red.Y <= 0 || p.Green.Y <= 0 || p.Blue.Y <= 0 || p.White.Y <= 0 {
		return fmt.Errorf("%w: primaries and white require positive y", ErrInvalidProfile)
	}
	if p.Gamma == 0 {
		p.Gamma = 2.2
	}
	if p.Gamma < 1 || p.Gamma > 3.5 {
		return fmt.Errorf("%w: gamma out of supported range [1,3.5]: %v", ErrInvalidProfile, p.Gamma)
	}
	return nil
}

// ExplicitProfiles returns an error unless both source and media are supplied.
func ExplicitProfiles(source, media ProfileRef) error {
	if source.Descriptor == nil && len(source.Bytes) == 0 && strings.TrimSpace(source.Path) == "" {
		return newProfileRequired(RoleSource)
	}
	if media.Descriptor == nil && len(media.Bytes) == 0 && strings.TrimSpace(media.Path) == "" {
		return newProfileRequired(RoleMedia)
	}
	return nil
}

func profileRefEmpty(ref ProfileRef) bool {
	return ref.Descriptor == nil && len(ref.Bytes) == 0 && strings.TrimSpace(ref.Path) == ""
}
