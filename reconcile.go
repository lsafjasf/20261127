package main

import (
	"fmt"
	"math"
)

type pair struct {
	src int
	dst int
}

// matchMonotonic aligns src frames onto dst frames by visual signature.
// When manyToOne is true, several src frames may map to the same dst frame
// (record duplicates -> one captured frame). When false, indexes advance
// one-to-one so equally-repeated frames line up by order; extra src repeats
// may still map back to the previous dst index.
func matchMonotonic(src, dst *traceAna, dupBits int, manyToOne bool) []pair {
	var pairs []pair
	sf := src.trace.Frames
	df := dst.trace.Frames
	cur := 0
	last := -1
	for i := range sf {
		if !sf[i].Sig.Valid() {
			continue
		}
		found := -1
		if manyToOne && last >= 0 && SigDistance(sf[i].Sig, df[last].Sig) == 0 {
			found = last
		}
		start := cur
		if manyToOne {
			start = last + 1
		}
		if found < 0 {
			for k := start; k < len(df); k++ {
				if d := SigDistance(sf[i].Sig, df[k].Sig); d == 0 {
					found = k
					break
				}
			}
		}
		if found < 0 {
			for k := start; k < len(df); k++ {
				if SigSame(sf[i].Sig, df[k].Sig, dupBits) {
					found = k
					break
				}
			}
		}
		if found < 0 && !manyToOne && last >= 0 && SigSame(sf[i].Sig, df[last].Sig, dupBits) {
			found = last // extra repeated presentation of the previous dst frame
		}
		if found >= 0 {
			pairs = append(pairs, pair{i, found})
			last = found
			if manyToOne {
				// cursor stays; same dst may absorb more src copies
			} else if found >= cur {
				cur = found + 1
			}
		}
	}
	return pairs
}

func reconcile(src Sources, anas map[Stage]*traceAna, opt Options, b *bag) {
	if rec, ok := anas[StageRecord]; ok {
		if capA, ok := anas[StageCapture]; ok {
			reconcileCaptureRecord(capA, rec, opt, b)
		}
		if pl, ok := anas[StagePlayer]; ok {
			reconcileRecordPlayer(rec, pl, opt, b)
		}
	}
}

// reconcileCaptureRecord confirms capture loss and encoded duplicates.
func reconcileCaptureRecord(capA, rec *traceAna, opt Options, b *bag) {
	pairs := matchMonotonic(rec, capA, opt.DupBits, true)

	// Median raw-time offset: recTime ~= capTime + offset.
	var offs []float64
	for _, p := range pairs {
		offs = append(offs, rec.times[p.src]-capA.times[p.dst])
	}
	offset := median(offs)

	// Many-to-one runs: several record frames map to one capture frame.
	for i := 0; i < len(pairs); {
		j := i + 1
		for j < len(pairs) && pairs[j].dst == pairs[i].dst {
			j++
		}
		if j-i >= 2 {
			firstCopyRecIdx := pairs[i+1].src
			key := fmt.Sprintf("dup:%s:idx:%d", StageRecord, firstCopyRecIdx)
			capFrame := capA.trace.Frames[pairs[i].dst]
			ev := map[string]any{
				"mapped_capture_seq": seqOrIndex(capFrame, pairs[i].dst),
				"record_copies":      j - i,
			}
			if f := b.get(key); f != nil {
				f.Kind = KindEncodeDuplicate
				f.Severity = "confirmed"
				f.Confidence = 0.98
				f.Stage = StageRecord
				f.Anomaly = true
				for k, v := range ev {
					f.Evidence[k] = v
				}
			} else {
				t := rec.rel[firstCopyRecIdx]
				b.add(Finding{
					Key: key, Kind: KindEncodeDuplicate, Stage: StageRecord,
					Severity: "confirmed", Confidence: 0.98, Anomaly: true,
					Window: rec.frameWin[firstCopyRecIdx], TimeS: round3(t),
					FrameIndex: firstCopyRecIdx, Frames: j - i - 1,
					Seq:      rec.trace.Frames[firstCopyRecIdx].Seq,
					HasSeq:   rec.trace.Frames[firstCopyRecIdx].HasSeq,
					Evidence: ev,
				})
			}
		}
		i = j
	}

	// Upgrade capture deficit findings with mapped record PTS / merge record loss.
	for _, w := range capA.wins {
		capKey := deficitKey(StageCapture, w.Index)
		cf := b.get(capKey)
		if cf == nil {
			continue
		}
		capWinRawStart := capA.trace.AxisOrigin + w.Start
		recRaw := capWinRawStart + offset
		recPTS := recRaw - rec.trace.AxisOrigin
		cf.PTS = floatPtr(round3(recPTS))
		cf.rank = recPTS
		cf.hasRank = true

		rw := int(math.Floor(recPTS/opt.WindowSec + 1e-9))
		recKey := deficitKey(StageRecord, rw)
		merged := b.mergeInto(capKey, recKey)
		cf = b.get(capKey)
		if cf != nil {
			if w.MarkedDropped > 0 || merged {
				cf.Severity = "confirmed"
				cf.Confidence = 0.97
			}
			if cf.Evidence == nil {
				cf.Evidence = map[string]any{}
			}
			cf.Evidence["mapped_record_pts"] = round3(recPTS)
		}
	}
}

func seqOrIndex(f *Frame, idx int) any {
	if f.HasSeq {
		return f.Seq
	}
	return idx
}

// reconcileRecordPlayer proves player-side skips against a healthy recording.
func reconcileRecordPlayer(rec, pl *traceAna, opt Options, b *bag) {
	pairs := matchMonotonic(pl, rec, opt.DupBits, false)
	trustworthy := len(pairs) >= maxInt(3, len(pl.trace.Frames)/2)
	for i := 0; i+1 < len(pairs); i++ {
		a, c := pairs[i], pairs[i+1]
		if c.dst <= a.dst+1 {
			continue
		}
		firstSkipped := a.dst + 1
		nSkipped := c.dst - a.dst - 1
		if firstSkipped >= len(rec.trace.Frames) {
			continue
		}
		skippedSeqs := []int64{}
		for k := firstSkipped; k < firstSkipped+nSkipped && k < len(rec.trace.Frames); k++ {
			if fr := rec.trace.Frames[k]; fr.HasSeq && len(skippedSeqs) < 12 {
				skippedSeqs = append(skippedSeqs, fr.Seq)
			}
		}
		pts := round3(rec.rel[firstSkipped])
		playerWin := pl.frameWin[c.src]
		key := deficitKey(StagePlayer, playerWin)
		ev := map[string]any{
			"skipped_record_frames": nSkipped,
			"first_skipped_index":   firstSkipped,
			"skipped_seq_sample":    skippedSeqs,
			"resume_at_record_idx":  c.dst,
		}
		ex := b.add(Finding{
			Key: key, Kind: KindPlayerSkip, Stage: StagePlayer,
			Severity: "confirmed", Confidence: 0.97, Anomaly: true,
			Window: playerWin, TimeS: pts, PTS: floatPtr(pts),
			rank: pts, hasRank: true, FrameIndex: firstSkipped,
			Frames: nSkipped, Evidence: ev,
		})
		rf := rec.trace.Frames[firstSkipped]
		if rf.HasSeq {
			ex.Seq, ex.HasSeq = rf.Seq, true
		}
		ex.PTS = floatPtr(pts)
		ex.TimeS = pts
		ex.rank, ex.hasRank = pts, true
		ex.FrameIndex = firstSkipped
	}
	// Remaining player shortage findings are downstream of the recording
	// itself when alignment is trustworthy but proved no record-index gap.
	if trustworthy {
		for _, w := range pl.wins {
			key := deficitKey(StagePlayer, w.Index)
			f := b.get(key)
			if f == nil || f.Kind != KindPlayerShort {
				continue
			}
			nearRecDeficit := false
			for rw := w.Index - 1; rw <= w.Index+1; rw++ {
				if rw >= 0 && rw < len(rec.wins) &&
					containsReason(rec.wins[rw].Reasons, "frame_loss") {
					nearRecDeficit = true
				}
			}
			f.Severity = "info"
			f.Anomaly = false
			f.Confidence = 0.8
			f.Kind = KindDownstreamShort
			f.Title = kindTitle[KindDownstreamShort]
			if f.Evidence == nil {
				f.Evidence = map[string]any{}
			}
			if nearRecDeficit {
				f.Evidence["attribution"] = "downstream_of_upstream_loss"
			} else {
				f.Evidence["attribution"] = "aligned_no_gap"
			}
		}
	}
}

func containsReason(rs []string, want string) bool {
	for _, r := range rs {
		if r == want {
			return true
		}
	}
	return false
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
