// 命令 mipstudy：输出三档缩放比下的层号/误差/轴比并排表，
// 并生成修正前后两张对比 PGM（各含三档横带，自上而下 r=1、2、4）。
package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"

	"vecgo/mipstudy"
	"vecgo/raster"
)

type tierCfg struct {
	label     string
	a, b      float64 // 足迹长轴、短轴（纹素）
	scaleText string
}

var tiers = []tierCfg{
	{label: "档1 等方 2:1x", a: 2, b: 2, scaleText: "sx=1/2 sy=1/2"},
	{label: "档2 各向异性 4:1x", a: 4, b: 2, scaleText: "sx=1/4 sy=1/2"},
	{label: "档3 各向异性 8:1x", a: 8, b: 2, scaleText: "sx=1/8 sy=1/2"},
}

func main() {
	outDir := "artifacts"
	if len(os.Args) > 1 {
		outDir = os.Args[1]
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "建目录失败:", err)
		os.Exit(1)
	}

	const bandW, bandH = 256, 96
	tex := mipstudy.NewCheckerTexture(256, 4)

	before, _ := raster.New(bandW, bandH*3+4) // 两条 2px 分隔线
	after, _ := raster.New(bandW, bandH*3+4)

	fmt.Println("三档并排表（b 固定为 2 纹素，a 为长轴）")
	fmt.Printf("%-22s %-16s %6s %8s %8s %8s %8s %8s %8s\n",
		"档位", "缩放", "r=a/b", "L0面积", "L*长边", "误差e", "修正C", "L0+C", "floor前后")
	var prevLstar float64
	for i, tc := range tiers {
		r := mipstudy.AxisRatio(tc.a, tc.b)
		l0 := mipstudy.LevelArea(tc.a, tc.b)
		lstar := mipstudy.LevelMaxAxis(tc.a, tc.b)
		e := mipstudy.ErrorArea(r)
		c := mipstudy.Correction(r)
		lc := mipstudy.LevelCorrected(tc.a, tc.b)
		k0 := mipstudy.FloorLevel(l0, tex.MaxLevel())
		kc := mipstudy.FloorLevel(lc, tex.MaxLevel())

		diffText := "-"
		if i > 0 {
			prevR := tiers[i-1].a / tiers[i-1].b
			diffText = fmt.Sprintf("ΔL*=%.3f=log2(%.0f/%.0f)", lstar-prevLstar, r, prevR)
		}
		fmt.Printf("%-22s %-16s %6.1f %8.3f %8.3f %8.3f %8.3f %8.3f   %d -> %d  %s\n",
			tc.label, tc.scaleText, r, l0, lstar, e, c, lc, k0, kc, diffText)
		prevLstar = lstar

		b0 := tex.RenderBand(bandW, bandH, tc.a, tc.b, false)
		b1 := tex.RenderBand(bandW, bandH, tc.a, tc.b, true)
		off := i * (bandH + 2)
		copy(before.Pix[off*bandW:(off+bandH)*bandW], b0.Pix)
		copy(after.Pix[off*bandW:(off+bandH)*bandW], b1.Pix)
	}

	// 分隔线置灰，便于在 PGM 里区分三档。
	for i := 1; i <= 2; i++ {
		y := i*bandH + (i-1)*2
		for x := 0; x < bandW; x++ {
			before.Pix[y*bandW+x] = 64
			after.Pix[y*bandW+x] = 64
		}
	}

	p1 := filepath.Join(outDir, "mip_before.pgm")
	p2 := filepath.Join(outDir, "mip_after.pgm")
	if err := before.SavePGM(p1); err != nil {
		fmt.Fprintln(os.Stderr, "导出失败:", err)
		os.Exit(2)
	}
	if err := after.SavePGM(p2); err != nil {
		fmt.Fprintln(os.Stderr, "导出失败:", err)
		os.Exit(2)
	}

	// 轴比趋一的单独验证 + 误差有界验证。
	fmt.Println("\n退化验证 r -> 1：C(r) = 1/2*log2(r)")
	for _, r := range []float64{4, 2, 1.25, 1.01, 1.0001, 1} {
		fmt.Printf("  r=%8.4f  C(r)=%.6f  残差(L0+C-L*)=%.2e\n",
			r, mipstudy.Correction(r), mipstudy.LevelCorrected(r, 1)-mipstudy.LevelMaxAxis(r, 1))
	}
	fmt.Println("\n误差随轴比增大（未修正 |e| = 1/2*log2(r)，无上界；修正后残差恒 0）")
	for _, r := range []float64{1, 2, 4, 8, 16, 64, 256} {
		fmt.Printf("  r=%6.1f  e=%.4f  修正残差=%.2e\n",
			r, mipstudy.ErrorArea(r), math.Abs(mipstudy.LevelCorrected(r, 1)-mipstudy.LevelMaxAxis(r, 1)))
	}
	fmt.Println("\n图片已保存:", p1, p2)
}
