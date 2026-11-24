package kernel

import (
	"math"

	"vecgo/raster"
)

// AreaTarget 是“核面积等于一”的目标值（几何面积 πab）。
const AreaTarget = 1.0

// FogLevel 是灰雾判定档数：偏离本侧纯色超过 2 档即计为灰雾像素。
const FogLevel = 2

// ProbeDistance 是平台探测列到边缘的法向距离（像素）。
const ProbeDistance = 1.5

// AxesInfo 是一组轴的可序列化快照。
type AxesInfo struct {
	Long          float64 `json:"long"`
	Short         float64 `json:"short"`
	AngleDeg      float64 `json:"angle_deg"`
	AxisRatio     float64 `json:"axis_ratio"`
	AxisProduct   float64 `json:"axis_product_ab"`
	GeometricArea float64 `json:"geometric_area_pi_ab"`
}

func axesInfo(e Ellipse) AxesInfo {
	return AxesInfo{
		Long:          e.Long,
		Short:         e.Short,
		AngleDeg:      e.Angle * 180 / math.Pi,
		AxisRatio:     e.Long / e.Short,
		AxisProduct:   e.AxisProduct(),
		GeometricArea: e.GeometricArea(),
	}
}

// GroupReport 是一组“变换 + 核”的完整对照结果。
type GroupReport struct {
	ID              string      `json:"id"`
	Label           string      `json:"label"`
	ThetaDeg        float64     `json:"theta_deg"`
	ScaleX          float64     `json:"scale_x"`
	ScaleY          float64     `json:"scale_y"`
	Jacobian        [4]float64  `json:"jacobian_abcd"`
	Det             float64     `json:"det"`
	SigmaLo         float64     `json:"sigma_lo"`
	SigmaHi         float64     `json:"sigma_hi"`
	Kappa           float64     `json:"kappa_sigma_ratio"`
	MajorAngleDeg   float64     `json:"major_angle_deg"`
	CorrectAxes     AxesInfo    `json:"correct_axes"`
	UsedAxes        AxesInfo    `json:"used_axes"`
	AreaTarget      float64     `json:"area_target"`
	AreaResidual    float64     `json:"area_residual"`
	ConservesArea   bool        `json:"conserves_area"`
	Tolerance       float64     `json:"conservation_tolerance"`
	EpsLong         float64     `json:"eps_long"`
	EpsShort        float64     `json:"eps_short"`
	AreaErrorBound  float64     `json:"area_error_bound"`
	BoundHolds      bool        `json:"measured_within_bound"`
	RatioUsed       float64     `json:"ratio_used"`
	RatioError      float64     `json:"ratio_error"`
	RatioErrorBound float64     `json:"ratio_error_bound"`
	RatioBoundHolds bool        `json:"ratio_measured_within_bound"`
	OffendingAxis   string      `json:"offending_axis"`
	GrayErrorBound  float64     `json:"gray_error_bound_1p5"`
	GrayBoundHolds  bool        `json:"gray_measured_within_bound"`
	Edge            EdgeMetrics `json:"edge"`

	Image *raster.Image `json:"-"`
}

// HeroBeforeAfter 是题目主场景“先转 45° 再缩一半”的修复前复现与修复后指引。
type HeroBeforeAfter struct {
	ThetaDeg float64    `json:"theta_deg"`
	Scale    float64    `json:"scale"`
	Jacobian [4]float64 `json:"jacobian_abcd"`
	Det      float64    `json:"det"`
	InvDet   float64    `json:"inverse_area_factor"`
	Before   struct {
		Axes AxesInfo    `json:"axes"`
		Edge EdgeMetrics `json:"edge"`
	} `json:"before_fix_raw_footprint"`
	AfterGroupID string        `json:"after_fix_group_id"`
	Image        *raster.Image `json:"-"`
}

// Report 是三组对照 + 主场景修复前后的全部结果。
type Report struct {
	Convention    string          `json:"area_convention"`
	FogLevel      int             `json:"fog_level"`
	ProbeDistance float64         `json:"probe_distance"`
	Groups        []*GroupReport  `json:"groups"`
	Hero          HeroBeforeAfter `json:"hero_rotate45_scalehalf"`
	ComboScan     []ComboRow      `json:"combo_scan"`
}

// ComboRow 是一个变换组合下“原始逆足迹核 vs 面积守恒核”的并列结果。
type ComboRow struct {
	Name        string      `json:"name"`
	ThetaDeg    float64     `json:"theta_deg"`
	ScaleX      float64     `json:"scale_x"`
	ScaleY      float64     `json:"scale_y"`
	Det         float64     `json:"det"`
	Kappa       float64     `json:"kappa"`
	RawAxes     AxesInfo    `json:"raw_inverse_footprint_axes"`
	RawAreaProd float64     `json:"raw_area_factor"`
	RawEdge     EdgeMetrics `json:"raw_edge"`
	Conserved   AxesInfo    `json:"conserved_axes"`
	ConsEdge    EdgeMetrics `json:"conserved_edge"`
}

// ComboScan 扫描几个自然的变换组合，分别用“未归一化的 J⁻¹ 足迹核”和
// “面积守恒反解核”渲染同一条直边，展示面积变化与灰雾的来源、以及守恒修复的效果。
func ComboScan() []ComboRow {
	specs := []struct {
		name          string
		theta, sx, sy float64
	}{
		{"只旋转45°", math.Pi / 4, 1, 1},
		{"只缩一半", 0, 0.5, 0.5},
		{"转45°再缩一半(题目)", math.Pi / 4, 0.5, 0.5},
		{"转45°+非均匀缩(0.5,1)", math.Pi / 4, 0.5, 1},
	}
	rows := make([]ComboRow, 0, len(specs))
	const edgeX = 31.5
	for _, sp := range specs {
		j := CompositeJacobian(sp.sx, sp.sy, sp.theta)
		lo, hi := j.SingularValues()
		// J⁻¹ 的奇异轴：长=1/σmin，短=1/σmax。
		raw := Ellipse{Long: 1 / lo, Short: 1 / hi, Angle: j.MajorAngle()}
		cons := ConservedForJacobian(j)

		imRaw, _ := RenderVerticalEdge(64, 64, raw, edgeX)
		emRaw := MeasureEdge(imRaw, raw, edgeX, FogLevel)
		imCons, _ := RenderVerticalEdge(64, 64, cons, edgeX)
		emCons := MeasureEdge(imCons, cons, edgeX, FogLevel)

		rows = append(rows, ComboRow{
			Name: sp.name, ThetaDeg: sp.theta * 180 / math.Pi,
			ScaleX: sp.sx, ScaleY: sp.sy, Det: j.Det(), Kappa: j.AxisRatio(),
			RawAxes: axesInfo(raw), RawAreaProd: raw.AxisProduct(), RawEdge: emRaw,
			Conserved: axesInfo(cons), ConsEdge: emCons,
		})
	}
	return rows
}

func buildGroup(id, label string, thetaDeg, sx, sy float64, used Ellipse, epsL, epsS, tol float64) *GroupReport {
	theta := thetaDeg * math.Pi / 180
	j := CompositeJacobian(sx, sy, theta)
	lo, hi := j.SingularValues()
	correct := ConservedEllipse(j.AxisRatio(), j.MajorAngle())

	edgeX := 31.5
	im, err := RenderVerticalEdge(64, 64, used, edgeX)
	if err != nil {
		panic(err) // 尺寸固定为正，不可能失败
	}
	em := MeasureEdge(im, used, edgeX, FogLevel)

	res := AreaResidual(used)
	bound := AreaErrorBound(epsL, epsS, correct.Long, correct.Short)
	kappa := j.AxisRatio()
	ratioUsed := used.Long / used.Short
	ratioErr := math.Abs(ratioUsed - kappa)
	ratioBound := RatioErrorBound(kappa, epsL, epsS, correct.Long, correct.Short)
	grayBound := GrayErrorBound(ProbeDistance, NormalRadius(used, 0)) + 0.5 // +0.5 为 uint8 四舍五入余量

	g := &GroupReport{
		ID: id, Label: label,
		ThetaDeg: thetaDeg, ScaleX: sx, ScaleY: sy,
		Jacobian: j.Slice(), Det: j.Det(),
		SigmaLo: lo, SigmaHi: hi,
		Kappa: kappa, MajorAngleDeg: j.MajorAngle() * 180 / math.Pi,
		CorrectAxes: axesInfo(correct), UsedAxes: axesInfo(used),
		AreaTarget: AreaTarget, AreaResidual: res,
		ConservesArea: res <= tol, Tolerance: tol,
		EpsLong: epsL, EpsShort: epsS,
		AreaErrorBound: bound, BoundHolds: res <= bound+1e-12,
		RatioUsed:       ratioUsed,
		RatioError:      ratioErr,
		RatioErrorBound: ratioBound,
		RatioBoundHolds: ratioErr <= ratioBound+1e-12,
		OffendingAxis:   OffendingAxis(used, correct.Long, correct.Short, 0.02),
		GrayErrorBound:  grayBound,
		GrayBoundHolds:  em.MaxSideDev <= grayBound+1e-9,
		Edge:            em,
		Image:           im,
	}
	return g
}

// DefaultReport 构造题目要求的三组变换组合：
//
//	G1 恒等基准（0°, 1×），守恒核；
//	G2 先转 45° 再均匀缩一半，面积守恒反解核（修复后）；
//	G3 先转 45° 再非均匀缩 (0.5,1)，并故意使用只放大长轴的失配核（负对照，守恒不成立）。
func DefaultReport() *Report {
	const tol = 1e-9

	// G1：恒等。
	j1 := CompositeJacobian(1, 1, 0)
	c1 := ConservedForJacobian(j1)
	g1 := buildGroup("G1", "恒等基准 0°/1×", 0, 1, 1, c1, 0, 0, tol)

	// G2：题目主变换 45° + 0.5 均匀缩放；κ=1，守恒轴为半径 1/√π 的圆核。
	th := 45.0
	j2 := CompositeJacobian(0.5, 0.5, math.Pi/4)
	c2 := ConservedEllipse(j2.AxisRatio(), 0) // 各向同性，朝向不影响结果
	g2 := buildGroup("G2", "转45°再缩一半·守恒核(修复)", th, 0.5, 0.5, c2, 0, 0, tol)

	// G3：真实各向异性变换 κ=2（被拉长又压扁），再注入单轴错误：
	// 长轴违规取成守恒解的 5 倍，短轴保持守恒值。面积残差=4；
	// 给定长轴偏差预算 εL=5a0（相对预算 β=5），面积上界 U=β=5，实测 4 落在界内。
	j3 := CompositeJacobian(0.5, 1.0, math.Pi/4)
	c3 := ConservedForJacobian(j3)
	bad := c3
	bad.Long = 5 * c3.Long
	g3 := buildGroup("G3", "转45°非均匀缩·错长轴(负对照)", th, 0.5, 1.0, bad, 5*c3.Long, 0, tol)

	// 主场景修复前复现：直接拿雅可比逆的像素足迹当核。
	// J=0.5R45，J⁻¹=2R(-45)，奇异轴 (2,2)，轴积面积因子=1/detJ=4，核过宽 → 灰雾。
	raw := Ellipse{Long: 2, Short: 2, Angle: 0}
	im, _ := RenderVerticalEdge(64, 64, raw, 31.5)
	em := MeasureEdge(im, raw, 31.5, FogLevel)
	hero := HeroBeforeAfter{
		ThetaDeg: 45, Scale: 0.5,
		Jacobian: j2.Slice(), Det: j2.Det(), InvDet: 1 / j2.Det(),
		Image: im,
	}
	hero.Before.Axes = axesInfo(raw)
	hero.Before.Edge = em
	hero.AfterGroupID = "G2"

	return &Report{
		Convention:    "椭圆几何面积 pi*a*b，守恒目标 pi*a*b=1；轴积 ab 作为雅可比面积因子一并列出",
		FogLevel:      FogLevel,
		ProbeDistance: ProbeDistance,
		Groups:        []*GroupReport{g1, g2, g3},
		Hero:          hero,
		ComboScan:     ComboScan(),
	}
}
