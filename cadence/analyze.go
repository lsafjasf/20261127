package cadence

import (
	"fmt"
	"math"
)

type state struct {
	cadence Cadence
	order   FieldOrder
}

var cadenceStates = []state{
	{Cadence24p32, OrderTFF},
	{Cadence24p32, OrderBFF},
	{Cadence30p22, OrderTFF},
	{Cadence30p22, OrderBFF},
}

const unknownState = 4

// Analyze classifies cadence locally and segments on cadence/order changes.
// Invalid cadence evidence turns only the affected segment into "unknown"; it
// never changes a whole-file fallback.
func Analyze(media Media, opts Options) (Report, error) {
	if len(media.Frames) == 0 {
		return Report{}, analysisError("%w", ErrNoVideo)
	}
	if len(media.Frames) < 3 {
		return Report{}, analysisError("%w: %d frames", ErrShortVideo, len(media.Frames))
	}
	for i, frame := range media.Frames {
		if err := frame.Top.Validate(); err != nil {
			return Report{}, analysisError("frame %d top field: %w", i, err)
		}
		if err := frame.Bottom.Validate(); err != nil {
			return Report{}, analysisError("frame %d bottom field: %w", i, err)
		}
	}
	if media.FrameRate <= 0 {
		media.FrameRate = 30000.0 / 1001.0
	}
	opts = normalizeAnalyzeOptions(opts)
	tff, bff := makeSequences(media.Frames)
	sequences := map[FieldOrder]fieldSequence{
		OrderTFF: tff,
		OrderBFF: bff,
	}

	hint := media.FieldOrder
	if hint == "" {
		hint = HintUnknown
	}
	costs := scoreFrames(media.Frames, sequences, hint, opts)
	labels := viterbi(costs, opts.ChangePenalty)
	labels = suppressShortRuns(labels, opts.MinSegmentFrames, costs)

	cuts := detectCuts(media.Frames, opts.SceneCutThreshold)
	runs := basicRuns(labels)
	runs = collapseUnknownTransitions(runs, media.Frames, sequences, hint, opts)
	runs = recoverUnknownRuns(runs, media.Frames, sequences, hint, opts)
	runs = collapseUnknownTransitions(runs, media.Frames, sequences, hint, opts)
	runs = mergeWeakRuns(runs, media.Frames, sequences, hint, opts)
	runs = refineRuns(runs, media.Frames, sequences, hint, opts, cuts)
	runs = collapseUnknownTransitions(runs, media.Frames, sequences, hint, opts)
	if err := validateRuns(runs, len(media.Frames)); err != nil {
		return Report{}, err
	}

	report := Report{
		Version:        1,
		FrameRate:      math.Round(media.FrameRate*1000) / 1000,
		FieldRate:      math.Round(media.FrameRate*2000) / 1000,
		FrameCount:     len(media.Frames),
		FieldCount:     len(media.Frames) * 2,
		DurationSec:    round3(durationSec(len(media.Frames), media.FrameRate)),
		ContainerOrder: hint,
	}
	if len(media.Frames) < opts.WindowFrames*2 {
		report.Warnings = append(report.Warnings,
			fmt.Sprintf("short sample: %d frames, local evidence uses a reduced window", len(media.Frames)))
	}
	for _, run := range runs {
		segment := buildSegment(len(report.Segments), run.start, run.end, media, sequences, hint)
		report.Segments = append(report.Segments, segment)
	}
	return report, nil
}

func validateRuns(runs []frameRun, total int) error {
	if len(runs) == 0 {
		return analysisError("segmenter produced no runs")
	}
	if runs[0].start != 0 || runs[len(runs)-1].end != total {
		return analysisError("segments do not cover the video: %d..%d of %d",
			runs[0].start, runs[len(runs)-1].end, total)
	}
	for i, run := range runs {
		if run.end <= run.start {
			return analysisError("empty segment at frame %d", run.start)
		}
		if i > 0 && runs[i-1].end != run.start {
			return analysisError("segment gap or overlap at frame %d", run.start)
		}
	}
	return nil
}

func normalizeAnalyzeOptions(opts Options) Options {
	def := DefaultOptions()
	if opts.WindowFrames <= 0 {
		opts.WindowFrames = def.WindowFrames
	}
	if opts.WindowFrames < 6 {
		opts.WindowFrames = 6
	}
	if opts.MinSegmentFrames <= 0 {
		opts.MinSegmentFrames = def.MinSegmentFrames
	}
	if opts.MinSegmentFrames < 2 {
		opts.MinSegmentFrames = 2
	}
	if opts.ChangePenalty <= 0 {
		opts.ChangePenalty = def.ChangePenalty
	}
	if opts.SceneCutThreshold <= 0 {
		opts.SceneCutThreshold = def.SceneCutThreshold
	}
	return opts
}

func scoreFrames(frames []Frame, sequences map[FieldOrder]fieldSequence,
	hint FieldOrderHint, opts Options) [][]float64 {
	n := len(frames)
	costs := make([][]float64, n)
	for f := 0; f < n; f++ {
		start, end := frameWindow(f, n, opts.WindowFrames)
		scores := make([]windowScore, len(cadenceStates))
		for i, st := range cadenceStates {
			seq := sequences[st.order]
			window := seq.window(start*2, end*2)
			if st.cadence == Cadence24p32 {
				scores[i] = window.score24p(hint)
			} else {
				scores[i] = window.score30p(hint)
			}
		}
		applyOrderGate(scores, sequences[OrderTFF].window(start*2, end*2),
			sequences[OrderBFF].window(start*2, end*2), hint)
		best := 0.0
		bestIdx := 0
		for i, score := range scores {
			if score.score > best {
				best, bestIdx = score.score, i
			}
		}
		row := make([]float64, len(cadenceStates)+1)
		medianMotion := scores[bestIdx].medianMotion
		static := scores[bestIdx].staticRatio > 0.72 && medianMotion < defaultStillFloor*2
		for i, score := range scores {
			row[i] = 1 - score.score
			if static {
				// Carry the surrounding decision through low-information stills.
				row[i] = 0.50
			}
		}
		row[unknownState] = unknownCost(best, static)
		costs[f] = row
	}
	return costs
}

func frameWindow(frame, total, width int) (int, int) {
	if width >= total {
		return 0, total
	}
	half := width / 2
	start := frame - half
	end := start + width
	if start < 0 {
		start = 0
		end = width
	}
	if end > total {
		end = total
		start = total - width
	}
	return start, end
}

func unknownCost(best float64, static bool) float64 {
	if static {
		return 0.72
	}
	switch {
	case best < 0.30:
		return 0.20
	case best < 0.48:
		return 0.42
	case best < 0.62:
		return 0.78
	default:
		return 1.40
	}
}

func viterbi(costs [][]float64, changePenalty float64) []int {
	n := len(costs)
	if n == 0 {
		return nil
	}
	states := len(costs[0])
	dp := make([][]float64, n)
	back := make([][]int, n)
	for i := range dp {
		dp[i] = make([]float64, states)
		back[i] = make([]int, states)
	}
	copy(dp[0], costs[0])
	for f := 1; f < n; f++ {
		for dst := 0; dst < states; dst++ {
			bestCost := math.Inf(1)
			bestPrev := 0
			for src := 0; src < states; src++ {
				cost := dp[f-1][src] + transitionCost(src, dst, changePenalty) + costs[f][dst]
				if cost < bestCost {
					bestCost = cost
					bestPrev = src
				}
			}
			dp[f][dst] = bestCost
			back[f][dst] = bestPrev
		}
	}
	last := 0
	for i := 1; i < states; i++ {
		if dp[n-1][i] < dp[n-1][last] {
			last = i
		}
	}
	out := make([]int, n)
	out[n-1] = last
	for f := n - 1; f > 0; f-- {
		out[f-1] = back[f][out[f]]
	}
	return out
}

func transitionCost(from, to int, changePenalty float64) float64 {
	if from == to {
		return 0
	}
	if from == unknownState || to == unknownState {
		return changePenalty * 0.45
	}
	fromState := cadenceStates[from]
	toState := cadenceStates[to]
	if fromState.cadence == toState.cadence {
		// Field parity is a property of the source, not a per-scene choice.
		// Keep order flips expensive unless cadence itself changes.
		return changePenalty * 1.8
	}
	return changePenalty
}

func suppressShortRuns(labels []int, minFrames int, costs [][]float64) []int {
	out := append([]int(nil), labels...)
	for pass := 0; pass < 8; pass++ {
		runs := basicRuns(out)
		changed := false
		for _, run := range runs {
			if run.end-run.start >= minFrames || len(runs) == 1 {
				continue
			}
			replacement := chooseNeighborState(run, runs, out, costs)
			for i := run.start; i < run.end; i++ {
				out[i] = replacement
			}
			changed = true
			break
		}
		if !changed {
			break
		}
	}
	return out
}

func chooseNeighborState(run frameRun, runs []frameRun, labels []int, costs [][]float64) int {
	if len(runs) == 1 {
		return labels[run.start]
	}
	var candidates []int
	if run.start > 0 {
		candidates = append(candidates, labels[run.start-1])
	}
	if run.end < len(labels) {
		candidates = append(candidates, labels[run.end])
	}
	if len(candidates) == 0 {
		return labels[run.start]
	}
	best := candidates[0]
	bestCost := math.Inf(1)
	for _, candidate := range candidates {
		if candidate == unknownState && len(candidates) > 1 {
			continue
		}
		var sum float64
		for i := run.start; i < run.end; i++ {
			sum += costs[i][candidate]
		}
		if sum < bestCost {
			bestCost = sum
			best = candidate
		}
	}
	return best
}
