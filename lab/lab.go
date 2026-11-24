// Package lab 编排“金字塔层数—重建误差”实验：用 vecgo 管线渲染真实场景，
// 建均值金字塔，拟合每层误差衰减，穷举全部层数，并对三档目标误差给出
// 解析最小层数与实测对照。cmd/pyrlab 与测试共用本包。
package lab

import (
	"math"

	"vecgo/config"
	"vecgo/geom"
	"vecgo/pipe"
	"vecgo/pyramid"
)

// Target 是一档目标误差（灰度级 RMSE，0..255）。
type Target struct {
	Name    string  `json:"name"`
	Epsilon float64 `json:"epsilon"`
}

// TargetResult 是一档目标的完整对照。
type TargetResult struct {
	Name             string  `json:"name"`
	Epsilon          float64 `json:"epsilon"`
	RequiredNReal    float64 `json:"required_n_real"`    // 解析实数根
	AnalyticN        int     `json:"analytic_n"`         // 向上取整后的最小层数
	ExhaustiveN      int     `json:"exhaustive_n"`       // 穷举首个实测达标层
	SolutionsMatch   bool    `json:"solutions_match"`    // 解析与穷举是否一致
	MeasuredRMSE     float64 `json:"measured_rmse"`      // 该层浮点实测
	ImageRMSE        float64 `json:"image_rmse"`         // 该层 8 位图像实测
	PredictedRMSE    float64 `json:"predicted_rmse"`     // A Q^n 预测
	FiniteTailRMSE   float64 `json:"finite_tail_rmse"`   // 有限平方尾和预测
	RelativeErrorPct float64 `json:"relative_error_pct"` // 实测相对 AQ^n 偏差
	ExhaustiveRMSE   float64 `json:"exhaustive_rmse"`    // 穷举层实测（不一致时备查）
}

// Report 是整份实验数据，可直接序列化为 JSON。
type Report struct {
	Width       int                  `json:"width"`
	Height      int                  `json:"height"`
	SuperSample int                  `json:"supersample"`
	Shading     string               `json:"shading"`
	Depth       int                  `json:"depth"`
	Model       pyramid.Model        `json:"model"`
	TotalFit    TotalFit             `json:"total_fit"`
	Delta       []float64            `json:"layer_rmse_coarse_to_fine"`
	Ratios      []float64            `json:"layer_ratios"`
	Sweep       []pyramid.SweepPoint `json:"sweep"`
	Targets     []TargetResult       `json:"targets"`
}

// TotalFit 是直接对总误差 E(n) 做对数线性拟合的结果，用来核对推导模型。
type TotalFit struct {
	A        float64 `json:"a"`
	Q        float64 `json:"q"`
	RSquared float64 `json:"r_squared"`
	FitFromN int     `json:"fit_from_n"`
	FitToN   int     `json:"fit_to_n"`
}

// CanonicalTargets 是选定的三档目标误差（灰度级 RMSE）：
// 宽松 / 中等 / 严苛，解出的层数为 3 / 5 / 7，等间距落在曲线的
// 初、中、近细段，且解析解与穷举解一致。
func CanonicalTargets() []Target {
	return []Target{
		{Name: "宽松 ε=20", Epsilon: 20},
		{Name: "中等 ε=5", Epsilon: 5},
		{Name: "严苛 ε=1.5", Epsilon: 1.5},
	}
}

// CanonicalScene 是实验固定使用的 vecgo 场景：一个铺满画幅的四边形 +
// gradient 着色，产生跨满 0..255 的纵向渐变，使块均值误差呈纯几何衰减。
func CanonicalScene() [][]geom.Point {
	return [][]geom.Point{
		{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}},
	}
}

// RenderCanonical 用固定高质量设置渲染基准图。
func RenderCanonical() (*pyramid.Pyramid, config.Config, error) {
	cfg := config.Default()
	cfg.Width, cfg.Height = 256, 256
	cfg.Supersample = 4
	cfg.Shading = "gradient"
	im, err := pipe.Run(CanonicalScene(), cfg)
	if err != nil {
		return nil, cfg, err
	}
	p, err := pyramid.FromImage(im)
	return p, cfg, err
}

// Run 渲染、建模、穷举并对三档目标求解。
func Run(targets []Target) (*Report, error) {
	p, cfg, err := RenderCanonical()
	if err != nil {
		return nil, err
	}
	delta := p.LayerErrors()
	model := pyramid.FitDecay(delta)
	model.Layers = p.Depth

	ratios := make([]float64, len(delta))
	for j := 1; j < len(delta); j++ {
		if delta[j-1] > 1e-12 {
			ratios[j] = delta[j] / delta[j-1]
		}
	}

	sweep, err := p.Sweep(model)
	if err != nil {
		return nil, err
	}

	tf := fitTotal(sweep, 1, p.Depth-1)

	if targets == nil {
		targets = CanonicalTargets()
	}
	results := make([]TargetResult, 0, len(targets))
	for _, t := range targets {
		nReal := model.RequiredN(t.Epsilon)
		nA := model.MinLevels(t.Epsilon)
		nExh := exhaustiveMin(sweep, t.Epsilon)
		spA := sweep[nA-1]
		spE := sweep[nExh-1]
		rel := math.NaN()
		if spA.Predicted > 1e-12 {
			rel = 100 * (spA.Measured - spA.Predicted) / spA.Predicted
		}
		results = append(results, TargetResult{
			Name:             t.Name,
			Epsilon:          t.Epsilon,
			RequiredNReal:    nReal,
			AnalyticN:        nA,
			ExhaustiveN:      nExh,
			SolutionsMatch:   nA == nExh,
			MeasuredRMSE:     spA.Measured,
			ImageRMSE:        spA.ImageMeasured,
			PredictedRMSE:    spA.Predicted,
			FiniteTailRMSE:   spA.FiniteTail,
			RelativeErrorPct: rel,
			ExhaustiveRMSE:   spE.Measured,
		})
	}

	return &Report{
		Width:       cfg.Width,
		Height:      cfg.Height,
		SuperSample: cfg.Supersample,
		Shading:     cfg.Shading,
		Depth:       p.Depth,
		Model:       model,
		TotalFit:    tf,
		Delta:       delta,
		Ratios:      ratios,
		Sweep:       sweep,
		Targets:     results,
	}, nil
}

// exhaustiveMin 线性扫描全部层数，返回首个实测浮点 RMSE<=epsilon 的层。
// 若没有任何非精确层满足，返回精确层 Depth（其误差为 0）。
func exhaustiveMin(sweep []pyramid.SweepPoint, epsilon float64) int {
	for _, sp := range sweep {
		if sp.Measured <= epsilon+1e-12 {
			return sp.N
		}
	}
	return sweep[len(sweep)-1].N
}

// fitTotal 直接对总误差扫描点 E(n)=A Q^n 做对数线性最小二乘，
// 拟合 n∈[fromN,toN]（含端点，按 SweepPoint.N）。
func fitTotal(sweep []pyramid.SweepPoint, fromN, toN int) TotalFit {
	var sx, sy, sxx, sxy float64
	n := 0.0
	for _, sp := range sweep {
		if sp.N < fromN || sp.N > toN || sp.Measured <= 1e-9 {
			continue
		}
		x := float64(sp.N)
		y := math.Log(sp.Measured)
		sx += x
		sy += y
		sxx += x * x
		sxy += x * y
		n++
	}
	den := n*sxx - sx*sx
	b := (n*sxy - sx*sy) / den
	a := (sy - b*sx) / n

	meanY := sy / n
	var ssTot, ssRes float64
	for _, sp := range sweep {
		if sp.N < fromN || sp.N > toN || sp.Measured <= 1e-9 {
			continue
		}
		y := math.Log(sp.Measured)
		yhat := a + b*float64(sp.N)
		ssTot += (y - meanY) * (y - meanY)
		ssRes += (y - yhat) * (y - yhat)
	}
	return TotalFit{
		A:        math.Exp(a),
		Q:        math.Exp(b),
		RSquared: 1 - ssRes/ssTot,
		FitFromN: fromN,
		FitToN:   toN,
	}
}
