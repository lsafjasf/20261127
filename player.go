package hdrswitch

import "fmt"

func applyPlayerSticky(evidence Evidence, metas []ColorMetadata, states []AUState, report *Report) {
	checkEnabled := policyTrue(evidence.Policy.CheckStickyPlayer)
	enabled := checkEnabled && (evidence.Player.UsesFirstFrameMetadata || evidence.Player.StickyMetadata != nil)
	if !enabled {
		return
	}

	sticky := metas[0]
	if evidence.Player.StickyMetadata != nil {
		sticky = mergeMetadata(metas[0], evidence.Player.StickyMetadata)
	}

	playerReport := &PlayerReport{
		StickyFirstFrame: evidence.Player.UsesFirstFrameMetadata || evidence.Player.StickyMetadata == nil,
		StickyMetadata:   &sticky,
	}

	firstMismatch := int64(-1)
	for i := range states {
		fields := stickyMismatchFields(sticky, metas[i])
		states[i].PlayerStale = len(fields) > 0
		states[i].StaleFields = fields
		if len(fields) == 0 {
			continue
		}
		playerReport.MismatchAUCount++
		if firstMismatch == -1 {
			firstMismatch = states[i].Index
			playerReport.FirstMismatchAU = &firstMismatch
			playerReport.StaleFields = fields
		}
	}

	report.Player = playerReport
	report.AccessUnits = states

	if playerReport.MismatchAUCount > 0 {
		report.Findings = append(report.Findings, Finding{
			Code:     "player_sticky_first_frame_metadata",
			Severity: SeverityError,
			AUIndex:  playerReport.FirstMismatchAU,
			Message: fmt.Sprintf(
				"player/global label remains %s/%s while %d AU(s) differ; first stale AU=%d, fields=%v",
				sticky.Transfer, sticky.Primaries, playerReport.MismatchAUCount,
				firstMismatch, playerReport.StaleFields,
			),
		})
		report.OK = false
	} else {
		report.Findings = append(report.Findings, Finding{
			Code:     "player_metadata_tracks_au",
			Severity: SeverityInfo,
			Message:  "player metadata matches every resolved AU; no first-frame sticky mismatch detected",
		})
	}
}
