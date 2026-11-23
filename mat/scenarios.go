package mat

import (
	"math"
	"math/rand"
	"time"
)

// Scenario 描述一个分解用例：输入矩阵、记录的构造顺序与是否预期报错。
type Scenario struct {
	Name      string
	M         Mat3
	Ops       []Op
	ExpectErr bool
}

// ScenarioResult 是单个用例的分解结果与度量。
type ScenarioResult struct {
	Name       string
	Err        error
	Pos        *PosError
	Parts      *Parts
	Recomposed Mat3
	MaxErr     float64 // 乘回矩阵与原矩阵的逐元素最大绝对差
	Elapsed    time.Duration
	OrderErr   error
}

// Tier1Scenarios 返回第一档用例：纯平移、纯旋转、非均匀缩放、
// 含错切、退化（缩放为零），另附 NaN 与行列式为零两个报错用例。
func Tier1Scenarios() []Scenario {
	nan := math.NaN()
	return []Scenario{
		{
			Name: "纯平移",
			M:    Translate(3, -2),
			Ops:  []Op{{Kind: OpTranslate, Param: 3, Param2: -2}},
		},
		{
			Name: "纯旋转",
			M:    Rotate(math.Pi / 3),
			Ops:  []Op{{Kind: OpRotate, Param: math.Pi / 3}},
		},
		{
			Name: "非均匀缩放",
			M:    Scale(2, 0.5),
			Ops:  []Op{{Kind: OpScale, Param: 2, Param2: 0.5}},
		},
		{
			Name: "含错切",
			M: NewBuilder().
				Rotate(0.4).
				Scale(1.5, 2).
				Shear(0.7).
				Matrix(),
			Ops: []Op{
				{Kind: OpRotate, Param: 0.4},
				{Kind: OpScale, Param: 1.5, Param2: 2},
				{Kind: OpShear, Param: 0.7},
			},
		},
		{
			Name: "全复合 T·R·S·H",
			M: NewBuilder().
				Translate(-1.5, 2.25).
				Rotate(-0.6).
				Scale(2, 3).
				Shear(0.3).
				Matrix(),
			Ops: []Op{
				{Kind: OpTranslate, Param: -1.5, Param2: 2.25},
				{Kind: OpRotate, Param: -0.6},
				{Kind: OpScale, Param: 2, Param2: 3},
				{Kind: OpShear, Param: 0.3},
			},
		},
		{
			Name:      "退化：缩放为零",
			M:         Scale(3, 0),
			ExpectErr: true,
		},
		{
			Name:      "退化：行列式为零",
			M:         New(1, 2, 0.5, 2, 4, -1, 0, 0, 1),
			ExpectErr: true,
		},
		{
			Name:      "报错：含 NaN",
			M:         New(1, nan, 0, 0, 1, 0, 0, 0, 1),
			ExpectErr: true,
		},
	}
}

// RunScenario 执行单个用例：分解、乘回比对、顺序核对并计时。
func RunScenario(s Scenario) ScenarioResult {
	r := ScenarioResult{Name: s.Name}
	start := time.Now()
	p, err := Decompose(s.M)
	r.Elapsed = time.Since(start)
	r.Err = err
	if err != nil {
		if pe, ok := AsPosError(err); ok {
			r.Pos = pe
		}
		return r
	}
	r.Parts = &p
	r.Recomposed = p.Recompose()
	r.MaxErr = MaxAbsDiff(s.M, r.Recomposed)
	if s.Ops != nil {
		r.OrderErr = CheckOrder(s.Ops, p)
	}
	return r
}

// RunTier1 执行第一档全部用例。
func RunTier1() []ScenarioResult {
	ss := Tier1Scenarios()
	out := make([]ScenarioResult, len(ss))
	for i, s := range ss {
		out[i] = RunScenario(s)
	}
	return out
}

// StressResult 汇总第二档压力用例。
type StressResult struct {
	N                 int
	MaxRecomposeErr   float64
	TotalElapsed      time.Duration
	AvgElapsed        time.Duration
	OrderMismatches   int
	RecomposeFailures int
}

// RunStress 是第二档：随机生成 n 个非退化仿射矩阵，
// 全部按 T·R·S·H 规范顺序构造，逐个分解、乘回并核对顺序。
func RunStress(n int, seed int64) StressResult {
	rng := rand.New(rand.NewSource(seed))
	res := StressResult{N: n}
	start := time.Now()
	for i := 0; i < n; i++ {
		tx, ty := (rng.Float64()-0.5)*20, (rng.Float64()-0.5)*20
		theta := (rng.Float64() - 0.5) * 2 * math.Pi
		// 缩放避开零点与过小区间。
		sx := 0.1 + rng.Float64()*4.9
		sy := 0.1 + rng.Float64()*4.9
		h := (rng.Float64() - 0.5) * 4

		b := NewBuilder().Translate(tx, ty).Rotate(theta).Scale(sx, sy).Shear(h)
		ops := b.Ops()
		m := b.Matrix()

		p, err := Decompose(m)
		if err != nil {
			res.RecomposeFailures++
			continue
		}
		if d := MaxAbsDiff(m, p.Recompose()); d > res.MaxRecomposeErr {
			res.MaxRecomposeErr = d
		}
		if err := CheckOrder(ops, p); err != nil {
			res.OrderMismatches++
		}
	}
	res.TotalElapsed = time.Since(start)
	if n > 0 {
		res.AvgElapsed = res.TotalElapsed / time.Duration(n)
	}
	return res
}
