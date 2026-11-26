package tailtrace

import "fmt"

// FixPlan is an actionable remediation. FixedEvidence is the target evidence
// after trimming; RevisedContract is the alternative contractual resolution.
type FixPlan struct {
	Mode            string    `json:"mode"`
	Action          string    `json:"action"`
	RemoveIndices   []int64   `json:"remove_indices,omitempty"`
	FixedEvidence   Evidence  `json:"fixed_evidence,omitempty"`
	RevisedContract *Contract `json:"revised_contract,omitempty"`
	Explanation     string    `json:"explanation"`
}

// PlanTrim removes renderer-generated tail padding after the declared content
// boundary. It refuses to remove unexplained non-black frames.
func PlanTrim(evidence Evidence) (*FixPlan, error) {
	if _, err := Analyze(evidence); err != nil {
		return nil, err
	}
	contentFrames := evidence.Contract.ContentFrames
	if contentFrames == 0 {
		contentFrames = int64(len(evidence.Source))
	}
	if contentFrames <= 0 {
		return nil, fmt.Errorf("cannot trim without contract.content_frames or source frames")
	}
	if contentFrames > int64(len(evidence.Output)) {
		return nil, fmt.Errorf("content boundary %d exceeds output frames %d", contentFrames, len(evidence.Output))
	}

	remove := evidence.Output[contentFrames:]
	indices := make([]int64, 0, len(remove))
	for _, frame := range remove {
		if !frame.Black && frame.Origin != "tail_pad" {
			return nil, fmt.Errorf("refusing to trim output frame %d: not marked black/tail_pad", frame.Index)
		}
		indices = append(indices, frame.Index)
	}

	fixed := evidence
	fixed.Output = append([]Frame(nil), evidence.Output[:contentFrames]...)
	fixedFrames := contentFrames
	frameDuration := Rational{Num: evidence.Contract.FPS.Den, Den: evidence.Contract.FPS.Num}.normalize()
	fixedDuration := MustRational(fixedFrames, 1).Mul(frameDuration)
	fixed.Contract.DeliveryFrames = &fixedFrames
	fixed.Contract.DeliveryDuration = &fixedDuration
	fixed.Contract.AllowedTailBlackFrames = 0
	fixed.Contract.TailPolicy = "no_tail_black_padding"
	fixed.Contract.ContainerDuration = &fixedDuration
	fixed.Contract.AudioDuration = &fixedDuration
	fixed.Contract.Player.Frames = nil
	fixed.Contract.Player.EditListEnd = nil
	fixed.Contract.Player.ReportedDuration = &fixedDuration

	return &FixPlan{
		Mode: "trim", Action: "remove_tail_black_samples_and_update_durations",
		RemoveIndices: indices, FixedEvidence: fixed,
		Explanation: fmt.Sprintf(
			"Remove output sample indices %v, preserve existing content PTS, and align delivery/container/audio/player duration to %s. Re-stamp sample durations during remux; deleting duration metadata alone is not a fix.",
			indices, fixedDuration.SecondsText(6),
		),
	}, nil
}

// PlanContract keeps the frames but makes the output contract explicit. This
// is the correct resolution when two black lead-out frames are intentional.
func PlanContract(evidence Evidence) (*FixPlan, error) {
	report, err := Analyze(evidence)
	if err != nil {
		return nil, err
	}
	tailFrames := report.Durations.TailBlackFrames
	if tailFrames == 0 {
		return nil, fmt.Errorf("no tail-black frames found to contract")
	}

	revised := evidence.Contract
	deliveryFrames := report.Durations.OutputFrames
	deliveryDuration := report.Durations.CFRDuration
	containerDuration := deliveryDuration
	revised.DeliveryFrames = &deliveryFrames
	revised.DeliveryDuration = &deliveryDuration
	revised.AllowedTailBlackFrames = tailFrames
	revised.TailPolicy = "explicit_marked_tail_black_lead_out"
	if revised.ContainerDuration == nil {
		revised.ContainerDuration = &containerDuration
	}

	return &FixPlan{
		Mode: "contract", Action: "recognize_tail_black_in_delivery_contract",
		RevisedContract: &revised,
		Explanation: fmt.Sprintf(
			"Contract for %d content frame(s) plus %d explicit black lead-out frame(s). Report content duration %s separately from delivery/container duration %s; players will show black for %s at the end.",
			report.Durations.ContentFrames, tailFrames,
			report.Durations.ContentDuration.SecondsText(6), deliveryDuration.SecondsText(6),
			report.Durations.TailBlackDuration.SecondsText(6),
		),
	}, nil
}
