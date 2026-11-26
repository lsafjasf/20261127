package gamutcompare

import (
	"fmt"
	"strings"
)

// FormatText renders a compact Chinese plain-text report.
func FormatText(report *Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "色域几何比较（坐标假设：%s）\n", report.AssumedCoordinates)
	fmt.Fprintf(&b, "目标：%s；xy 面积 %.6f；u'v' 面积 %.6f\n",
		report.Target.Name, report.Target.AreaXY, report.Target.AreaUpVP)

	for _, display := range report.Displays {
		fmt.Fprintf(&b, "\n显示器：%s\n", display.Name)
		fmt.Fprintf(&b, "  色域面积：xy=%.6f，u'v'=%.6f\n", display.Area.XY, display.Area.UpVP)
		fmt.Fprintf(&b, "  目标覆盖：xy %.2f%% / u'v' %.2f%%；交集 xy 面积=%.6f（占本机三角形 %.2f%%）\n",
			display.Coverage.TargetCoveragePercent,
			display.Coverage.TargetCoverageUpVPPercent,
			display.Coverage.IntersectionAreaXY,
			display.Coverage.DisplayAreaInTargetRatio*100)
		if display.White != nil {
			fmt.Fprintf(&b, "  白点可混出：%v；RGB 相对亮度贡献=(%.4f, %.4f, %.4f)\n",
				display.White.Inside,
				display.White.LuminanceShares[0],
				display.White.LuminanceShares[1],
				display.White.LuminanceShares[2])
		}
		writeDirections(&b, "不可覆盖方向", display.Directions)
		if display.CommonDirections != nil {
			writeDirections(&b, "不可覆盖方向（统一目标白点）", *display.CommonDirections)
		}
		for _, warning := range display.Warnings {
			fmt.Fprintf(&b, "  警告：%s\n", warning)
		}
	}

	fmt.Fprintf(&b, "\n比较\n")
	fmt.Fprintf(&b, "  xy 面积最大：%s\n", report.Comparison.LargestAreaDisplay)
	fmt.Fprintf(&b, "  u'v' 面积最大：%s\n", report.Comparison.LargestUpVPAreaDisplay)
	fmt.Fprintf(&b, "  目标覆盖最高：%s\n", report.Comparison.HighestTargetCoverageDisplay)
	for _, pair := range report.Comparison.Pairs {
		fmt.Fprintf(&b, "  %s vs %s：面积差 %.6f（%.2f%%），覆盖差 %.2f 个百分点；面积领先=%s，覆盖领先=%s\n",
			pair.A, pair.B, pair.AreaDifferenceXY, pair.AreaRelativeDifference*100,
			pair.CoverageDifferencePercentPts, pair.LargerArea, pair.HigherTargetCoverage)
		if pair.AreaAndCoverageOrdersDiffer {
			b.WriteString("  注意：面积领先与目标覆盖领先不是同一台，不能用面积排序代替目标覆盖。\n")
		}
	}

	b.WriteString("\n限制\n")
	for _, caveat := range report.Caveats {
		fmt.Fprintf(&b, "  - %s\n", caveat)
	}
	return b.String()
}

func writeDirections(b *strings.Builder, title string, report DirectionReport) {
	if report.FullTargetCovered {
		fmt.Fprintf(b, "  %s（中心=%s）：无；目标边界在该中心下均被覆盖\n", title, report.CenterKind)
		return
	}
	fmt.Fprintf(b, "  %s（中心=%s）：%d 段（角度：0°=+x，逆时针）\n", title, report.CenterKind, len(report.Sectors))
	for _, sector := range report.Sectors {
		end := sector.EndAngleDeg
		wrap := ""
		if end >= 360 {
			wrap = "（跨 0°）"
		}
		fmt.Fprintf(b, "    * %7.2f° → %7.2f°，宽 %6.2f°%s；最大缺口 %.5f @ %7.2f°，中线缺口 %.5f\n",
			sector.StartAngleDeg, end, sector.WidthDeg, wrap,
			sector.MaxRadialGap, sector.MaxGapAngleDeg, sector.RadialGapAtMidpoint)
	}
	if !report.CenterInsideTarget || !report.CenterInsideCover {
		fmt.Fprintf(b, "    注意：中心不在目标/显示器多边形内部，径向缺口仅作边界距离参考\n")
	}
}
