// 命令 symexp 验证亚像素采样布局的对称性与镜像光栅化偏差的关系。
// 它对同一非对称形状分别渲染原图、水平镜像图（和 180° 旋转图），
// 逐像素比较灰度，并对每个布局做集合等式判定。
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"vecgo/geom"
	"vecgo/raster"
)

const (
	imgW = 64
	imgH = 64
)

// layout 描述一组候选采样点，Grid=4，坐标均为 0..3 的整数格点。
type layout struct {
	name string
	ss   raster.SampleSet
}

// 三组布局，每组 6 个点：
// S1 破坏对称（既不镜像对称也不中心对称）——反例；
// S2 关于竖直中线镜像对称，但不中心对称；
// S3 关于像素中心中心对称（180° 旋转对称），但不镜像对称。
func layouts() []layout {
	return []layout{
		{
			name: "S1-broken",
			ss: raster.SampleSet{Grid: 4, Pts: []raster.Sample{
				{I: 0, J: 0}, {I: 0, J: 1}, {I: 0, J: 2},
				{I: 1, J: 1}, {I: 1, J: 3}, {I: 2, J: 0},
			}},
		},
		{
			name: "S2-mirror",
			ss: raster.SampleSet{Grid: 4, Pts: []raster.Sample{
				{I: 0, J: 0}, {I: 3, J: 0},
				{I: 0, J: 2}, {I: 3, J: 2},
				{I: 1, J: 1}, {I: 2, J: 1},
			}},
		},
		{
			name: "S3-central",
			ss: raster.SampleSet{Grid: 4, Pts: []raster.Sample{
				{I: 0, J: 0}, {I: 3, J: 3},
				{I: 0, J: 2}, {I: 3, J: 1},
				{I: 1, J: 0}, {I: 2, J: 3},
			}},
		},
	}
}

// shape 是一个非对称直角三角形，包围盒为 [0,64]×[0,64]，
// 宽高为 2 的幂，Grid=4 的采样坐标全部可被 float64 精确表示。
// 两条斜边的斜率为 17/64 与 47/64：分子为奇数，而所有采样点坐标都是
// 1/8 的奇数倍（非整数），可由 17k=64m、47k=64m 无整数解证明没有任何
// 采样点恰好落在边线上，从而排除射线法在边界上的 tie-break 退化。
func shape() []geom.Point {
	return []geom.Point{
		{X: 0, Y: 0},
		{X: 64, Y: 17},
		{X: 0, Y: 64},
	}
}

// normalize 平移多边形使包围盒左下角落在原点 (0,0)。
// 原图、镜像图、旋转图经过归一化后拥有完全相同的 minx/miny/maxx/maxy，
// 光栅器对同一像素列生成的采样坐标逐位相同，排除包围盒原点不同引入的
// 浮点运算顺序差异，让镜像比较只反映采样布局本身的对称性。
func normalize(poly []geom.Point) []geom.Point {
	minx, miny := poly[0].X, poly[0].Y
	for _, p := range poly {
		if p.X < minx {
			minx = p.X
		}
		if p.Y < miny {
			miny = p.Y
		}
	}
	out := make([]geom.Point, len(poly))
	for i, p := range poly {
		out[i] = geom.Point{X: p.X - minx, Y: p.Y - miny}
	}
	return out
}

// mirrorX 关于包围盒竖直中线水平镜像，结果归一化到原点。
func mirrorX(poly []geom.Point) []geom.Point {
	minx, maxx := poly[0].X, poly[0].X
	for _, p := range poly {
		if p.X < minx {
			minx = p.X
		}
		if p.X > maxx {
			maxx = p.X
		}
	}
	out := make([]geom.Point, len(poly))
	for i, p := range poly {
		out[i] = geom.Point{X: minx + maxx - p.X, Y: p.Y}
	}
	return normalize(out)
}

// rotate180 关于包围盒中心旋转 180°，结果归一化到原点。
func rotate180(poly []geom.Point) []geom.Point {
	minx, miny := poly[0].X, poly[0].Y
	maxx, maxy := minx, miny
	for _, p := range poly {
		if p.X < minx {
			minx = p.X
		}
		if p.X > maxx {
			maxx = p.X
		}
		if p.Y < miny {
			miny = p.Y
		}
		if p.Y > maxy {
			maxy = p.Y
		}
	}
	out := make([]geom.Point, len(poly))
	for i, p := range poly {
		out[i] = geom.Point{X: minx + maxx - p.X, Y: miny + maxy - p.Y}
	}
	return normalize(out)
}

func render(poly []geom.Point, ss raster.SampleSet) *raster.Image {
	im, err := raster.New(imgW, imgH)
	if err != nil {
		panic(err)
	}
	if err := im.FillSamples([][]geom.Point{poly}, ss); err != nil {
		panic(err)
	}
	return im
}

// stats 汇总两张图的逐像素灰度差异。
type stats struct {
	nDiff  int     // 灰度不同的像素数
	maxAbs int     // 最大绝对灰度差（0..255）
	l1     int     // 绝对灰度差之和
	pct    float64 // 差异像素占整图百分比
}

func compare(a, b *raster.Image) stats {
	st := stats{}
	for i := range a.Pix {
		d := int(a.Pix[i]) - int(b.Pix[i])
		if d < 0 {
			d = -d
		}
		if d > 0 {
			st.nDiff++
			st.l1 += d
		}
		if d > st.maxAbs {
			st.maxAbs = d
		}
	}
	st.pct = 100 * float64(st.nDiff) / float64(len(a.Pix))
	return st
}

// flipH 把图像沿竖直中线翻转，使镜像图与原图落在同一像素坐标系下比较。
func flipH(im *raster.Image) *raster.Image {
	out, _ := raster.New(im.W, im.H)
	for row := 0; row < im.H; row++ {
		for col := 0; col < im.W; col++ {
			out.Pix[row*im.W+(im.W-1-col)] = im.Pix[row*im.W+col]
		}
	}
	return out
}

// flip180 把图像旋转 180°。
func flip180(im *raster.Image) *raster.Image {
	out, _ := raster.New(im.W, im.H)
	for row := 0; row < im.H; row++ {
		for col := 0; col < im.W; col++ {
			out.Pix[(im.H-1-row)*im.W+(im.W-1-col)] = im.Pix[row*im.W+col]
		}
	}
	return out
}

// saveDiffPGM 把绝对灰度差放大到 0..255 可视范围后写出。
func saveDiffPGM(path string, a, b *raster.Image) error {
	diff, _ := raster.New(a.W, a.H)
	maxd := 1
	for i := range a.Pix {
		d := int(a.Pix[i]) - int(b.Pix[i])
		if d < 0 {
			d = -d
		}
		if d > maxd {
			maxd = d
		}
		diff.Pix[i] = uint8(d)
	}
	if maxd > 1 {
		for i := range diff.Pix {
			diff.Pix[i] = uint8(int(diff.Pix[i]) * 255 / maxd)
		}
	}
	return diff.SavePGM(path)
}

func fmtPts(pts []raster.Sample) string {
	parts := make([]string, len(pts))
	for i, p := range pts {
		parts[i] = fmt.Sprintf("(%d,%d)", p.I, p.J)
	}
	return strings.Join(parts, " ")
}

func main() {
	outDir := filepath.Join("artifacts", "aa_sym")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "创建输出目录失败:", err)
		os.Exit(1)
	}

	var b strings.Builder
	printf := func(format string, args ...any) {
		fmt.Fprintf(&b, format, args...)
		fmt.Printf(format, args...)
	}

	tri := normalize(shape())
	mir := mirrorX(shape())
	rot := rotate180(shape())
	printf("画幅: %dx%d, 整图像素数: %d\n", imgW, imgH, imgW*imgH)
	printf("形状(三角形): %v\n", tri)
	printf("水平镜像:     %v\n", mir)
	printf("180°旋转:     %v\n\n", rot)

	for _, lyt := range layouts() {
		if err := lyt.ss.Validate(); err != nil {
			fmt.Fprintln(os.Stderr, "布局非法:", err)
			os.Exit(1)
		}
		printf("=== %s ===\n", lyt.name)
		printf("点位(%d个, Grid=%d): %s\n", len(lyt.ss.Pts), lyt.ss.Grid, fmtPts(lyt.ss.Pts))

		mEq, mL, mR := lyt.ss.MirrorSymmetric()
		printf("镜像集合等式 S = M(S): %v\n", mEq)
		if !mEq {
			printf("  S\\\\M(S) = {%s}\n", fmtPts(mL))
			printf("  M(S)\\\\S = {%s}\n", fmtPts(mR))
		}
		cEq, cL, cR := lyt.ss.CentralSymmetric()
		printf("中心集合等式 S = R(S): %v\n", cEq)
		if !cEq {
			printf("  S\\\\R(S) = {%s}\n", fmtPts(cL))
			printf("  R(S)\\\\S = {%s}\n", fmtPts(cR))
		}

		orig := render(tri, lyt.ss)
		mirIm := flipH(render(mir, lyt.ss))
		mst := compare(orig, mirIm)
		printf("镜像偏差: 最大灰度差=%d, 灰度差之和=%d, 差异像素=%d (%.4f%%)\n",
			mst.maxAbs, mst.l1, mst.nDiff, mst.pct)

		rotIm := flip180(render(rot, lyt.ss))
		rst := compare(orig, rotIm)
		printf("旋转偏差: 最大灰度差=%d, 灰度差之和=%d, 差异像素=%d (%.4f%%)\n",
			rst.maxAbs, rst.l1, rst.nDiff, rst.pct)

		base := filepath.Join(outDir, lyt.name)
		if err := orig.SavePGM(base + "_orig.pgm"); err != nil {
			panic(err)
		}
		if err := mirIm.SavePGM(base + "_mirror.pgm"); err != nil {
			panic(err)
		}
		if err := saveDiffPGM(base+"_diff.pgm", orig, mirIm); err != nil {
			panic(err)
		}
		printf("写出: %s_orig.pgm, %s_mirror.pgm, %s_diff.pgm\n\n",
			lyt.name, lyt.name, lyt.name)
	}

	if err := os.WriteFile(filepath.Join(outDir, "results.txt"),
		[]byte(b.String()), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "写 results.txt 失败:", err)
		os.Exit(1)
	}
}
