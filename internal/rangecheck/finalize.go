package rangecheck

import (
	"fmt"
	"sort"
)

func finalizeReport(rep *Report) error {
	if len(rep.Frames) == 0 {
		return fmt.Errorf("no frames were analyzed")
	}
	finalizeFPS(rep)
	aggregateSources(rep)
	buildSegments(rep)
	buildStyleIntervals(rep)
	buildConclusions(rep)
	return nil
}

func buildStyleIntervals(rep *Report) {
	start := -1
	var black, white, outOfRange float64
	count := 0
	flush := func(end int) {
		if start < 0 || count == 0 {
			return
		}
		interval := StyleInterval{
			FirstFrame:      rep.Frames[start].Index,
			LastFrame:       rep.Frames[end].Index,
			StartSeconds:    rep.Frames[start].PTSSeconds,
			EndSeconds:      rep.Frames[end].PTSSeconds + frameDuration(rep),
			FrameCount:      count,
			BlackWallRatio:  black / float64(count),
			WhiteWallRatio:  white / float64(count),
			OutOfRangeRatio: outOfRange / float64(count),
		}
		if interval.OutOfRangeRatio < 0.001 {
			interval.Interpretation = "strong_legal_16_or_235_walls_are_consistent_with_intentional_crush_or_clip"
		} else {
			interval.Interpretation = "legal_style_walls_with_small_out_of_range_tail; style is plausible, thin overshoot should be treated as codec/range conversion evidence"
		}
		rep.StyleIntervals = append(rep.StyleIntervals, interval)
		start = -1
		black, white, outOfRange, count = 0, 0, 0, 0
	}
	for i, frame := range rep.Frames {
		pixels := maxInt(1, frame.Y.Pixels)
		blackWall := float64(frame.Y.NearBlackLegal) / float64(pixels)
		whiteWall := float64(frame.Y.NearWhiteLegal) / float64(pixels)
		stylish := blackWall >= 0.12 || whiteWall >= 0.12
		if stylish {
			if start < 0 {
				start = i
			}
			black += blackWall
			white += whiteWall
			outOfRange += frame.OutOfRangeRatio
			count++
		}
		if start >= 0 {
			if !stylish {
				flush(i - 1)
			} else if i == len(rep.Frames)-1 {
				flush(i)
			}
		}
	}
}

func finalizeFPS(rep *Report) {
	fpsReport := &rep.FPS
	fpsReport.FrameCount = len(rep.Frames)
	if rep.Settings.FPS.Num != 0 && rep.Settings.FPS.Den != 0 {
		fpsReport.DeclaredRational = fmt.Sprintf("%d/%d", rep.Settings.FPS.Num, rep.Settings.FPS.Den)
		fpsReport.DeclaredFPS = rep.Settings.FPS.Float64()
	}
	if rep.Metadata.FrameRate.Num != 0 && rep.Metadata.FrameRate.Den != 0 {
		fpsReport.DeclaredRational = fmt.Sprintf("%d/%d", rep.Metadata.FrameRate.Num, rep.Metadata.FrameRate.Den)
		fpsReport.DeclaredFPS = rep.Metadata.FrameRate.Float64()
	}

	type interval struct {
		ms float64
	}
	var intervals []float64
	nonZeroPTS := 0
	for i := 1; i < len(rep.Frames); i++ {
		cur, prev := rep.Frames[i].PTSSeconds, rep.Frames[i-1].PTSSeconds
		if cur > 0 {
			nonZeroPTS++
		}
		if cur >= prev && cur-prev > 0 {
			intervals = append(intervals, (cur-prev)*1000)
		}
	}
	if nonZeroPTS > 0 {
		fpsReport.PTSSource = "frame_pts"
	} else if fpsReport.DeclaredFPS > 0 {
		fpsReport.PTSSource = "declared_fps_assumed_constant"
		for i := 1; i < len(rep.Frames); i++ {
			intervals = append(intervals, 1000/fpsReport.DeclaredFPS)
		}
	} else {
		fpsReport.PTSSource = "unavailable"
		fpsReport.Warnings = append(fpsReport.Warnings, "no PTS or FPS supplied; frame rates cannot be verified from pixels alone")
	}
	if len(intervals) == 0 {
		return
	}
	sorted := append([]float64(nil), intervals...)
	sort.Float64s(sorted)
	median := sorted[len(sorted)/2]
	fpsReport.ObservedMedian = 1000 / median
	fpsReport.MinIntervalMS = sorted[0]
	fpsReport.MaxIntervalMS = sorted[len(sorted)-1]
	if len(rep.Frames) >= 2 {
		first := rep.Frames[0].PTSSeconds
		last := rep.Frames[len(rep.Frames)-1].PTSSeconds
		if last > first {
			fpsReport.AverageFPS = float64(len(rep.Frames)-1) / (last - first)
		}
	}
	outliers := 0
	for _, d := range intervals {
		relative := absFloat(d-median) / median
		if relative > 0.06 {
			outliers++
		}
	}
	fpsReport.VFRScore = float64(outliers) / float64(len(intervals))
	if fpsReport.VFRScore > 0.10 {
		fpsReport.Warnings = append(fpsReport.Warnings,
			fmt.Sprintf("%.1f%% frame intervals differ from median by >6%%; treat stream as VFR or check dropped/duplicated frames", fpsReport.VFRScore*100))
	}
}

func absFloat(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func aggregateSources(rep *Report) {
	type agg struct {
		summary SourceSummary
		frames  map[int]bool
	}
	bySource := make(map[SourceClass]*agg)
	for _, frame := range rep.Frames {
		for _, region := range frame.Regions {
			a := bySource[region.Source]
			if a == nil {
				a = &agg{frames: make(map[int]bool)}
				a.summary = SourceSummary{
					Source: region.Source, Verdict: region.Verdict,
					FirstFrame: frame.Index, LastFrame: frame.Index,
					ExampleFrame: frame.Index, ExampleRect: region.Rect,
				}
				bySource[region.Source] = a
			}
			a.summary.RegionCount++
			a.summary.PixelCount += int64(region.PixelCount)
			a.frames[frame.Index] = true
			a.summary.LastFrame = frame.Index
			if region.Confidence > 0.7 && a.summary.ExampleRect.Area() < region.Rect.Area() {
				a.summary.ExampleFrame = frame.Index
				a.summary.ExampleRect = region.Rect
				a.summary.Verdict = region.Verdict
			}
		}
	}
	for _, a := range bySource {
		a.summary.Frames = len(a.frames)
		rep.Sources = append(rep.Sources, a.summary)
	}
	sort.Slice(rep.Sources, func(i, j int) bool { return rep.Sources[i].PixelCount > rep.Sources[j].PixelCount })
}

func buildSegments(rep *Report) {
	var start int = -1
	for i := range rep.Frames {
		if rep.Frames[i].HasOutOfRange && start < 0 {
			start = i
		}
		if start >= 0 && (!rep.Frames[i].HasOutOfRange || i == len(rep.Frames)-1) {
			end := i - 1
			if rep.Frames[i].HasOutOfRange {
				end = i
			}
			rep.Segments = append(rep.Segments, makeSegment(rep, len(rep.Segments)+1, start, end))
			start = -1
		}
	}
}

func makeSegment(rep *Report, id, first, last int) Segment {
	seg := Segment{ID: id, FirstFrame: rep.Frames[first].Index, LastFrame: rep.Frames[last].Index}
	seg.StartSeconds = rep.Frames[first].PTSSeconds
	seg.EndSeconds = rep.Frames[last].PTSSeconds + frameDuration(rep)
	seenKinds := map[RegionKind]bool{}
	seenTracks := map[int]bool{}
	sourcePixels := map[SourceClass]int64{}
	var exactBlack, exactWhite, totalPixels int64
	for i := first; i <= last; i++ {
		f := rep.Frames[i]
		totalPixels += int64(f.Width * f.Height)
		exactBlack += int64(f.Y.ExactBlack)
		exactWhite += int64(f.Y.ExactWhite)
		if f.OutOfRangeRatio > seg.PeakRatio {
			seg.PeakRatio = f.OutOfRangeRatio
			seg.PeakFrame = f.Index
		}
		for _, r := range f.Regions {
			seenKinds[r.Kind] = true
			if r.TrackID != 0 {
				seenTracks[r.TrackID] = true
			}
			sourcePixels[r.Source] += int64(r.PixelCount)
		}
	}
	for kind := range seenKinds {
		seg.Kinds = append(seg.Kinds, string(kind))
	}
	sort.Strings(seg.Kinds)
	seg.TrackIDs = nil
	for track := range seenTracks {
		seg.TrackIDs = append(seg.TrackIDs, track)
	}
	sort.Ints(seg.TrackIDs)
	seg.FrameCount = last - first + 1

	hasSubtitle := sourcePixels[SourceBurnedSubtitle] > 0
	hasGraphicText := sourcePixels[SourceGraphicText] > 0
	hasHighlight := sourcePixels[SourceHighlight] > 0
	hasCodec := sourcePixels[SourceCodecEdge] > 0
	hasLevelShift := sourcePixels[SourceLevelShift] > 0
	hasStyle := sourcePixels[SourceStylizedGrade] > 0
	denominator := float64(totalPixels)
	if denominator <= 0 {
		denominator = 1
	}
	anchorMismatch := float64(exactBlack)/denominator > 0.0002 &&
		float64(exactWhite)/denominator > 0.0002

	switch {
	case hasSubtitle && hasHighlight:
		seg.Classification = "localized_superblack_subtitle_and_superwhite_highlight"
		seg.Conclusion = "同一段内同时出现下区字幕型超黑团块和高光型超白团块；优先检查字幕/图形叠层与全范围到 limited range 的转换，不能用整片峰值代替定位。"
	case hasSubtitle:
		seg.Classification = "superblack_burned_subtitle_range_error"
		seg.Conclusion = "超黑像素呈稳定下区字形/行框，更像烧录字幕或图形层未做 16 黑位转换，属于 limited-range 母带/编码错误。"
	case hasGraphicText:
		seg.Classification = "superblack_graphic_or_ui_text_range_error"
		seg.Conclusion = "超黑像素呈字形/行框但不在典型字幕带，优先按烧录标题、UI 或终端文字的范围转换错误排查。"
	case hasLevelShift && dominantSource(sourcePixels) == SourceLevelShift:
		seg.Classification = "flat_level_shift_or_range_tag_error"
		seg.Conclusion = "大面积区域仅浅度越过 16/235，更像量化、拼接段电平或范围标签偏移；不像局部字幕/高光创作。"
	case hasCodec && dominantSource(sourcePixels) == SourceCodecEdge:
		seg.Classification = "codec_edge_overshoot"
		seg.Conclusion = "越界主要是高对比边缘两侧的浅过冲，更像压缩/反复编码产生的 ringing，而不是独立风格段。"
	case hasStyle:
		seg.Classification = "intentional_style_candidate_with_illegal_codes"
		seg.Conclusion = "大面积连续色调区域更像风格化压黑/提白；风格意图可以保留，但码值越过 limited range 法定区间，交付前仍应合法化或确认播放器/标签链路。"
	case hasHighlight:
		seg.Classification = "superwhite_highlight_requires_mastering_review"
		seg.Conclusion = "超白集中在高光区域，可能是保留的高光余量或风格化提白；是否为编码错误需结合母带元数据和转换链确认。"
	case anchorMismatch:
		seg.Classification = "possible_full_range_tag_or_conversion_error"
		seg.Conclusion = "该段同时出现显著码值 0 和 255 锚点，疑似 full-range 数据被按 limited range 标记，或转换/拼接参数不一致。"
	default:
		seg.Classification = "out_of_range_manual_review"
		seg.Conclusion = "存在逐帧越界，但单靠码值和几何形态不足以唯一确定来源；应查看叠加框、前后母版和色彩标签。"
	}
	if anchorMismatch && (hasSubtitle || hasHighlight) {
		seg.Classification += "_with_full_range_anchors"
	}
	return seg
}

func dominantSource(pixels map[SourceClass]int64) SourceClass {
	var best SourceClass
	var n int64
	for source, count := range pixels {
		if count > n {
			n, best = count, source
		}
	}
	return best
}

func frameDuration(rep *Report) float64 {
	if rep.FPS.DeclaredFPS > 0 {
		return 1 / rep.FPS.DeclaredFPS
	}
	return 0
}

func buildConclusions(rep *Report) {
	anomalyFrames := 0
	var black, white int64
	for _, f := range rep.Frames {
		if f.HasOutOfRange {
			anomalyFrames++
		}
		black += int64(f.Y.BelowLegal)
		white += int64(f.Y.AboveLegal)
	}
	rep.Conclusions = append(rep.Conclusions,
		fmt.Sprintf("共分析 %d 帧；%d 帧存在 Y<16 或 Y>235，超黑样本 %d 像素次、超白样本 %d 像素次。", len(rep.Frames), anomalyFrames, black, white))
	if len(rep.Segments) == 0 {
		rep.Conclusions = append(rep.Conclusions, "所有逐帧 Y 值均在配置的 legal range 内；未发现题述超黑/超白。")
	} else {
		rep.Conclusions = append(rep.Conclusions, fmt.Sprintf("检测到 %d 个连续越界段；详见 segments、tracks 和每帧 regions。", len(rep.Segments)))
	}
	if len(rep.StyleIntervals) > 0 {
		rep.Conclusions = append(rep.Conclusions, fmt.Sprintf("检测到 %d 段强 16/235 合法墙；这更像故意压黑/提白或硬裁切，需与少量越界 ringing 分开判断。", len(rep.StyleIntervals)))
	}

	if rep.Metadata.ColorRange != "" {
		if rep.Settings.Range == RangeTV && (rep.Metadata.ColorRange == "full" || rep.Metadata.ColorRange == "pc" || rep.Metadata.ColorRange == "jpeg") {
			rep.Conclusions = append(rep.Conclusions, "容器/码流色彩范围声明为 full，但分析阈值使用 limited；必须先确认是否为标签错误，不能直接把越界判成创意意图。")
		}
		if rep.Metadata.ColorRange == "tv" || rep.Metadata.ColorRange == "limited" {
			rep.Conclusions = append(rep.Conclusions, "元数据声明为 tv/limited；Y<16 与 Y>235 是与交付声明不一致的技术证据。")
		}
	}

	rep.ManualChecks = append(rep.ManualChecks,
		"查看 JSON 中每个 frame/region 的 frame、pts_seconds 和 rect；可用 --overlay-dir 输出框选 PNG。",
		"核对源工程/字幕轨：烧录字幕的 RGB 黑是否经 limited-range 矩阵转换，高光是否应在母带阶段 legalize。",
		"核对 H.264 VUI、容器 colr 与播放器实际色彩标签；原始 YUV 统计优先于解码后的 RGB 截图。",
	)
}
