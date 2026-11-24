// Package scaleaudit 回答一个问题：同一个形状在五个缩放档上取「同一条边」，
// 边缘像素的灰度变化是否满足覆盖率的单调性，以及与「按面积比 s²」给出的预测
// 差在哪里、在哪一档、哪个像素位置被破坏。
//
// 设计要点：
//
//   - 覆盖率 C = hit / ss² 是「像素内被覆盖的面积占比」，因此均匀缩放 s 时，
//     一个在像素内作二维相似伸缩的覆盖块面积按 s² 变化，预测灰度用面积比
//     (s/s0)²，而不是边长比 s/s0。
//   - 对固定像素与单调平移的直边（或关于锚点嵌套的凸形），子样本点只会逐个
//     从外翻到内，hit 与 uint8 灰度关于 s 单调不减。本包同时对全图逐像素
//     验证这条不等式（FixedPairs）。
//   - 「同一条边、每档取扫描线穿越所在像素」取到的是随缩放移动的不同像素，
//     叠加 2×2 量化后，这一列代表灰度可能出现反向跳变；Breaks 精确定位到
//     相邻档位与两个像素坐标。
package scaleaudit

import (
	"fmt"
	"math"

	"vecgo/geom"
	"vecgo/pipe"
	"vecgo/raster"
)

// 固定实验夹具：64×64 画幅、2×2 超采样（与 config.Default 一致），
// 固定世界视口 [-3,13]² → 64 像素，即 PPU=4。
const (
	Width       = 64
	Height      = 64
	Supersample = 2
	ViewMinX    = -3.0
	ViewMinY    = -3.0
	PPU         = 4.0
	EdgeIndex   = 1 // 方形顶点 (10,0)->(10,10) 这条边
	ScanRow     = 21
	AnchorScale = 1.0
	HalfScale   = 0.5
)

// Scales 按题目叙事从全尺寸到一半排列（档1..档5）。
func Scales() []float64 { return []float64{1.0, 0.875, 0.75, 0.625, 0.5} }

// Angle 是审计夹具的固定转角 13.625°。选这个确定值是因为在固定扫描行
// ScanRow=21 上，边与全部五档扫描线都严格交于线段内部，且五档穿越像素恰好
// 给出一列全为抗锯齿中间灰、并含一次可定位反升的样本（数值由
// scaleaudit_test.go 锁定，换相邻角度会在若干扫描行上得到同类反升）。
// 该边接近 4:1 斜边（tan≈0.2423，斜率≈-4.13）。
func Angle() float64 { return 13.625 * math.Pi / 180.0 }

// FixtureSquare 是与 cmd 默认场景相同的 10×10 方形。
func FixtureSquare() []geom.Point {
	return []geom.Point{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}
}

func view() raster.View { return raster.View{MinX: ViewMinX, MinY: ViewMinY, PPU: PPU} }

// Level 是一个缩放档上、同一条边在固定扫描行处的测量与预测。
type Level struct {
	Index      int     // 0-based 档位（按 Scales 顺序，全尺寸→一半）
	Dang       int     // 1-based 档位，便于阅读
	Scale      float64 // s
	Col        int     // 该档边与扫描行交点落入的像素列
	Row        int     // 固定扫描行
	Hits       int     // 命中子样本数
	Den        int     // 子样本总数 ss²
	Gray       int     // 实际 uint8 灰度 255*Hits/Den
	PredRaw    float64 // 面积比预测（未取整），以 AnchorScale 档实测灰度为基准
	PredGray   int     // PredRaw 四舍五入并夹到 [0,255]
	LinearRaw  float64 // 对照：错误的边长比预测（仅用于说明，不进主预测列）
	LinearGray int
}

// Break 是缩小方向单调性 G[i] >= G[i+1] 被破坏的一对相邻档。
type Break struct {
	Pair               int // i：第 i 档与第 i+1 档之间（0-based）
	ScaleFrom, ScaleTo float64
	PixelFrom          [2]int // 较大缩放档的穿越像素 (col,row)
	GrayFrom           int
	PixelTo            [2]int // 较小缩放档的穿越像素 (col,row)
	GrayTo             int
	Rise               int    // GrayTo-GrayFrom（>0 即缩小却变亮，破坏）
	Location           string // 人话定位
}

// FixedPair 记录相邻两档在「同一固定像素」上的全图单调性核对。
// Scales 按全尺寸→一半降序排列，A 档尺度大于 B 档，因此嵌套缩放下应有
// Pix[B] <= Pix[A]：变暗/不变都正常，B 反而更亮才算破坏。
type FixedPair struct {
	I                  int
	ScaleA, ScaleB     float64
	DarkerPixels       int // Pix[B] < Pix[A]：缩小后覆盖变少，符合预期
	EqualPixels        int
	BrighterViolations int // Pix[B] > Pix[A]：嵌套缩放下应为 0
}

// EdgeSpot 是沿同一条边、另一条扫描行上发现的单调性破坏位置。
type EdgeSpot struct {
	Row   int
	Break Break
}

// Neighborhood 是破坏档位附近一行像素的灰度切片，用于展示边移动。
type Neighborhood struct {
	Scale float64
	Row   int
	Cols  []int
	Grays []int
}

// Report 是完整审计结果。
type Report struct {
	AngleDeg float64
	Levels   []Level
	Breaks   []Break
	// EdgePartialBreaks：同一条边上、相邻两档取到的都是抗锯齿中间灰
	// (0<G<255) 时的缩小反升位置——这才是题目所说「边缘像素灰度」的破坏。
	EdgePartialBreaks []EdgeSpot
	// ProbeJumps：扫描线穿越探针落到纯内/外像素（含 0 或 255）时的反升，
	// 属于探针身份/二值跳变，单列以免与抗锯齿边缘灰混淆。
	ProbeJumps   []EdgeSpot
	FixedPairs   []FixedPair
	Neighborhood []Neighborhood
	Patch        *PatchCheck
	// Images 是五档原始覆盖率灰度图（未经 shading/gamma/blur），供导出 PGM。
	// 不进 JSON。
	Images []*raster.Image `json:"-"`
}

// AreaGray 返回面积比预测：基准灰度 g0 在 s0 档，预测 s 档为 g0*(s/s0)²。
func AreaGray(g0 int, s, s0 float64) float64 {
	r := s / s0
	return float64(g0) * r * r
}

// LinearGray 是被否定的边长比预测 g0*(s/s0)，仅作对照。
func LinearGray(g0 int, s, s0 float64) float64 {
	return float64(g0) * (s / s0)
}

// QuantizePred 与渲染器一致地夹到 [0,255]，再四舍五入到整数灰度。
func QuantizePred(v float64) int {
	if math.IsNaN(v) || v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return int(math.Round(v))
}

// ShrinkBreaks 在「全尺寸→一半」的序列上找破坏：缩小却变亮 G[i+1] > G[i]。
// 返回破坏对的下标 i（levels[i] -> levels[i+1]）。
func ShrinkBreaks(grays []int) []int {
	var out []int
	for i := 0; i+1 < len(grays); i++ {
		if grays[i+1] > grays[i] {
			out = append(out, i)
		}
	}
	return out
}

// CrossingPixel 求第 edge 条边与「第 row 行像素中心扫描线」的交点所落入的像素列。
// ok=false 表示交点不在线段上或落出画幅。
func CrossingPixel(poly []geom.Point, edge, row int, v raster.View) (col int, t float64, x, y float64, ok bool) {
	a := poly[edge]
	b := poly[(edge+1)%len(poly)]
	y = v.WorldY(float64(row) + 0.5)
	if math.Abs(b.Y-a.Y) < 1e-12 {
		return 0, 0, 0, y, false
	}
	t = (y - a.Y) / (b.Y - a.Y)
	if t < -1e-9 || t > 1+1e-9 {
		return 0, t, 0, y, false
	}
	x = a.X + (b.X-a.X)*t
	col = int(math.Floor((x - v.MinX) * v.PPU))
	if col < 0 || col >= Width {
		return col, t, x, y, false
	}
	return col, t, x, y, true
}

// Build 用固定夹具渲染五档并完成全部审计。
func Build() (*Report, error) {
	scales := Scales()
	v := view()
	angle := Angle()

	imgs := make([]*raster.Image, len(scales))
	polys := make([][]geom.Point, len(scales))
	levels := make([]Level, len(scales))
	hitsAll := make([][]int, len(scales))
	den := Supersample * Supersample

	var anchorGray int
	for i, s := range scales {
		poly := pipe.Transform(FixtureSquare(), s, angle)
		polys[i] = poly
		im, err := raster.New(Width, Height)
		if err != nil {
			return nil, err
		}
		hits, d, err := im.CountView([][]geom.Point{poly}, Supersample, v)
		if err != nil {
			return nil, err
		}
		den = d
		hitsAll[i] = hits
		if err := im.FillView([][]geom.Point{poly}, Supersample, v); err != nil {
			return nil, err
		}
		imgs[i] = im

		col, _, _, _, ok := CrossingPixel(poly, EdgeIndex, ScanRow, v)
		if !ok {
			return nil, fmt.Errorf("scale=%v 扫描行 %d 未与边有效相交", s, ScanRow)
		}
		g := int(im.Pix[ScanRow*Width+col])
		if math.Abs(s-AnchorScale) < 1e-12 {
			anchorGray = g
		}
		levels[i] = Level{Index: i, Dang: i + 1, Scale: s, Col: col, Row: ScanRow,
			Hits: hits[ScanRow*Width+col], Den: den, Gray: g}
	}

	// 以全尺寸档为基准补面积比 / 边长比预测列。
	for i := range levels {
		l := &levels[i]
		l.PredRaw = AreaGray(anchorGray, l.Scale, AnchorScale)
		l.PredGray = QuantizePred(l.PredRaw)
		l.LinearRaw = LinearGray(anchorGray, l.Scale, AnchorScale)
		l.LinearGray = QuantizePred(l.LinearRaw)
	}

	graySeq := make([]int, len(levels))
	for i, l := range levels {
		graySeq[i] = l.Gray
	}
	var breaks []Break
	for _, i := range ShrinkBreaks(graySeq) {
		br := makeBreak(i, levels)
		breaks = append(breaks, br)
	}

	// 沿同一条边扫所有扫描行，收集额外破坏位置，并区分抗锯齿中间灰与二值跳变。
	partialSpots := []EdgeSpot{}
	binaryJumps := []EdgeSpot{}
	for row := 0; row < Height; row++ {
		cols := make([]int, len(scales))
		gs := make([]int, len(scales))
		valid := true
		for i := range scales {
			col, _, _, _, ok := CrossingPixel(polys[i], EdgeIndex, row, v)
			if !ok {
				valid = false
				break
			}
			cols[i] = col
			gs[i] = int(imgs[i].Pix[row*Width+col])
		}
		if !valid {
			continue
		}
		for _, i := range ShrinkBreaks(gs) {
			br := Break{
				Pair: i, ScaleFrom: scales[i], ScaleTo: scales[i+1],
				PixelFrom: [2]int{cols[i], row}, GrayFrom: gs[i],
				PixelTo: [2]int{cols[i+1], row}, GrayTo: gs[i+1],
				Rise: gs[i+1] - gs[i],
			}
			br.Location = locate(br)
			spot := EdgeSpot{Row: row, Break: br}
			if isPartialEdgeGray(gs[i]) && isPartialEdgeGray(gs[i+1]) {
				partialSpots = append(partialSpots, spot)
			} else {
				binaryJumps = append(binaryJumps, spot)
			}
		}
	}

	// 固定像素全图单调性：相邻档（大尺度→小尺度）逐像素比较，B 比 A 亮即破坏。
	fixed := []FixedPair{}
	for i := 0; i+1 < len(scales); i++ {
		fp := FixedPair{I: i, ScaleA: scales[i], ScaleB: scales[i+1]}
		a, b := imgs[i].Pix, imgs[i+1].Pix
		for k := range a {
			switch {
			case b[k] < a[k]:
				fp.DarkerPixels++
			case b[k] == a[k]:
				fp.EqualPixels++
			default:
				fp.BrighterViolations++
			}
		}
		fixed = append(fixed, fp)
	}

	// 破坏处邻域切片（破坏对的两档，取 ScanRow 上 cols 附近）。
	neigh := []Neighborhood{}
	if len(breaks) > 0 {
		i := breaks[0].Pair
		for _, idx := range []int{i, i + 1} {
			c0 := levels[idx].Col - 4
			cols := []int{}
			gs := []int{}
			for c := c0; c <= levels[idx].Col+3; c++ {
				if c < 0 || c >= Width {
					continue
				}
				cols = append(cols, c)
				gs = append(gs, int(imgs[idx].Pix[ScanRow*Width+c]))
			}
			neigh = append(neigh, Neighborhood{Scale: scales[idx], Row: ScanRow, Cols: cols, Grays: gs})
		}
	}

	patch, perr := buildPatchCheck()
	if perr != nil {
		return nil, perr
	}

	return &Report{
		AngleDeg:          angle * 180 / math.Pi,
		Levels:            levels,
		Breaks:            breaks,
		EdgePartialBreaks: partialSpots,
		ProbeJumps:        binaryJumps,
		FixedPairs:        fixed,
		Neighborhood:      neigh,
		Patch:             patch,
		Images:            imgs,
	}, nil
}

func isPartialEdgeGray(g int) bool { return g > 0 && g < 255 }

func makeBreak(i int, lv []Level) Break {
	a, b := lv[i], lv[i+1]
	br := Break{
		Pair: i, ScaleFrom: a.Scale, ScaleTo: b.Scale,
		PixelFrom: [2]int{a.Col, a.Row}, GrayFrom: a.Gray,
		PixelTo: [2]int{b.Col, b.Row}, GrayTo: b.Gray,
		Rise: b.Gray - a.Gray,
	}
	br.Location = locate(br)
	return br
}

func locate(br Break) string {
	return fmt.Sprintf("档%d(s=%.3f)像素(col=%d,row=%d)灰度%d → 档%d(s=%.3f)像素(col=%d,row=%d)灰度%d；缩小一档灰度反升 %d，破坏点在 %s 与 %s 之间，异常峰位于 s=%.3f 的 (col=%d,row=%d)",
		br.Pair+1, br.ScaleFrom, br.PixelFrom[0], br.PixelFrom[1], br.GrayFrom,
		br.Pair+2, br.ScaleTo, br.PixelTo[0], br.PixelTo[1], br.GrayTo,
		br.Rise,
		fmt.Sprintf("(col=%d,row=%d)", br.PixelFrom[0], br.PixelFrom[1]),
		fmt.Sprintf("(col=%d,row=%d)", br.PixelTo[0], br.PixelTo[1]),
		br.ScaleTo, br.PixelTo[0], br.PixelTo[1])
}
