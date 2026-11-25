// cullcheck 验证包围球剔除的保守性：
//   - 保守球（r=D/2）相对逐三角形精确判定，误杀数必须为零，逐像素完全一致；
//   - 缩水球（r=max(w,h)/2，r/D<0.5）作为缺陷对照，复现边界误杀。
//
// 三组场景输出误杀数、漏杀样例编号、剔除率与耗时。
package main

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"time"

	"vecgo/cull"
	"vecgo/geom"
	"vecgo/raster"
)

const (
	imgW, imgH  = 128, 128
	supersample = 2
	repeats     = 20
)

var view = cull.View{MinX: 0, MinY: 0, MaxX: 100, MaxY: 100}
var viewArr = [4]float64{0, 0, 100, 100}

type modeResult struct {
	name         string
	culled       int
	falseKills   int   // 球剔除但精确可见
	missedCulls  []int // 球保留但精确不可见（漏杀）
	falseKillIDs []int
	pixelDiffs   int
	ns           int64 // 剔除判定平均耗时
}

// gridScene：规则网格。内部 10×10 共 200 个全可见三角形；
// 外环 44 个完全在视窗外的三角形，供剔除路径发挥。
func gridScene() [][]geom.Point {
	var tris [][]geom.Point
	g := 10.0
	// 内部：(0,0)-(100,100)，每格两个三角形
	for i := 0; i < 10; i++ {
		for j := 0; j < 10; j++ {
			x0, y0 := float64(i)*g, float64(j)*g
			tris = append(tris,
				[]geom.Point{{X: x0, Y: y0}, {X: x0 + g, Y: y0}, {X: x0, Y: y0 + g}},
				[]geom.Point{{X: x0 + g, Y: y0}, {X: x0 + g, Y: y0 + g}, {X: x0, Y: y0 + g}},
			)
		}
	}
	// 下/上外环（各 11 个）
	for i := 0; i < 11; i++ {
		x := float64(i) * g
		tris = append(tris,
			[]geom.Point{{X: x, Y: -20}, {X: x + g, Y: -20}, {X: x, Y: -10}},
			[]geom.Point{{X: x, Y: 110}, {X: x + g, Y: 110}, {X: x, Y: 120}},
		)
	}
	// 左/右外环（各 11 个）
	for j := 0; j < 11; j++ {
		y := float64(j) * g
		tris = append(tris,
			[]geom.Point{{X: -20, Y: y}, {X: -20, Y: y + g}, {X: -10, Y: y}},
			[]geom.Point{{X: 110, Y: y}, {X: 110, Y: y + g}, {X: 120, Y: y}},
		)
	}
	return tris
}

// edgeScene：四边边界条带 + 四角细长三角形 + 完全在角外的小片 + 保证漏杀样例。
func edgeScene() [][]geom.Point {
	var tris [][]geom.Point
	add := func(pts ...geom.Point) { tris = append(tris, pts) }
	// 贴四边的边界三角形，每条边 12 个，共 48 个（多数与视窗相交）
	for k := 0; k < 12; k++ {
		x := float64(k)*8 + 2
		add(geom.Point{X: x, Y: -4}, geom.Point{X: x + 5, Y: -4}, geom.Point{X: x + 2, Y: 3})    // 下
		add(geom.Point{X: x, Y: 104}, geom.Point{X: x + 5, Y: 104}, geom.Point{X: x + 2, Y: 97}) // 上
		add(geom.Point{X: -4, Y: x}, geom.Point{X: -4, Y: x + 5}, geom.Point{X: 3, Y: x + 2})    // 左
		add(geom.Point{X: 104, Y: x}, geom.Point{X: 104, Y: x + 5}, geom.Point{X: 97, Y: x + 2}) // 右
	}
	// 四角的细长切角三角形：三顶点在视窗外、一条边切入角部（精确可见）
	corners := []struct{ sx, sy float64 }{
		{-1, -1}, {1, -1}, {-1, 1}, {1, 1},
	}
	for _, c := range corners {
		ox := (1 + c.sx) / 2 * 100 // sx=-1 -> 0，sx=1 -> 100
		oy := (1 + c.sy) / 2 * 100
		far := geom.Point{X: ox + c.sx*120, Y: oy + c.sy*120}
		pa := geom.Point{X: ox - c.sx*3, Y: oy + c.sy*2}
		pb := geom.Point{X: ox + c.sx*2, Y: oy - c.sy*3}
		add(far, pa, pb)
	}
	// 角外小片：完全在视窗外，离角点很近（漏杀候选，精确不可见）
	for _, c := range corners {
		bx := (1 + c.sx) / 2 * 100
		by := (1 + c.sy) / 2 * 100
		add(
			geom.Point{X: bx + c.sx*12, Y: by + c.sy*12},
			geom.Point{X: bx + c.sx*5, Y: by + c.sy*12},
			geom.Point{X: bx + c.sx*12, Y: by + c.sy*5},
		)
	}
	// 保证漏杀样例：三角形本体全在视窗外，但包围球（r=D/2）压进视窗，
	// 球判定保留、精确判定不可见。
	// 下方扁三角：盒 (-2,-12)-(70,-2)，球顶 = -2 + D/2 > 0
	add(geom.Point{X: -2, Y: -12}, geom.Point{X: 70, Y: -12}, geom.Point{X: 70, Y: -2})
	// 左侧扁三角：盒 (-12,30)-(-2,90)，球右缘 > 0
	add(geom.Point{X: -12, Y: 30}, geom.Point{X: -2, Y: 30}, geom.Point{X: -2, Y: 90})
	return tris
}

// randomScene：随机三角形（含跨边界）+ 随机细长切角三角形。
func randomScene(rng *rand.Rand) [][]geom.Point {
	var tris [][]geom.Point
	for n := 0; n < 400; n++ {
		cx, cy := rng.Float64()*120-10, rng.Float64()*120-10
		s := rng.Float64()*12 + 2
		tris = append(tris, []geom.Point{
			{X: cx + rng.NormFloat64()*s, Y: cy + rng.NormFloat64()*s},
			{X: cx + rng.NormFloat64()*s, Y: cy + rng.NormFloat64()*s},
			{X: cx + rng.NormFloat64()*s, Y: cy + rng.NormFloat64()*s},
		})
	}
	// 30 个随机细长切角三角形，随机落在四角附近，产生误杀/漏杀样本
	for n := 0; n < 30; n++ {
		sx := []float64{-1, 1}[rng.Intn(2)]
		sy := []float64{-1, 1}[rng.Intn(2)]
		ox := (1 + sx) / 2 * 100
		oy := (1 + sy) / 2 * 100
		reach := rng.Float64()*8 + 0.5 // 切入视窗 0.5~8.5
		side := rng.Float64()*10 + 3
		far := geom.Point{X: ox + sx*(rng.Float64()*80+40), Y: oy + sy*(rng.Float64()*80+40)}
		pa := geom.Point{X: ox - sx*reach, Y: oy + sy*side}
		pb := geom.Point{X: ox + sx*side, Y: oy - sy*reach}
		tris = append(tris, []geom.Point{far, pa, pb})
	}
	// 4 个确定性角部细长三角形：精确可见；保守球保留，缩水球必误杀（r/D≈0.354）。
	// 几何与 cull_test 的误杀样例同形，分别置于四角。
	for _, c := range corners4 {
		ox := (1 + c.sx) / 2 * 100
		oy := (1 + c.sy) / 2 * 100
		tris = append(tris, []geom.Point{
			{X: ox + c.sx*23, Y: oy + c.sy*23}, // 远顶点（视窗外）
			{X: ox - c.sx*3, Y: oy + c.sy*2},   // 邻点 a
			{X: ox + c.sx*2, Y: oy - c.sy*3},   // 邻点 b
		})
	}
	return tris
}

var corners4 = []struct{ sx, sy float64 }{
	{-1, -1}, {1, -1}, {-1, 1}, {1, 1},
}

func render(active []bool, tris [][]geom.Point) *raster.Image {
	im, err := raster.New(imgW, imgH)
	if err != nil {
		panic(err)
	}
	if err := im.FillView(tris, active, viewArr, supersample); err != nil {
		panic(err)
	}
	return im
}

func timeCull(spheres []cull.Sphere, strict bool) (int64, int) {
	var culled int
	start := time.Now()
	for rep := 0; rep < repeats; rep++ {
		culled = 0
		for _, s := range spheres {
			if s.Culled(view, strict) {
				culled++
			}
		}
	}
	return time.Since(start).Nanoseconds() / int64(repeats), culled
}

func runMode(name string, spheres []cull.Sphere, exact []bool, ref *raster.Image, tris [][]geom.Point, strict bool) modeResult {
	ns, culled := timeCull(spheres, strict)
	active := make([]bool, len(tris))
	res := modeResult{name: name, culled: culled, ns: ns}
	for i := range tris {
		cullOut := spheres[i].Culled(view, strict)
		active[i] = !cullOut
		if cullOut && exact[i] {
			res.falseKills++
			res.falseKillIDs = append(res.falseKillIDs, i)
		}
		if !cullOut && !exact[i] {
			res.missedCulls = append(res.missedCulls, i)
		}
	}
	im := render(active, tris)
	for i := range ref.Pix {
		if im.Pix[i] != ref.Pix[i] {
			res.pixelDiffs++
		}
	}
	return res
}

func evalScene(name string, tris [][]geom.Point) {
	n := len(tris)
	bounds := make([]cull.Bounds, n)
	cons := make([]cull.Sphere, n)
	loose := make([]cull.Sphere, n)
	exact := make([]bool, n)
	exactStart := time.Now()
	for i, tri := range tris {
		bounds[i] = cull.TriBounds(tri)
		cons[i] = cull.ConservativeSphere(bounds[i])
		loose[i] = cull.LooseSphere(bounds[i])
		exact[i] = cull.ExactVisible(tri, view, imgW, imgH, supersample)
	}
	exactNS := time.Since(exactStart).Nanoseconds()
	exactVisibleCount := 0
	for _, v := range exact {
		if v {
			exactVisibleCount++
		}
	}

	// 参考图：全部精确可见三角形
	refActive := make([]bool, n)
	for i, v := range exact {
		refActive[i] = v
	}
	ref := render(refActive, tris)

	rCons := runMode("保守球 r=D/2", cons, exact, ref, tris, false)
	rLoose := runMode("缩水球 r=max(w,h)/2", loose, exact, ref, tris, true)

	// r/D 统计（缩水球）
	var minRatio, sumRatio float64 = 1, 0
	for _, b := range bounds {
		r := cull.LooseSphere(b).R / b.Diagonal()
		if r < minRatio {
			minRatio = r
		}
		sumRatio += r
	}

	fmt.Printf("================ 场景 %s：%d 个三角形，精确可见 %d 个 ================\n", name, n, exactVisibleCount)
	fmt.Printf("逐三角形精确判定总耗时: %d ns（平均 %.1f ns/三角形）\n", exactNS, float64(exactNS)/float64(n))
	for _, r := range []modeResult{rCons, rLoose} {
		fmt.Printf("--- %s ---\n", r.name)
		fmt.Printf("  剔除 %d 个，剔除率 %.2f%%，判定耗时 %d ns（%.1f ns/三角形，相对精确判定加速 %.0fx）\n",
			r.culled, 100*float64(r.culled)/float64(n), r.ns,
			float64(r.ns)/float64(n), float64(exactNS)/float64(r.ns))
		fmt.Printf("  误杀数 %d（球剔除但精确可见），逐像素差异 %d\n", r.falseKills, r.pixelDiffs)
		if len(r.falseKillIDs) > 0 {
			fmt.Printf("  误杀三角形编号: %v\n", sorted(r.falseKillIDs))
		}
		fmt.Printf("  漏杀数 %d（球保留但精确不可见）\n", len(r.missedCulls))
		if len(r.missedCulls) > 0 {
			fmt.Printf("  漏杀三角形编号: %v\n", sorted(r.missedCulls))
		}
	}
	fmt.Printf("缩水球 r/D：最小 %.4f，均值 %.4f（保守性条件要求 r/D ≥ 0.5）\n\n",
		minRatio, sumRatio/float64(n))
}

func sorted(ids []int) []int {
	out := append([]int{}, ids...)
	sort.Ints(out)
	return out
}

func main() {
	// 数学推导的数值核验：保守球 r/D 恒为 0.5
	b := cull.Bounds{MinX: 0, MinY: 0, MaxX: 4, MaxY: 3}
	fmt.Printf("保守性数值核验: D=%.4f, r=%.4f, r/D=%.4f\n\n",
		b.Diagonal(), cull.ConservativeSphere(b).R, cull.ConservativeSphere(b).R/b.Diagonal())

	evalScene("A 规则网格", gridScene())
	evalScene("B 边界/角部压力", edgeScene())
	evalScene("C 随机+细长切角", randomScene(rand.New(rand.NewSource(42))))

	// 防止编译器优化掉结果
	if math.Abs(view.MinX-view.MaxX) < 0 {
		fmt.Println("unreachable")
	}
}
