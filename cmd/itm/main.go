// 命令 itm 运行逆色调映射对比实验并输出报告与可视化 PNG。
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"itm/internal/experiment"
	"itm/internal/img"
	"itm/internal/scene"
	"itm/internal/tmo"
	"itm/internal/viz"
)

func main() {
	outDir := flag.String("out", "output", "报告与图像输出目录")
	pngOut := flag.Bool("png", true, "是否导出各场景的 SDR/三策略/真值对比 PNG")
	flag.Parse()

	p := experiment.DefaultParams()
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fatal(err)
	}

	rows := experiment.Run(p)

	csvPath, err := experiment.WriteCSV(rows, *outDir)
	if err != nil {
		fatal(err)
	}
	mdPath, err := experiment.WriteMarkdown(rows, p, *outDir)
	if err != nil {
		fatal(err)
	}

	if *pngOut {
		if err := exportPNGs(p, *outDir); err != nil {
			fatal(err)
		}
	}

	fmt.Println("== 策略总览 ==")
	for _, a := range experiment.AggregateByStrategy(rows) {
		fmt.Println(a.String())
	}
	fmt.Printf("\n报告已写出：\n- %s\n- %s\n", csvPath, mdPath)
}

func exportPNGs(p experiment.Params, outDir string) error {
	imgDir := filepath.Join(outDir, "png")
	if err := os.MkdirAll(imgDir, 0o755); err != nil {
		return err
	}
	for _, sc := range scene.All() {
		sdr := tmo.CameraToSDR(sc.Image, p.ShoulderN, p.White)
		lin := tmo.LinearExpand(sdr, p.LinearGain)
		soft := tmo.SoftShoulder(sdr, p.ShoulderN, p.White, p.Upper)
		local := tmo.LocalProtect(sdr, p.ShoulderN, p.White, p.Upper,
			p.LocalSigma, p.LocalLow, p.LocalHigh, p.LocalFloor)

		// 用真值峰值做统一显示归一化，保证五张图视觉可比。
		peak := 0.0
		for _, v := range sc.Image.R {
			if v > peak {
				peak = v
			}
		}
		if peak < 1 {
			peak = 1
		}
		base := filepath.Join(imgDir, sc.Name)
		for name, im := range map[string]*img.Image{
			"groundtruth": sc.Image,
			"sdr":         sdr,
			"linear":      lin,
			"soft":        soft,
			"local":       local,
		} {
			if err := viz.SavePNG(base+"_"+name+".png", im, peak); err != nil {
				return err
			}
		}
	}
	return nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
