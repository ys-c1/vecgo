// 自适应采样验证：五幅测试图，分别校验
//  (A) 与全图统一高采样的差异低于阈值 diffLimit
//  (B) 总样本数低于统一采样的 ratioCap 比例
// 两条独立判定，并把 PGM 输出到 artifacts/adaptive/。
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"vecgo/geom"
	"vecgo/raster"
)

const (
	W, H        = 64, 64
	refSS       = 8    // 基准：每像素 8×8 = 64 样本
	tol         = 0.1  // 自适应容差（覆盖率标准误上限）
	maxPerPixel = 64   // 单像素样本上限
	diffLimit   = 26   // 差异阈值（灰阶，约 10% 覆盖率）
	ratioCap    = 0.5  // 样本数占统一采样的比例上限
)

var view = raster.View{MinX: 0, MinY: 0, MaxX: 64, MaxY: 64}

type scene struct {
	name  string
	polys [][]geom.Point
}

func rect(x0, y0, x1, y1 float64) []geom.Point {
	return []geom.Point{{X: x0, Y: y0}, {X: x1, Y: y0}, {X: x1, Y: y1}, {X: x0, Y: y1}}
}

func scenes() []scene {
	texture := [][]geom.Point{}
	for j := 0; j < 8; j++ {
		for i := 0; i < 8; i++ {
			x, y := float64(8*i), float64(8*j)
			texture = append(texture, rect(x, y, x+4, y+4))
		}
	}
	return []scene{
		{name: "平坦", polys: [][]geom.Point{rect(8, 8, 56, 56)}},
		{name: "斜边", polys: [][]geom.Point{{{X: 32, Y: 4}, {X: 60, Y: 32}, {X: 32, Y: 60}, {X: 4, Y: 32}}}},
		{name: "细缝", polys: [][]geom.Point{rect(0, 0, 30.3, 64), rect(33.7, 0, 64, 64)}},
		{name: "纹理", polys: texture},
		{name: "整幅同色", polys: [][]geom.Point{rect(-8, -8, 72, 72)}},
	}
}

func mark(ok bool) string {
	if ok {
		return "通过"
	}
	return "不通过"
}

func main() {
	dir := "artifacts/adaptive"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "建目录失败:", err)
		os.Exit(1)
	}
	uniformTotal := W * H * refSS * refSS
	failed := false
	for _, sc := range scenes() {
		fmt.Printf("== %s ==\n", sc.name)
		ref, _ := raster.New(W, H)
		if err := ref.FillUniform(sc.polys, view, refSS); err != nil {
			fmt.Println("  基准采样失败:", err)
			failed = true
			continue
		}
		ad, _ := raster.New(W, H)
		st, err := ad.FillAdaptive(sc.polys, view, tol, maxPerPixel)
		if err != nil {
			fmt.Println("  自适应采样拒绝:", err)
			fmt.Println()
			continue
		}
		// 差异分布
		buckets := [5]int{} // 0 / 1-8 / 9-16 / 17-25 / >=26
		maxDiff, sum := 0, 0
		for i := range ad.Pix {
			d := int(ad.Pix[i]) - int(ref.Pix[i])
			if d < 0 {
				d = -d
			}
			sum += d
			if d > maxDiff {
				maxDiff = d
			}
			switch {
			case d == 0:
				buckets[0]++
			case d <= 8:
				buckets[1]++
			case d <= 16:
				buckets[2]++
			case d <= 25:
				buckets[3]++
			default:
				buckets[4]++
			}
		}
		mean := float64(sum) / float64(len(ad.Pix))
		ratio := float64(st.Total()) / float64(uniformTotal)
		okDiff := maxDiff <= diffLimit
		okRatio := ratio < ratioCap
		fmt.Printf("  样本: 自适应 %d (探测 %d + 合成 %d, 单像素最多 %d) vs 统一 %d, 比例 %.3f\n",
			st.Total(), st.ProbeSamples, st.PixelSamples, st.MaxPerPixel, uniformTotal, ratio)
		fmt.Printf("  差异: max %d, mean %.2f, 分布 [0]:%d [1-8]:%d [9-16]:%d [17-25]:%d [>=26]:%d\n",
			maxDiff, mean, buckets[0], buckets[1], buckets[2], buckets[3], buckets[4])
		fmt.Printf("  差异校验(max<=%d): %s    样本校验(比例<%.2f): %s\n", diffLimit, mark(okDiff), ratioCap, mark(okRatio))
		if !okDiff || !okRatio {
			failed = true
		}
		base := filepath.Join(dir, sc.name)
		if err := ref.SavePGM(base + "_uniform.pgm"); err != nil {
			fmt.Fprintln(os.Stderr, "导出失败:", err)
		}
		if err := ad.SavePGM(base + "_adaptive.pgm"); err != nil {
			fmt.Fprintln(os.Stderr, "导出失败:", err)
		}
		fmt.Println()
	}
	if failed {
		os.Exit(1)
	}
}
