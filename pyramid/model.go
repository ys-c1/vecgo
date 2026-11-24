package pyramid

import (
	"math"
)

// Model 是“总误差 ~ 层数”的几何衰减模型 E(n)=A·Q^n。
//
// 参数由每层细节误差 δ_j=d1·Q^(j-1) 拟合得到，再经正交平方尾和
// E(n)=δ_n/√(1-Q²) 推出 A=d1/(Q·√(1-Q²))。
type Model struct {
	D1        float64 `json:"d1"`         // 首层（最粗）细节 RMS δ_1
	Q         float64 `json:"q"`          // 逐层衰减率（0<Q<1）；退化常量图为 0
	A         float64 `json:"a"`          // 总误差曲线振幅
	RSquared  float64 `json:"r_squared"`  // ln δ 线性拟合优度
	Layers    int     `json:"layers"`     // 金字塔总层数 L
	FitPoints int     `json:"fit_points"` // 参与拟合的正误差层数
}

// Usable 报告模型是否为可用于反解层数的有效几何衰减（0<Q<1 且 A>0）。
func (m Model) Usable() bool {
	return m.A > 0 && m.Q > 0 && m.Q < 1 && !math.IsNaN(m.A) && !math.IsNaN(m.Q)
}

// FitDecay 对由粗到精的每层 RMS δ_j 做 ln δ_j = ln d1 + (j-1) ln Q
// 最小二乘拟合。δ<=epsilon 的层（通常是末端零层）跳过。
// 对常量图（没有任何正细节误差）返回 A=Q=0 的退化模型，MinLevels 据此
// 直接选第 1 层，不会产生 NaN。
func FitDecay(delta []float64) Model {
	var sx, sy, sxx, sxy float64
	n := 0.0
	for j0, d := range delta {
		if d <= 1e-9 {
			continue
		}
		x := float64(j0) // x=j-1，首层 x=0 -> 截距即 ln d1
		y := math.Log(d)
		sx += x
		sy += y
		sxx += x * x
		sxy += x * y
		n++
	}
	if n == 0 {
		return Model{Q: 0, D1: 0, A: 0, RSquared: 1, FitPoints: 0}
	}

	den := n*sxx - sx*sx
	var b, a float64
	if den == 0 {
		// 只有一个有效 x 点（极小图），无法估计斜率；按“其后快速衰减”
		// 给一个安全 q，选层仍可由穷举兜底。
		b = math.Log(0.5)
		a = sy / n
	} else {
		b = (n*sxy - sx*sy) / den
		a = (sy - b*sx) / n
	}
	q := math.Exp(b)
	d1 := math.Exp(a)

	// 拟合优度（对数域 R²）。
	meanY := sy / n
	var ssTot, ssRes float64
	for j0, d := range delta {
		if d <= 1e-9 {
			continue
		}
		y := math.Log(d)
		yhat := a + b*float64(j0)
		ssTot += (y - meanY) * (y - meanY)
		ssRes += (y - yhat) * (y - yhat)
	}
	r2 := 1 - ssRes/ssTot
	if ssTot == 0 || math.IsNaN(r2) {
		r2 = 1
	}

	m := Model{D1: d1, Q: q, RSquared: r2, FitPoints: int(n)}
	if q > 0 && q < 1 {
		m.A = d1 / (q * math.Sqrt(math.Max(0, 1-q*q)))
	} else {
		m.A = d1
	}
	return m
}

// Predict 返回解析曲线 E(n)=A·Q^n 在实数 n 处的值。
func (m Model) Predict(n float64) float64 {
	return m.A * math.Pow(m.Q, n)
}

// FiniteTailPredict 是带有限层数修正的平方尾和预测：
//
//	E(n)=d1·Q^(n-1)·sqrt((1-Q^(2·r))/(1-Q²)), r=L-n（剩余细节层数）。
//
// n=L 时无剩余细节，返回 0。
func (m Model) FiniteTailPredict(n int) float64 {
	if n >= m.Layers {
		return 0
	}
	r := m.Layers - n // 省略的细节层个数 M-n+1，M=L-1 => L-n
	dn := m.D1 * math.Pow(m.Q, float64(n-1))
	num := 1 - math.Pow(m.Q, 2*float64(r))
	return dn * math.Sqrt(num/(1-m.Q*m.Q))
}

// RequiredN 返回满足 A·Q^n=epsilon 的实数层数根
// n0=ln(epsilon/A)/ln Q。epsilon<=0 返回 +Inf；退化模型返回 NaN。
func (m Model) RequiredN(epsilon float64) float64 {
	if epsilon <= 0 {
		return math.Inf(1)
	}
	if !m.Usable() {
		return math.NaN()
	}
	return math.Log(epsilon/m.A) / math.Log(m.Q)
}

// MinLevels 是解析解：满足预测误差 <= epsilon 的最小整数层数，
// 截断到 [1, Layers]。由于第 L 层精确还原（误差 0），任意 epsilon>=0
// 都有可行整数解。退化模型（常量图，A=0）在第 1 层误差即为 0，返回 1。
func (m Model) MinLevels(epsilon float64) int {
	if epsilon <= 0 {
		return m.Layers
	}
	if m.A == 0 {
		return 1
	}
	if !m.Usable() {
		// 无法可靠反解衰减率时，精确层兜底，保证误差达标。
		return m.Layers
	}
	n0 := m.RequiredN(epsilon)
	if math.IsNaN(n0) || math.IsInf(n0, 0) {
		return m.Layers
	}
	n := int(math.Ceil(n0 - 1e-12)) // 抵消整根处的浮点上偏
	if n < 1 {
		n = 1
	}
	if n > m.Layers {
		n = m.Layers
	}
	return n
}

// ExhaustiveMinLevels 逐层重建并实测，返回第一个浮点 RMSE<=epsilon 的层；
// 没有非精确层达标时返回 Depth（精确还原，误差 0）。它是解析解的穷举对照，
// 也作为退化图像上的稳健选层兜底。
func (p *Pyramid) ExhaustiveMinLevels(epsilon float64) int {
	for n := 1; n <= p.Depth; n++ {
		f, err := p.ReconstructFloat(n)
		if err != nil {
			continue
		}
		if RMS(f, p.src) <= epsilon+1e-12 {
			return n
		}
	}
	return p.Depth
}

// SweepPoint 是固定层数穷举扫描中的一行。
type SweepPoint struct {
	N             int     `json:"n"`
	Measured      float64 `json:"measured_rmse"`          // 浮点重建 vs 原始亮度
	ImageMeasured float64 `json:"image_rmse"`             // 8 位往返后实测
	Predicted     float64 `json:"predicted_rmse"`         // A Q^n
	FiniteTail    float64 `json:"finite_tail_rmse"`       // 有限平方尾和预测
	EnergyRMSE    float64 `json:"orthogonal_energy_rmse"` // sqrt(Σ省略 δ²)
}

// Sweep 穷举 n=1..Depth 的实测重建误差，并并列两种预测。
func (p *Pyramid) Sweep(model Model) ([]SweepPoint, error) {
	delta := p.LayerErrors() // 索引 0=δ_1
	out := make([]SweepPoint, p.Depth)
	for n := 1; n <= p.Depth; n++ {
		f, err := p.ReconstructFloat(n)
		if err != nil {
			return nil, err
		}
		meas := RMS(f, p.src)
		im, err := p.Reconstruct(n)
		if err != nil {
			return nil, err
		}
		imRMSE, err := ImageRMSE(im, p.srcIm)
		if err != nil {
			return nil, err
		}

		// 正交能量预测：省略的细节是 δ_n..δ_M（切片 delta[n-1:]）。
		var e2 float64
		for j := n; j <= p.Depth-1; j++ {
			d := delta[j-1]
			e2 += d * d
		}
		out[n-1] = SweepPoint{
			N:             n,
			Measured:      meas,
			ImageMeasured: imRMSE,
			Predicted:     model.Predict(float64(n)),
			FiniteTail:    model.FiniteTailPredict(n),
			EnergyRMSE:    math.Sqrt(e2),
		}
	}
	return out, nil
}
