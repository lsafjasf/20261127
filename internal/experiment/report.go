package experiment

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
)

var headers = []string{
	"scene", "region", "strategy",
	"luma_rmse", "log_rmse",
	"highlight_rel_err", "highlight_pixels",
	"hue_err_deg", "chroma_ratio", "color_pixels",
	"gt_luma_std", "est_luma_std", "noise_ratio",
}

// WriteCSV 把逐区域指标写为 CSV。
func WriteCSV(rows []RegionMetrics, dir string) (string, error) {
	path := filepath.Join(dir, "metrics_detail.csv")
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	if err := w.Write(headers); err != nil {
		return "", err
	}
	for _, r := range rows {
		rec := []string{
			r.Scene, r.Region, r.Strategy,
			fp(r.RMSE), fp(r.LogRMSE),
			fp(r.HighlightRelErr), fmt.Sprintf("%d", r.HighlightPixels),
			fp(r.HueDeg), fp(r.ChromaRatio), fmt.Sprintf("%d", r.ColorPixels),
			fp(r.GTStd), fp(r.EstStd), fp(r.NoiseR),
		}
		if err := w.Write(rec); err != nil {
			return "", err
		}
	}
	w.Flush()
	return path, w.Error()
}

// WriteMarkdown 输出策略总览与重点场景分区表。
func WriteMarkdown(rows []RegionMetrics, p Params, dir string) (string, error) {
	path := filepath.Join(dir, "report.md")
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	fmt.Fprintf(f, "# 逆色调映射（iTM）实验报告\n\n")
	fmt.Fprintf(f, "参数：肩指数 n=%.2f，白参考 w=%.2f，外推上界 upper=%.2f，线性增益 gain=%.2f，局部 sigma=%.1f\n\n",
		p.ShoulderN, p.White, p.Upper, p.LinearGain, p.LocalSigma)

	fmt.Fprintf(f, "## 策略总览（全区域均值）\n\n")
	fmt.Fprintf(f, "| 策略 | 亮度 RMSE | log10 RMSE | 色相误差(°) | 噪声放大倍数 |\n")
	fmt.Fprintf(f, "|---|---|---|---|---|\n")
	for _, a := range AggregateByStrategy(rows) {
		fmt.Fprintf(f, "| %s | %.4f | %.4f | %.3f | %.3f |\n",
			a.Strategy, a.MeanRMSE, a.MeanLogRMSE, a.MeanHueDeg, a.MeanNoiseR)
	}

	for _, sc := range []string{"exposure_staircase", "controlled_highlights", "hue_chart", "noise_chart"} {
		fmt.Fprintf(f, "\n## 场景：%s\n\n", sc)
		fmt.Fprintf(f, "| 区域 | 策略 | 亮度 RMSE | 高光相对误差 | 色相误差(°) | 色度比 | 噪声倍数 |\n")
		fmt.Fprintf(f, "|---|---|---|---|---|---|---|\n")
		for _, r := range rows {
			if r.Scene != sc {
				continue
			}
			fmt.Fprintf(f, "| %s | %s | %.4f | %.4f | %.3f | %.3f | %.3f |\n",
				r.Region, r.Strategy, r.RMSE, r.HighlightRelErr,
				r.HueDeg, r.ChromaRatio, r.NoiseR)
		}
	}
	return path, nil
}

func fp(v float64) string { return fmt.Sprintf("%.6f", v) }
