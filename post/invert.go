package post

import "math"

// 两次串联盒式模糊的半径反解。
//
// 目标核 g 取网格上归一化的高斯核，串联等效核为两个一维盒核的卷积
//（Blur 的方形核可分离，一维结论直接适用于二维）。残差表达式：
//
//	E(r1,r2) = Σ_x [ (b_{r1} ⊗ b_{r2})(x) − g(x) ]²
//
// 反解用连续松弛的 Gauss–Newton 最小二乘：盒核半径放宽为实数
//（面积采样，整数半径处与离散盒核完全一致），每步解 2×2 法方程
//
//	Δθ = −(JᵀJ)⁻¹ Jᵀ e
//
// 连续解收敛后在 floor/ceil 邻域中取整，得到整数半径组合。

// SigmaForRadius 给出与单次大核半径 R 等方差的高斯 σ：var(b_R) = R(R+1)/3。
func SigmaForRadius(R int) float64 {
	return math.Sqrt(float64(R*(R+1)) / 3)
}

// KernelHalf 取核网格半宽，保证高斯尾部与串联支撑都落在网格内。
func KernelHalf(sigma float64, rmax int) int {
	return 4*int(math.Ceil(sigma)) + 2*rmax + 2
}

// boxKernel1D 返回 −half..half 网格上归一化的一维盒核。
// 实数半径按面积采样，整数半径处与 Blur 用的离散盒核一致。
func boxKernel1D(r float64, half int) []float64 {
	k := make([]float64, 2*half+1)
	sum := 0.0
	for x := -half; x <= half; x++ {
		a := r + 1 - math.Abs(float64(x))
		if a > 1 {
			a = 1
		}
		if a < 0 {
			a = 0
		}
		k[x+half] = a
		sum += a
	}
	if sum > 0 {
		for i := range k {
			k[i] /= sum
		}
	}
	return k
}

// gaussKernel1D 返回网格上归一化的离散高斯核。
func gaussKernel1D(sigma float64, half int) []float64 {
	k := make([]float64, 2*half+1)
	sum := 0.0
	for x := -half; x <= half; x++ {
		v := math.Exp(-float64(x*x) / (2 * sigma * sigma))
		k[x+half] = v
		sum += v
	}
	for i := range k {
		k[i] /= sum
	}
	return k
}

// convolve1DSame 计算居中卷积，输出与 a 等长。
func convolve1DSame(a, b []float64) []float64 {
	n := len(a)
	hb := len(b) / 2
	out := make([]float64, n)
	for i := range out {
		s := 0.0
		for j, v := range b {
			if k := i + j - hb; k >= 0 && k < n {
				s += a[k] * v
			}
		}
		out[i] = s
	}
	return out
}

// cascadeKernel 是半径 r1、r2 两次串联的等效一维核。
func cascadeKernel(r1, r2 float64, half int) []float64 {
	return convolve1DSame(boxKernel1D(r1, half), boxKernel1D(r2, half))
}

// kernelResidual 即残差表达式 E = Σ (t − g)²。
func kernelResidual(t, g []float64) float64 {
	s := 0.0
	for i := range t {
		d := t[i] - g[i]
		s += d * d
	}
	return s
}

// SolveCascadePair 用 Gauss–Newton 最小二乘反解串联半径组合。
// 返回取整后的整数组合、连续解与串联残差。
func SolveCascadePair(sigma float64, half int) (r1, r2 int, cont [2]float64, resid float64) {
	g := gaussKernel1D(sigma, half)
	// 初值：等半径、方差各半，且取非对称点避开 r1==r2 处 J 的秩亏
	s2 := 0.0
	for x := -half; x <= half; x++ {
		s2 += float64(x*x) * g[x+half]
	}
	r0 := (math.Sqrt(1+6*s2) - 1) / 2
	th := [2]float64{1.25 * r0, 0.75 * r0}
	const h = 1e-4
	for it := 0; it < 500; it++ {
		t := cascadeKernel(th[0], th[1], half)
		j1 := make([]float64, len(t))
		j2 := make([]float64, len(t))
		t1p := cascadeKernel(th[0]+h, th[1], half)
		t1m := cascadeKernel(th[0]-h, th[1], half)
		t2p := cascadeKernel(th[0], th[1]+h, half)
		t2m := cascadeKernel(th[0], th[1]-h, half)
		var s11, s12, s22, u1, u2 float64
		for i := range t {
			j1[i] = (t1p[i] - t1m[i]) / (2 * h)
			j2[i] = (t2p[i] - t2m[i]) / (2 * h)
			e := t[i] - g[i]
			s11 += j1[i] * j1[i]
			s12 += j1[i] * j2[i]
			s22 += j2[i] * j2[i]
			u1 += j1[i] * e
			u2 += j2[i] * e
		}
		lam := 1e-10*(s11+s22)/2 + 1e-15
		s11 += lam
		s22 += lam
		det := s11*s22 - s12*s12
		// 法方程一行：Δθ = −(JᵀJ)⁻¹ Jᵀe
		d1 := -(s22*u1 - s12*u2) / det
		d2 := -(-s12*u1 + s11*u2) / det
		if n := math.Hypot(d1, d2); n > 2 {
			d1 *= 2 / n
			d2 *= 2 / n
		} else if n < 1e-12 {
			break
		}
		th[0] = math.Max(th[0]+d1, 0)
		th[1] = math.Max(th[1]+d2, 0)
	}
	cont = th
	// 连续解的 floor/ceil 邻域中取残差最小的整数组合
	resid = math.Inf(1)
	for _, a := range floorCeil(th[0]) {
		for _, b := range floorCeil(th[1]) {
			if a < 0 || b < 0 {
				continue
			}
			x, y := a, b
			if y > x {
				x, y = y, x
			}
			e := kernelResidual(cascadeKernel(float64(x), float64(y), half), g)
			if e < resid {
				resid, r1, r2 = e, x, y
			}
		}
	}
	return r1, r2, cont, resid
}

func floorCeil(v float64) [2]int {
	f := int(math.Floor(v))
	return [2]int{f, f + 1}
}

// ExhaustiveCascadePair 穷举 0≤r2≤r1≤rmax 的整数半径对，返回最优组合与残差。
func ExhaustiveCascadePair(sigma float64, half, rmax int) (r1, r2 int, resid float64) {
	g := gaussKernel1D(sigma, half)
	resid = math.Inf(1)
	for a := 0; a <= rmax; a++ {
		ka := boxKernel1D(float64(a), half)
		for b := 0; b <= a; b++ {
			e := kernelResidual(convolve1DSame(ka, boxKernel1D(float64(b), half)), g)
			if e < resid {
				resid, r1, r2 = e, a, b
			}
		}
	}
	return r1, r2, resid
}

// ControlResidual 是对照路径（单次大核 b_R）与目标核的残差。
func ControlResidual(R int, sigma float64, half int) float64 {
	return kernelResidual(boxKernel1D(float64(R), half), gaussKernel1D(sigma, half))
}
