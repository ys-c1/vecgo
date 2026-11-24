// Package alloc 把细长三角形的逐行采样预算分配问题建成方差模型，
// 并用拉格朗日乘子给出闭式最优分配；同时保留固定均分路径以便切回对照。
//
// 模型（逐行 Bernoulli 覆盖率估计）:
//
//	第 i 行斜边切割的覆盖比例为 p_i，单次采样命中三角形的方差为
//		v_i = p_i(1-p_i)。
//	给第 i 行 n_i 次独立采样，覆盖率估计量的方差为 v_i/n_i；
//	各行相加得到整张三角形面积/覆盖估计的总方差
//		V(n) = Σ_i v_i/n_i，约束 Σ_i n_i = N。
//
// 对斜率为 s = dx/dy（斜边每下移一行的横向推进，单位为像素）的单条斜边，
// 行中心处的边缘位置为 x_i = φ + s*i（φ 为亚像素相位），故
//
//	p_i = frac(x_i)，v_i = p_i(1-p_i)。
//
// 完全在内/完全在外的行 v_i=0，属于精确行，不消耗采样预算。
package alloc

import (
	"errors"
	"math"
	"math/rand"
	"sort"
)

// Strategy 选择采样预算分配路径。
type Strategy int

const (
	// Optimal 为拉格朗日闭式最优分配（最小总方差）。
	Optimal Strategy = iota
	// Fixed 为固定均分路径（每行同样多的采样，可随时切回）。
	Fixed
)

func (s Strategy) String() string {
	switch s {
	case Optimal:
		return "optimal"
	case Fixed:
		return "fixed"
	default:
		return "unknown"
	}
}

// Row 是一行扫描线的几何与误差贡献。
type Row struct {
	Index int     // 行号（0 起）
	P     float64 // 该行覆盖率 p_i
	V     float64 // 单次采样方差 v_i = p_i(1-p_i)
	N     int     // 分到的采样数 n_i
	Err   float64 // 该行误差贡献 v_i/n_i（V=0 时为 0）
}

// Coverage 由斜边斜率 s（像素/行）、亚像素相位 phi 和行数 m 构造逐行模型。
func Coverage(s, phi float64, m int) ([]Row, error) {
	if m <= 0 {
		return nil, errors.New("行数必须为正")
	}
	if s < 0 {
		return nil, errors.New("斜率取非负的 |dx/dy|")
	}
	if math.IsNaN(s) || math.IsNaN(phi) {
		return nil, errors.New("斜率/相位不能为 NaN")
	}
	rows := make([]Row, m)
	for i := 0; i < m; i++ {
		p := phi + s*float64(i)
		p -= math.Floor(p) // frac，落在 [0,1)
		v := p * (1 - p)
		rows[i] = Row{Index: i, P: p, V: v}
	}
	return rows, nil
}

// ContinuousShare 返回闭式最优的连续采样份额 n_i* = N·sqrt(v_i)/Σsqrt(v)。
// v_i=0 的行份额为 0（精确行）。
func ContinuousShare(rows []Row, N float64) []float64 {
	var sum float64
	for _, r := range rows {
		sum += math.Sqrt(r.V)
	}
	out := make([]float64, len(rows))
	if sum == 0 || N <= 0 {
		return out
	}
	for i, r := range rows {
		out[i] = N * math.Sqrt(r.V) / sum
	}
	return out
}

// OptimalVariance 是闭式连续最优总方差 V* = (Σ sqrt(v_i))² / N。
func OptimalVariance(rows []Row, N float64) float64 {
	var sum float64
	for _, r := range rows {
		sum += math.Sqrt(r.V)
	}
	return sum * sum / N
}

// UniformVariance 是固定均分（n_i=N/m）的连续总方差。
func UniformVariance(rows []Row, N float64) float64 {
	if N <= 0 || len(rows) == 0 {
		return 0
	}
	var sum float64
	for _, r := range rows {
		sum += r.V
	}
	return sum * float64(len(rows)) / N
}

// Allocate 把总预算 N 按指定策略分到各行，并回填 N 与误差贡献 Err。
//
// Optimal: 拉格朗日解 n_i ∝ sqrt(v_i) 的整数实现（最大余数法，
// 正方差行至少 1 次采样，零方差行精确不取）。
// Fixed:   固定均分，每行 floor(N/m)，余数按行号顺序补 1。
func Allocate(rows []Row, N int, strategy Strategy) ([]Row, error) {
	m := len(rows)
	if m == 0 {
		return nil, errors.New("没有扫描线行")
	}
	if N <= 0 {
		return nil, errors.New("采样预算必须为正")
	}

	out := make([]Row, m)
	copy(out, rows)
	for i := range out {
		out[i].N = 0
		out[i].Err = 0
	}

	switch strategy {
	case Fixed:
		if N < m {
			return nil, errors.New("固定均分要求预算 N >= 行数 m")
		}
		base, rem := N/m, N%m
		for i := range out {
			out[i].N = base
			if i < rem {
				out[i].N++
			}
			if out[i].V > 0 {
				out[i].Err = out[i].V / float64(out[i].N)
			}
		}
		return out, nil

	case Optimal:
		active := make([]int, 0, m)
		for i, r := range out {
			if r.V > 0 {
				active = append(active, i)
			}
		}
		k := len(active)
		if k == 0 {
			return out, nil // 全部精确，无需采样
		}
		if N < k {
			return nil, errors.New("最优分配要求预算 N >= 正方差行数")
		}
		var sumSqrt float64
		for _, i := range active {
			sumSqrt += math.Sqrt(out[i].V)
		}
		// 连续份额 -> 先保底 1 次，再按最大余数法分配剩余预算。
		type rem struct {
			idx int
			fr  float64
		}
		rems := make([]rem, 0, k)
		for _, i := range active {
			raw := float64(N) * math.Sqrt(out[i].V) / sumSqrt
			fl := math.Floor(raw)
			if fl < 1 {
				fl = 1
			}
			out[i].N = int(fl)
			rems = append(rems, rem{idx: i, fr: raw - fl})
		}
		spent := 0
		for _, i := range active {
			spent += out[i].N
		}
		left := N - spent
		// 理论上 left>=0；若浮点导致个别行超分，用保底回收循环纠正。
		if left < 0 {
			for left < 0 {
				j := 0
				for _, i := range active {
					if out[i].N > 1 && (j == 0 || out[i].N > out[j].N) {
						j = i
					}
				}
				out[j].N--
				left++
			}
		} else {
			sort.SliceStable(rems, func(a, b int) bool { return rems[a].fr > rems[b].fr })
			for t := 0; t < left; t++ {
				out[rems[t%len(rems)].idx].N++
			}
		}
		for _, i := range active {
			out[i].Err = out[i].V / float64(out[i].N)
		}
		return out, nil

	default:
		return nil, errors.New("未知分配策略")
	}
}

// TotalVariance 汇总各行误差贡献。
func TotalVariance(rows []Row) float64 {
	var v float64
	for _, r := range rows {
		v += r.Err
	}
	return v
}

// TotalSamples 汇总各行采样数。
func TotalSamples(rows []Row) int {
	var n int
	for _, r := range rows {
		n += r.N
	}
	return n
}

// Simulate 用 trials 次蒙特卡洛实验估计当前分配下覆盖总和估计量的均方误差。
// 每行独立做 n_i 次 Bernoulli(p_i) 试验，估计覆盖总和，相对真实值 Σp_i 计 MSE。
// 固定种子保证两条路径、三档斜率之间可复现对照。
func Simulate(rows []Row, trials int, seed int64) (mse float64) {
	rng := rand.New(rand.NewSource(seed))
	var truth float64
	for _, r := range rows {
		truth += r.P
	}
	var acc float64
	for t := 0; t < trials; t++ {
		var est float64
		for _, r := range rows {
			if r.N == 0 {
				est += r.P // 精确行直接用真实覆盖
				continue
			}
			hits := 0
			for k := 0; k < r.N; k++ {
				if rng.Float64() < r.P {
					hits++
				}
			}
			est += float64(hits) / float64(r.N)
		}
		d := est - truth
		acc += d * d
	}
	return acc / float64(trials)
}
