package check

import (
	"fmt"
	"image"
	"path/filepath"

	"proalpha/internal/imagex"
)

func WriteFrameArtifacts(outputDir, fileBase string, analyzed *AnalyzedFrame, source, variant *image.NRGBA64) error {
	if analyzed.Report.Outputs == nil {
		analyzed.Report.Outputs = make(map[string]string)
	}
	frameName := fmt.Sprintf("frame_%04d", analyzed.Report.Index)
	prefix := filepath.Join(outputDir, fileBase+"_"+frameName)

	repairedPath := prefix + "_repaired.png"
	if err := imagex.SavePNG(repairedPath, analyzed.Repaired); err != nil {
		return err
	}
	analyzed.Report.Outputs["repaired_rgba_png"] = repairedPath

	alphaPath := prefix + "_alpha.png"
	if err := imagex.SavePNG(alphaPath, imagex.AlphaMask(analyzed.Repaired)); err != nil {
		return err
	}
	analyzed.Report.Outputs["repaired_alpha_mask"] = alphaPath

	checkerVariant := imagex.CompositeChecker(variant, 12, 52428, 13107)
	checkerRepaired := imagex.CompositeChecker(analyzed.Repaired, 12, 52428, 13107)
	pair := imagex.HorizontalPair(checkerVariant, checkerRepaired)
	checkerPath := prefix + "_checker_before_after.png"
	if err := imagex.SavePNG(checkerPath, pair); err != nil {
		return err
	}
	analyzed.Report.Outputs["checker_before_after"] = checkerPath

	variantPath := prefix + "_variant_decoded.png"
	if err := imagex.SavePNG(variantPath, variant); err != nil {
		return err
	}
	analyzed.Report.Outputs["variant_decoded"] = variantPath

	if source != nil {
		sourcePath := prefix + "_source_aligned.png"
		if err := imagex.SavePNG(sourcePath, source); err != nil {
			return err
		}
		analyzed.Report.Outputs["source_aligned"] = sourcePath
	}
	return nil
}
