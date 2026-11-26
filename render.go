package laddercheck

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// PrettyJSON indents v without HTML-escaping so commands like "<rungs...>"
// stay readable in emitted reports.
func PrettyJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

func rs(r Rat) string { return r.String() + "s" }

func rm(r Rat) string {
	v := r.Float64() * 1000
	if v < 0 {
		v = -v
	}
	return fmt.Sprintf("%.3fms", v)
}

func rsm(r Rat) string { return rs(r) + " ≈ " + rm(r) }

func dash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}

func idrCounts(f RenditionFacts) (idr, nonIDR, unknown int) {
	for _, k := range f.Keyframes {
		switch {
		case k.IDR == nil:
			unknown++
		case *k.IDR:
			idr++
		default:
			nonIDR++
		}
	}
	return
}

func elstSummary(f RenditionFacts) string {
	if !f.EditListPresent || len(f.Edits) == 0 {
		return "none"
	}
	parts := make([]string, 0, len(f.Edits))
	for _, e := range f.Edits {
		if e.Empty {
			parts = append(parts, "empty("+rs(e.SegmentDuration)+")")
			continue
		}
		mt := "?"
		if e.MediaTime != nil {
			mt = rs(*e.MediaTime)
		}
		parts = append(parts, "seg "+rs(e.SegmentDuration)+" @media "+mt)
	}
	return fmt.Sprintf("%d: %s", len(f.Edits), strings.Join(parts, " + "))
}

func gopSummary(f RenditionFacts) string {
	if len(f.GOPMediaDurations) == 0 {
		return "n/a"
	}
	seen := map[string]bool{}
	var vals []string
	for _, g := range f.GOPMediaDurations {
		if !seen[g.String()] {
			seen[g.String()] = true
			vals = append(vals, rs(g))
		}
	}
	uniform := "non-uniform"
	if f.GOPUniform {
		uniform = "uniform"
	}
	return strings.Join(vals, ",") + " (" + uniform + ")"
}

func statusCN(st string) string {
	switch st {
	case StatusCompliant:
		return "compliant 已对齐"
	case StatusRemuxOnly:
		return "remux-only 仅需改封装/编辑列表"
	case StatusReencode:
		return "re-encode 必须重编码"
	case StatusInsufficient:
		return "insufficient 证据不足"
	}
	return st
}

// RenderText renders a concise console summary.
func RenderText(rep *Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "laddercheck %s  input=%s  rungs=%d\n", rep.Schema, dash(rep.Input), rep.Summary.Renditions)
	c := rep.Constraint
	fmt.Fprintf(&b, "unified grid: origin=%s gop=%s gop_frames=%d fps=%s tol=%s require_idr=%v\n",
		rs(c.OriginSeconds), rs(c.GOPSeconds), c.GOPFrames, c.FPS.String(), rs(c.ToleranceSeconds), c.RequireIDR)
	fmt.Fprintln(&b, strings.Repeat("=", 100))
	fmt.Fprintf(&b, "%-16s %-24s %-10s %-12s %-14s %s\n", "RUNG", "STATUS", "FPS", "FIRST-DISP", "MAX-ERR", "LABEL-OFFSET")
	for _, v := range rep.Verdicts {
		off := "—"
		if v.LabelOffsetSeconds != nil {
			off = rs(*v.LabelOffsetSeconds)
		}
		fmt.Fprintf(&b, "%-16s %-24s %-10s %-12s %-14s %s\n",
			trunc(v.ID, 16), trunc(statusCN(v.Status), 24), c.FPS.String(),
			rs(v.FirstDisplayPTS), rs(v.MaxDisplayError), off)
	}
	fmt.Fprintln(&b, strings.Repeat("=", 100))
	fmt.Fprintf(&b, "compliant=%v\n", rep.Summary.Compliant)
	fmt.Fprintf(&b, "remux_only_fixable=%v\n", rep.Summary.RemuxOnly)
	fmt.Fprintf(&b, "requires_reencode=%v\n", rep.Summary.RequiresReencode)
	if len(rep.Summary.Insufficient) > 0 {
		fmt.Fprintf(&b, "insufficient=%v\n", rep.Summary.Insufficient)
	}
	fmt.Fprintf(&b, "switch_safe_now=%v  alignable_without_reencode=%v\n",
		rep.Summary.SwitchSafe, rep.Summary.AlignableWithoutReencode)

	for _, v := range rep.Verdicts {
		if v.Status == StatusCompliant {
			continue
		}
		fmt.Fprintf(&b, "\n## %s -> %s\n", v.ID, statusCN(v.Status))
		for _, r := range v.Reasons {
			fmt.Fprintf(&b, "  reason: %s\n", r)
		}
		for _, a := range v.Actions {
			fmt.Fprintf(&b, "  action: %s\n", a)
		}
	}
	return b.String()
}

func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
