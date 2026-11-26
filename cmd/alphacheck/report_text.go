package main

import (
	"fmt"
	"strings"

	"alphacheck/webm"
)

func printTextReport(r *webm.Report) {
	fmt.Printf("文件: %s (%d bytes)\n", r.Path, r.FileSize)
	fmt.Printf("TimecodeScale: %d ns; 帧率参考: %.3f fps\n", r.TimecodeScaleNS, r.Timing.DefaultFrameRate)
	for _, t := range r.Tracks {
		if t.TrackType == 2 {
			fmt.Printf("轨 %d: %s (audio)\n", t.TrackNumber, t.CodecID)
			continue
		}
		visible := fmt.Sprintf("%dx%d", t.VisibleWidth, t.VisibleHeight)
		if t.VisibleWidth == 0 || t.VisibleHeight == 0 {
			visible = "invalid"
		}
		fmt.Printf("轨 %d: %s (%s), AlphaMode=%s, 编码尺寸=%dx%d, 容器尺寸=%dx%d, 裁剪=L%d/R%d/T%d/B%d, 可见=%s, 帧=%d, Alpha载荷=%d\n",
			t.TrackNumber, t.CodecID, t.CodecName, alphaModeText(t),
			bitstreamWidth(t), bitstreamHeight(t), t.PixelWidth, t.PixelHeight,
			t.Crop.Left, t.Crop.Right, t.Crop.Top, t.Crop.Bottom,
			visible, t.FrameCount, t.Alpha.FramesWithAdditional)
	}
	fmt.Printf("时间: 首帧 %.3f ms, 末帧 %.3f ms, 平均间隔 %.3f ms, CV=%.4f, 倒退=%d, 重复=%d, 缺口=%d, 不规则=%d\n",
		float64(r.Timing.FirstPTSNS)/1e6, float64(r.Timing.LastPTSNS)/1e6,
		r.Timing.MeanIntervalNS/1e6, r.Timing.IntervalCV,
		r.Timing.NonMonotonic, r.Timing.Duplicates, r.Timing.Gaps, r.Timing.IrregularIntervals)

	if len(r.Findings) == 0 {
		fmt.Println("结论: PASS — AlphaMode、裁剪尺寸和帧时间未发现结构矛盾")
		return
	}
	fmt.Printf("结论: %s — %s\n", strings.ToUpper(r.Verdict.Status), r.Verdict.ResponsibilityZH)
	for _, f := range r.Findings {
		where := ""
		if f.Frame > 0 {
			where = fmt.Sprintf(" [track=%d frame=%d]", f.Track, f.Frame)
		} else if f.Track > 0 {
			where = fmt.Sprintf(" [track=%d]", f.Track)
		}
		fmt.Printf("- %s/%s %s%s: %s\n", f.Severity, f.Class, f.Code, where, f.Message)
		if f.Evidence != "" {
			fmt.Printf("  证据: %s\n", f.Evidence)
		}
	}
}

func alphaModeText(t webm.TrackReport) string {
	if !t.AlphaModePresent {
		return "absent"
	}
	return fmt.Sprint(t.AlphaMode)
}

func bitstreamWidth(t webm.TrackReport) int {
	if t.Bitstream == nil {
		return 0
	}
	return t.Bitstream.Width
}

func bitstreamHeight(t webm.TrackReport) int {
	if t.Bitstream == nil {
		return 0
	}
	return t.Bitstream.Height
}
