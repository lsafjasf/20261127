package hdrswitch

import "fmt"

type AUMetadataUpdate struct {
	AUIndex  int64         `json:"au_index"`
	PTS      Rational      `json:"pts"`
	Action   string        `json:"action"`
	Metadata ColorMetadata `json:"metadata"`
}

type RepairPlan struct {
	Mode                  string             `json:"mode"`
	SafeToRetag           bool               `json:"safe_to_retag"`
	ExactBoundaryRequired bool               `json:"exact_boundary_required"`
	PreviousRandomAccess  *Boundary          `json:"previous_random_access,omitempty"`
	NextRandomAccess      *Boundary          `json:"next_random_access,omitempty"`
	MetadataUpdates       []AUMetadataUpdate `json:"metadata_updates"`
	Actions               []string           `json:"actions"`
	RejectedShortcut      string             `json:"rejected_shortcut"`
}

// PlanRepair turns the report into an AU/GOP remediation plan. It never
// represents a container-wide metadata label change as a sufficient fix.
func PlanRepair(evidence Evidence) (*RepairPlan, *Report, error) {
	report, err := Analyze(evidence)
	if err != nil {
		return nil, nil, err
	}
	if report.Before == nil || report.After == nil || report.CutAUIndex == nil {
		return nil, report, fmt.Errorf("report is missing cut segments")
	}

	cutPosition := -1
	for i := range report.AccessUnits {
		if report.AccessUnits[i].IsCutAU {
			cutPosition = i
			break
		}
	}
	if cutPosition < 0 {
		return nil, report, fmt.Errorf("cut AU state not found")
	}
	cutState := report.AccessUnits[cutPosition]
	risk := report.GOPRisk
	beforePTSPosition := positionByIndex(report, report.Before.LastAUIndex)
	if beforePTSPosition < 0 {
		return nil, report, fmt.Errorf("before-segment AU %d not found in report", report.Before.LastAUIndex)
	}
	beforePTS := report.AccessUnits[beforePTSPosition].PTS

	plan := &RepairPlan{
		Mode:                  "rewrite_au_local_metadata_at_safe_boundary",
		SafeToRetag:           risk != nil && risk.Safe,
		ExactBoundaryRequired: risk != nil && risk.AtRisk,
		RejectedShortcut:      "changing only a global/container HLG/PQ label or first-frame metadata",
		MetadataUpdates: []AUMetadataUpdate{
			{
				AUIndex:  report.Before.LastAUIndex,
				PTS:      beforePTS,
				Action:   "last AU that must remain on pre-switch transfer/peak/gamut",
				Metadata: report.Before.BoundaryMetadata,
			},
			{
				AUIndex:  *report.CutAUIndex,
				PTS:      cutState.PTS,
				Action:   "first AU that must carry/activate post-switch SPS/VUI and HDR SEI",
				Metadata: report.After.BoundaryMetadata,
			},
		},
	}

	if risk != nil {
		plan.PreviousRandomAccess = risk.PreviousRandomAccess
		plan.NextRandomAccess = risk.NextRandomAccess
	}

	if plan.SafeToRetag {
		plan.Actions = []string{
			"Keep the pre-switch AU metadata unchanged through AU " + fmt.Sprint(report.Before.LastAUIndex) + ".",
			"At IDR/CVS AU " + fmt.Sprint(*report.CutAUIndex) + ", write/activate new SPS/VUI transfer and primaries, PPS, mastering display SEI, and content light level SEI.",
			"Reset player color/metadata state when the new coded video sequence starts; do not inherit the first-frame HLG label.",
			"Decode and probe every AU around the splice to verify transfer=" + report.After.LegalRange.Transfer + ", peak and gamut.",
		}
		return plan, report, nil
	}

	plan.Mode = "fix_gop_before_metadata_retag"
	plan.Actions = []string{
		"Stop at the GOP risk report; do not retag the global stream first.",
		"Re-encode the affected GOP so an IDR or verified closed-GOP CVS starts exactly at " + report.SwitchAt.SecondsText(3) + ".",
		"Remove cross-boundary references/RASL dependencies or explicitly validate open-GOP splice behavior.",
		"Insert new SPS/VUI/PPS and repeat HDR static/dynamic metadata at the new IDR.",
		"Alternatively move the creative switch to PreviousRandomAccess or NextRandomAccess and update the edit list; this changes the switch time.",
		"Only after the bitstream boundary is safe should players reset their per-CVS/per-AU metadata.",
	}
	return plan, report, nil
}

func positionByIndex(report *Report, index int64) int {
	for i, state := range report.AccessUnits {
		if state.Index == index {
			return i
		}
	}
	return -1
}

func RenderRepairPlan(plan *RepairPlan) string {
	var out string
	out += "\n修复计划: " + plan.Mode + "\n"
	out += fmt.Sprintf("  safe_to_retag=%v exact_boundary_required=%v\n", plan.SafeToRetag, plan.ExactBoundaryRequired)
	out += "  拒绝的捷径: " + plan.RejectedShortcut + "\n"
	out += "  动作:\n"
	for _, action := range plan.Actions {
		out += "    - " + action + "\n"
	}
	out += "  AU 元数据点:\n"
	for _, update := range plan.MetadataUpdates {
		out += fmt.Sprintf("    - AU %d @ %s: %s [%s/%s]\n",
			update.AUIndex, update.PTS.SecondsText(6), update.Action,
			update.Metadata.Transfer, update.Metadata.Primaries)
	}
	return out
}
