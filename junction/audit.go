package junction

import (
	"fmt"
	"math"
)

// InkBrightness 是周围正常填充的参考灰度（0..255）。
// 取 200 而不是 255，好让“三份全量索赔直接相加”在截断前明显溢出。
const InkBrightness = 200.0

// Junction 是一个待核的三边接点实例。
type Junction struct {
	ID     string
	Name   string
	Angles Angles
}

// Degrees 返回角度制三元组，仅供报表展示。
func (j Junction) Degrees() [3]float64 {
	var d [3]float64
	for i, a := range j.Angles {
		d[i] = a * 180 / math.Pi
	}
	return d
}

// Class 是报表指定的三类标准接点角度：
//  1. eq-y   等角 Y 形      120/120/120
//  2. t      直角 T 形       90/90/180
//  3. acute  锐角偏置分叉     60/90/210（含一个优角扇区）
//
// 等角类是“除以 3 恰好成立”的对照组；后两类给出非对称反例。
func Classes() []Junction {
	return []Junction{
		{ID: "eq-y", Name: "等角Y形 120/120/120", Angles: MustAnglesDeg([3]float64{120, 120, 120})},
		{ID: "t", Name: "直角T接 90/90/180", Angles: MustAnglesDeg([3]float64{90, 90, 180})},
		{ID: "acute", Name: "锐角偏置 60/90/210", Angles: MustAnglesDeg([3]float64{60, 90, 210})},
	}
}

// ExtraJunctions 提供更多接点，用来证明“权重和为 1”不是三个样板点上的巧合。
func ExtraJunctions() []Junction {
	return []Junction{
		{ID: "scalene", Name: "不等角斜分 90/120/150", Angles: MustAnglesDeg([3]float64{90, 120, 150})},
		{ID: "narrow", Name: "双窄扇 45/105/210", Angles: MustAnglesDeg([3]float64{45, 105, 210})},
		{ID: "wide-reflex", Name: "极大优角 30/60/270", Angles: MustAnglesDeg([3]float64{30, 60, 270})},
	}
}

// SpotCheckJunctions 是一批确定性的非对称抽检接点（含近退化的 5°/5°/350°）。
func SpotCheckJunctions() []Junction {
	degs := [][3]float64{
		{80, 130, 150}, {10, 170, 180}, {100, 100, 160}, {5, 5, 350}, {72, 144, 144},
	}
	out := make([]Junction, 0, len(degs))
	for i, d := range degs {
		out = append(out, Junction{
			ID:     fmt.Sprintf("spot-%d", i+1),
			Name:   "非对称抽检",
			Angles: MustAnglesDeg(d),
		})
	}
	return out
}

// ThirdsGap 是“每个扇区统一除以 3 / 权重取 1/3”的反例数据。
type ThirdsGap struct {
	// 统一权重 1/3 作用在守恒总预算 TotalArea 上时，每个扇区分到的面积。
	UniformArea Areas
	// 守恒要求每个扇区拿到的真实面积。
	RequiredArea Areas
	// 错配面积 = UniformArea - RequiredArea；正=多占（过亮），负=缺口（欠覆盖）。
	MisallocArea Areas
	// 每个扇区的亮度偏差（0..255 灰度阶，相对参考墨色 Ink）。
	BrightnessDelta [3]float64
	// 最大“缺口”面积（只取负错配的绝对值）。
	MaxDeficitArea float64
	// 缺口扇区对应的最大欠亮阶数（正数表示暗多少阶）。
	WorstDarkLevels float64
	// 过亮扇区的最大过亮阶数。
	WorstBrightLevels float64

	// LiteralGapArea 是字面意义“把每个扇区面积各自除以 3”后丢失的面积：
	// ΣA_i - Σ(A_i/3) = 2/3 · ΣA。
	LiteralGapArea float64
	// LiteralGapFraction 是丢失面积占总覆盖的比例，恒为 2/3。
	LiteralGapFraction float64
	// LiteralDarkLevels 是均摊到整个接点环上的欠亮阶数。
	LiteralDarkLevels float64
}

// EqualThirdsGap 用给定接点的真实扇区面积生成“除以 3”反例数据。
// radius 是扇区半径（像素），ink 是参考灰度（0..255）。
func EqualThirdsGap(j Junction, radius, ink float64) (ThirdsGap, error) {
	ar, err := SectorAreas(j.Angles, radius)
	if err != nil {
		return ThirdsGap{}, err
	}
	if ink < 0 || ink > 255 {
		return ThirdsGap{}, fmt.Errorf("灰度必须在 0..255，实际 %g", ink)
	}
	total := SumAreas(ar)

	var g ThirdsGap
	litKept := 0.0
	for i := 0; i < 3; i++ {
		g.RequiredArea[i] = ar[i]
		// 权重 1/3 分到的公共预算。
		g.UniformArea[i] = total / 3
		g.MisallocArea[i] = g.UniformArea[i] - ar[i]
		// 扇区单位面积亮度不变，分到的面积比例偏差即亮度比例偏差。
		g.BrightnessDelta[i] = ink * ((1.0/3.0)/ConservationWeightsMust(j.Angles)[i] - 1.0)
		if g.MisallocArea[i] < 0 {
			if d := -g.MisallocArea[i]; d > g.MaxDeficitArea {
				g.MaxDeficitArea = d
			}
			if lv := -g.BrightnessDelta[i]; lv > g.WorstDarkLevels {
				g.WorstDarkLevels = lv
			}
		} else if lv := g.BrightnessDelta[i]; lv > g.WorstBrightLevels {
			g.WorstBrightLevels = lv
		}
		litKept += ar[i] / 3
	}
	g.LiteralGapArea = total - litKept
	g.LiteralGapFraction = g.LiteralGapArea / total
	g.LiteralDarkLevels = ink * g.LiteralGapFraction
	return g, nil
}

// ConservationWeightsMust 是报表/测试内部使用的无错版本（角度已在 Junction 构造时校验）。
func ConservationWeightsMust(a Angles) Weights {
	w, err := ConservationWeights(a)
	if err != nil {
		panic(err)
	}
	return w
}

// RingPixel 是接点周围一圈离散像素（角采样）的亮度核对记录。
type RingPixel struct {
	Index             int
	AngleDeg          float64
	Sector            int
	SurroundGray      uint8
	CorrectedGray     uint8
	NaiveAddGray      uint8
	ThirdsLiteralGray uint8
	// ThirdsUniformGray 是“权重一律取 1/3、作用于守恒总预算”后，
	// 按扇区单位面积亮度折算回该扇区像素的灰度（非对称角度下各扇区不同）。
	ThirdsUniformGray uint8
}

// RingAudit 是一个接点的逐像素亮度核对结果。
type RingAudit struct {
	JunctionID string
	Pixels     []RingPixel

	// 面积权重修正后，与周围参考灰度的最大偏差（灰度阶）。理论为 0。
	CorrectedMaxDelta int
	// 浮点台账偏差（未量化），衡量 Σw=1 的精确程度。
	LedgerMaxFloatDelta float64
	// 朴素三份相加的最大灰度（截断后）与相对周围的最大上偏。
	NaiveMaxGray      uint8
	NaiveMaxOvershoot int
	// “字面把每个扇区除以 3”造成的最大欠亮阶数。
	ThirdsLiteralMaxUndershoot int
	// “统一权重 1/3”在最宽扇区造成的最大欠亮阶数、最窄扇区的过亮阶数。
	ThirdsUniformMaxUndershoot int
	ThirdsUniformMaxOvershoot  int
}

// AuditRing 把接点环离散成 n 个等角像素，逐像素核对三种台账：
//
//   - 周围参考：普通填充像素灰度 ink；
//   - 面积权重修正：三个图元的全量接点索赔按 w 加权，灰度 = ink·Σw = ink；
//   - 朴素相加：三份全量索赔直接相加后截断（亮一圈）；
//   - 字面除以三：扇区 i 只保留其真实亮度的 1/3，灰度 ink/3（留下 2/3 缺口）；
//   - 统一 1/3 权重：总量守恒但错配扇区，宽扇区欠亮、窄扇区过亮。
//
// 每个像素都独立给出记录，权重和为 1 的效果在每个像素上都可核对。
func AuditRing(j Junction, n int, ink float64) (RingAudit, error) {
	if n < 3 {
		return RingAudit{}, fmt.Errorf("环像素数至少 3，实际 %d", n)
	}
	if ink < 0 || ink > 255 {
		return RingAudit{}, fmt.Errorf("灰度必须在 0..255，实际 %g", ink)
	}
	w, err := ConservationWeights(j.Angles)
	if err != nil {
		return RingAudit{}, err
	}
	sumErr, err := CheckWeightSum(w)
	if err != nil {
		return RingAudit{}, err
	}

	surround := uint8(math.Round(ink))
	naive := uint8(math.Min(255, math.Round(3*ink)))
	thirdsLit := uint8(math.Round(clamp(ink/3, 0, 255)))

	px := make([]RingPixel, n)
	au := RingAudit{JunctionID: j.ID, Pixels: px, NaiveMaxGray: naive}
	au.LedgerMaxFloatDelta = math.Abs(ink*SumWeights(w) - ink)
	_ = sumErr

	cum := [3]float64{j.Angles[0], j.Angles[0] + j.Angles[1], Tau}
	for k := 0; k < n; k++ {
		phi := Tau * (float64(k) + 0.5) / float64(n)
		sec := 0
		if phi >= cum[0] {
			sec = 1
		}
		if phi >= cum[1] {
			sec = 2
		}
		// 面积权重组合：因为 Σw=1，结果恒等于 ink。
		corrected := uint8(math.Round(clamp(ink*SumWeights(w), 0, 255)))
		// 统一 1/3 权重折算回扇区单位面积亮度：ink · (1/3)/w_sec。
		uniform := uint8(math.Round(clamp(ink*(1.0/3.0)/w[sec], 0, 255)))
		px[k] = RingPixel{
			Index: k, AngleDeg: phi * 180 / math.Pi, Sector: sec,
			SurroundGray: surround, CorrectedGray: corrected,
			NaiveAddGray: naive, ThirdsLiteralGray: thirdsLit,
			ThirdsUniformGray: uniform,
		}
		if d := absInt(int(corrected) - int(surround)); d > au.CorrectedMaxDelta {
			au.CorrectedMaxDelta = d
		}
		if d := int(naive) - int(surround); d > au.NaiveMaxOvershoot {
			au.NaiveMaxOvershoot = d
		}
		if d := int(surround) - int(thirdsLit); d > au.ThirdsLiteralMaxUndershoot {
			au.ThirdsLiteralMaxUndershoot = d
		}
		if d := int(surround) - int(uniform); d > au.ThirdsUniformMaxUndershoot {
			au.ThirdsUniformMaxUndershoot = d
		}
		if d := int(uniform) - int(surround); d > au.ThirdsUniformMaxOvershoot {
			au.ThirdsUniformMaxOvershoot = d
		}
	}
	return au, nil
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
