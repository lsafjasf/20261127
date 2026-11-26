package analyze

type Options struct {
	CeilingThreshold  float64
	FlatTolerance     float64
	MinPlateauRun     int
	MergeGapSamples   int
	FFTSize           int
	WaveBefore        int
	WaveAfter         int
	MinConfidence     float64
	CodecFrameSamples int
}

func DefaultOptions(sampleRate int) Options {
	return Options{
		CeilingThreshold: 0.98,
		FlatTolerance:    0.00012,
		MinPlateauRun:    2,
		MergeGapSamples:  sampleRate * 30 / 1000,
		FFTSize:          2048,
		WaveBefore:       24,
		WaveAfter:        48,
		MinConfidence:    0.55,
	}
}

func NormalizeOptions(opts Options, sampleRate int) Options {
	d := DefaultOptions(sampleRate)
	if opts.CeilingThreshold == 0 {
		opts.CeilingThreshold = d.CeilingThreshold
	}
	if opts.FlatTolerance == 0 {
		opts.FlatTolerance = d.FlatTolerance
	}
	if opts.MinPlateauRun == 0 {
		opts.MinPlateauRun = d.MinPlateauRun
	}
	if opts.MergeGapSamples == 0 {
		opts.MergeGapSamples = d.MergeGapSamples
	}
	if opts.FFTSize == 0 {
		opts.FFTSize = d.FFTSize
	}
	if opts.WaveBefore == 0 {
		opts.WaveBefore = d.WaveBefore
	}
	if opts.WaveAfter == 0 {
		opts.WaveAfter = d.WaveAfter
	}
	if opts.MinConfidence == 0 {
		opts.MinConfidence = d.MinConfidence
	}
	return opts
}

type Report struct {
	SchemaVersion string       `json:"schema_version"`
	Tool          string       `json:"tool"`
	Files         []FileReport `json:"files"`
	Verdict       string       `json:"verdict"`
	Summary       Summary      `json:"summary"`
	Notes         []string     `json:"notes,omitempty"`
}

type Summary struct {
	FilesAnalyzed       int `json:"files_analyzed"`
	IncidentCandidates  int `json:"incident_candidates"`
	ArtisticRegions     int `json:"artistic_regions"`
	InconclusiveRegions int `json:"inconclusive_regions"`
}

type FileReport struct {
	File        string        `json:"file"`
	Label       string        `json:"label,omitempty"`
	SampleRate  int           `json:"sample_rate"`
	Channels    int           `json:"channels"`
	Frames      int           `json:"frames"`
	DurationSec float64       `json:"duration_sec"`
	Global      GlobalMetrics `json:"global_metrics"`
	Candidates  []Candidate   `json:"candidates"`
	Artistic    []Candidate   `json:"artistic_distortion"`
	Verdict     string        `json:"verdict"`
}

type GlobalMetrics struct {
	PeakAmplitude         float64 `json:"peak_amplitude"`
	PeakDBFS              float64 `json:"peak_dbfs"`
	DCOffset              float64 `json:"dc_offset"`
	ExactFullScaleSamples int     `json:"exact_full_scale_samples"`
	NearCeilingSamples    int     `json:"near_ceiling_samples"`
	PlateauRunCount       int     `json:"plateau_run_count"`
	PlateauSamples        int     `json:"plateau_samples"`
	LongestPlateauRun     int     `json:"longest_plateau_run"`
}

type Candidate struct {
	ID             string              `json:"id"`
	Status         string              `json:"status"`
	Incident       bool                `json:"incident"`
	Classification string              `json:"classification"`
	Attribution    string              `json:"attribution"`
	StartSample    int                 `json:"start_sample"`
	EndSample      int                 `json:"end_sample"`
	StartSec       float64             `json:"start_sec"`
	EndSec         float64             `json:"end_sec"`
	PeakSample     int                 `json:"peak_sample"`
	EvidenceSample int                 `json:"evidence_sample"`
	Channel        int                 `json:"primary_channel"`
	Channels       []int               `json:"channels"`
	Confidence     float64             `json:"confidence"`
	Severity       string              `json:"severity"`
	Reasons        []string            `json:"reasons"`
	Plateau        PlateauMetrics      `json:"plateau"`
	Edge           EdgeMetrics         `json:"edge"`
	Spectrum       SpectrumMetrics     `json:"spectrum"`
	Waveform       WaveformEvidence    `json:"waveform"`
	CodecGrid      *GridEvidence       `json:"codec_grid,omitempty"`
	Comparison     *ComparisonEvidence `json:"comparison,omitempty"`
}

type PlateauMetrics struct {
	RunCount              int     `json:"run_count"`
	PlateauSamples        int     `json:"plateau_samples"`
	LongestRun            int     `json:"longest_run"`
	ExactFullScaleSamples int     `json:"exact_full_scale_samples"`
	NearCeilingSamples    int     `json:"near_ceiling_samples"`
	Density               float64 `json:"density_in_region"`
	MeanRun               float64 `json:"mean_run_samples"`
}

type EdgeMetrics struct {
	MaxEntryJump        float64 `json:"max_entry_jump"`
	MaxExitJump         float64 `json:"max_exit_jump"`
	MaxSlope            float64 `json:"max_abs_slope_per_sample"`
	MedianRiseSamples   float64 `json:"median_10_90_rise_samples"`
	MedianCornerSamples float64 `json:"median_90_to_plateau_samples"`
	SteepEdgeRatio      float64 `json:"steep_edge_ratio"`
	OvershootAmplitude  float64 `json:"edge_overshoot_amplitude"`
}

type SpectrumMetrics struct {
	FFTSize          int     `json:"fft_size"`
	CentroidHz       float64 `json:"centroid_hz"`
	Flatness         float64 `json:"spectral_flatness"`
	Rolloff85Hz      float64 `json:"rolloff_85_hz"`
	LowRatio         float64 `json:"low_20_250_ratio"`
	MidRatio         float64 `json:"mid_250_4000_ratio"`
	HighRatio        float64 `json:"high_4k_12k_ratio"`
	AirRatio         float64 `json:"air_12k_plus_ratio"`
	SplatterDeltaDB  float64 `json:"splatter_2k_12k_delta_db_vs_neighbor"`
	HarmonicStrength float64 `json:"harmonic_strength"`
	EstimatedF0Hz    float64 `json:"estimated_f0_hz"`
	OddEvenRatio     float64 `json:"odd_even_energy_ratio"`
}

type WaveformEvidence struct {
	FirstSample    int         `json:"first_sample"`
	LastSample     int         `json:"last_sample"`
	EvidenceSample int         `json:"evidence_sample"`
	SampleRate     int         `json:"sample_rate"`
	Channels       [][]float64 `json:"channels"`
	PlateauMask    [][]bool    `json:"plateau_mask"`
}

type GridEvidence struct {
	FrameSamples   int     `json:"frame_samples"`
	AlignedCount   int     `json:"aligned_count"`
	CandidateCount int     `json:"candidate_count"`
	AlignmentRatio float64 `json:"alignment_ratio"`
}

type ComparisonEvidence struct {
	ReferenceFile        string  `json:"reference_file"`
	AlignedLagSamples    int     `json:"aligned_lag_samples"`
	ReferencePeak        float64 `json:"reference_peak"`
	ReferencePlateauRuns int     `json:"reference_plateau_runs"`
	ReferenceHadPlateau  bool    `json:"reference_had_plateau"`
	NewPlateauRuns       int     `json:"new_plateau_runs"`
	PeakDifferenceDB     float64 `json:"peak_difference_db"`
	Verdict              string  `json:"verdict"`
}
