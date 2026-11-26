package ripple

import (
	"fmt"
	"sort"
	"strings"
)

type SyncIssue struct {
	LinkGroup string   `json:"linkGroup"`
	Tracks    []string `json:"tracks"`
	Message   string   `json:"message"`
}

type clipSignature struct {
	Start    Tick `json:"start"`
	Duration Tick `json:"duration"`
}

// CheckLinkedSync compares fragments that originated from the same linked
// A/V group. A track-local ripple is therefore visible as a sync error rather
// than being silently accepted.
func CheckLinkedSync(tl Timeline) []SyncIssue {
	type groupData struct {
		tracks map[string][]clipSignature
	}

	groups := make(map[string]*groupData)
	for _, clip := range tl.Clips {
		if clip.LinkGroup == "" {
			continue
		}
		group := groups[clip.LinkGroup]
		if group == nil {
			group = &groupData{tracks: make(map[string][]clipSignature)}
			groups[clip.LinkGroup] = group
		}
		group.tracks[clip.TrackID] = append(group.tracks[clip.TrackID], clipSignature{
			Start: clip.Start, Duration: clip.Duration,
		})
	}

	issues := make([]SyncIssue, 0)
	for groupID, group := range groups {
		trackIDs := make([]string, 0, len(group.tracks))
		for trackID := range group.tracks {
			trackIDs = append(trackIDs, trackID)
		}
		sort.Strings(trackIDs)
		if len(trackIDs) < 2 {
			continue
		}

		baseTrack := trackIDs[0]
		base := normalizedSignatures(group.tracks[baseTrack])
		for _, trackID := range trackIDs[1:] {
			current := normalizedSignatures(group.tracks[trackID])
			if signaturesEqual(base, current) {
				continue
			}
			issues = append(issues, SyncIssue{
				LinkGroup: groupID,
				Tracks:    []string{baseTrack, trackID},
				Message: fmt.Sprintf("linked fragments differ: %s=%s vs %s=%s",
					baseTrack, formatSignatures(base), trackID, formatSignatures(current)),
			})
		}
	}

	sort.Slice(issues, func(i, j int) bool { return issues[i].LinkGroup < issues[j].LinkGroup })
	return issues
}

func normalizedSignatures(input []clipSignature) []clipSignature {
	out := append([]clipSignature(nil), input...)
	sort.Slice(out, func(i, j int) bool { return out[i].Start < out[j].Start })
	return out
}

func signaturesEqual(a, b []clipSignature) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func formatSignatures(signatures []clipSignature) string {
	parts := make([]string, 0, len(signatures))
	for _, signature := range signatures {
		parts = append(parts, fmt.Sprintf("@%d+%d", signature.Start, signature.Duration))
	}
	return "[" + strings.Join(parts, ",") + "]"
}
