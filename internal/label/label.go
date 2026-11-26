package label

import (
	"colorfix/internal/color"
	"colorfix/internal/video"
)

type ColorLabel struct {
	MatrixName     string `json:"matrix"`
	Matrix         string `json:"matrix_family"`
	Label          string `json:"label"`
	ColorPrimaries string `json:"color_primaries,omitempty"`
	ColorTransfer  string `json:"color_transfer,omitempty"`
	ColorRange     string `json:"color_range,omitempty"`
	MatrixCode     int    `json:"matrix_coefficients"`
	PrimariesCode  int    `json:"colour_primaries,omitempty"`
	TransferCode   int    `json:"transfer_characteristics,omitempty"`
	Scope          string `json:"scope"`
}

type LabelSet struct {
	DeclaredName   string       `json:"declared_matrix,omitempty"`
	DeclaredSource string       `json:"declared_source,omitempty"`
	Recommended    *ColorLabel  `json:"recommended,omitempty"`
	Candidates     []ColorLabel `json:"candidates,omitempty"`
	Note           string       `json:"note"`
}

func Build(probe video.Probe, selected color.Matrix, candidates []color.Matrix) LabelSet {
	set := LabelSet{
		DeclaredName: probe.MatrixName, DeclaredSource: probe.MatrixSource,
		Note: "Only the matrix-coefficients declaration is changed. Primaries, transfer and range from the source are retained unless explicitly listed.",
	}
	if selected != "" {
		l := forMatrix(probe, selected)
		set.Recommended = &l
	}
	for _, m := range candidates {
		set.Candidates = append(set.Candidates, forMatrix(probe, m))
	}
	return set
}

func forMatrix(probe video.Probe, matrix color.Matrix) ColorLabel {
	primaries := probe.Stream.ColorPrimaries
	transfer := probe.Stream.ColorTransfer
	matrixName := matrix.FFprobeName()
	matrixCode := 5
	primariesCode := codeForName(primaries)
	transferCode := codeForName(transfer)
	if matrix == color.BT709 {
		matrixCode = 1
	}
	return ColorLabel{
		MatrixName: matrixName, Matrix: string(matrix), Label: matrix.Label(),
		ColorPrimaries: primaries, ColorTransfer: transfer,
		ColorRange: string(probe.Range), MatrixCode: matrixCode,
		PrimariesCode: primariesCode, TransferCode: transferCode,
		Scope: "matrix_coefficients only; other colour descriptions are preserved",
	}
}

func codeForName(name string) int {
	switch name {
	case "bt709":
		return 1
	case "bt470bg":
		return 5
	case "smpte170m":
		return 6
	default:
		return 0
	}
}
