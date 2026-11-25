// Package interp 在 [0,1]^2 的规则三角网格上做重心坐标线性插值，
// 并给出与二阶导数有关的先验误差界。
//
// 误差界推导：设三角形 T 顶点 v1,v2,v3，最长边 h，f ∈ C²。
// 在 x 处 Taylor 展开 f(vi)，乘重心坐标 λi(x) 求和，
// 利用 Σλi = 1、Σλi(vi−x) = 0，常数项与一阶项相消：
//
//	f(x) − I_h f(x) = −½ Σi λi(x) (vi−x)ᵀ H(ξi) (vi−x)
//
// 记 M2 = max_T max_|d|=1 |dᵀHd| ≤ max_T (|fxx| + 2|fxy| + |fyy|)，则
// |e| ≤ ½ M2 Σi λi|x−vi|²。再由恒等式
//
//	Σi λi|x−vi|² = ½ Σij λiλj |vi−vj|² ≤ ½ h²
//
// 得二阶收敛界
//
//	|f − I_h f| ≤ (M2/4)·h²
//
// 系数 C = M2/4 只与二阶导有关。反算边长：h ≤ 2·sqrt(eps/M2)。
package interp

import "math"

// Field 是定义在 [0,1]^2 上的标量场。
type Field func(x, y float64) float64

// Mesh 是 [0,1]^2 上的规则三角网格：N×N 个单元，
// 每个单元沿主对角线分成两个三角形。最大边长 h = sqrt(2)/N。
type Mesh struct{ N int }

func NewMesh(n int) Mesh {
	if n < 1 {
		n = 1
	}
	return Mesh{N: n}
}

// H 返回网格最大边长（单元对角线）。
func (m Mesh) H() float64 { return math.Sqrt2 / float64(m.N) }

// Nodal 把场函数采样到网格节点，行优先存储，下标 j*(N+1)+i。
func (m Mesh) Nodal(f Field) []float64 {
	n := m.N
	vals := make([]float64, (n+1)*(n+1))
	for j := 0; j <= n; j++ {
		for i := 0; i <= n; i++ {
			vals[j*(n+1)+i] = f(float64(i)/float64(n), float64(j)/float64(n))
		}
	}
	return vals
}

// Eval 返回 (x,y) 处对节点值 nodal 的重心线性插值。
func (m Mesh) Eval(nodal []float64, x, y float64) float64 {
	n := m.N
	if x < 0 {
		x = 0
	}
	if x > 1 {
		x = 1
	}
	if y < 0 {
		y = 0
	}
	if y > 1 {
		y = 1
	}
	fx, fy := x*float64(n), y*float64(n)
	i := int(math.Floor(fx))
	j := int(math.Floor(fy))
	if i >= n {
		i = n - 1
	}
	if j >= n {
		j = n - 1
	}
	u := fx - float64(i)
	v := fy - float64(j)
	at := func(ii, jj int) float64 { return nodal[jj*(n+1)+ii] }
	if u+v <= 1 {
		// 下三角形 (i,j),(i+1,j),(i,j+1)
		return (1-u-v)*at(i, j) + u*at(i+1, j) + v*at(i, j+1)
	}
	// 上三角形 (i+1,j+1),(i,j+1),(i+1,j)
	a := u + v - 1
	return a*at(i+1, j+1) + (1-u)*at(i, j+1) + (1-v)*at(i+1, j)
}

// Coeff 返回误差界系数 C = M2/4，使 |f − I_h f| ≤ C·h²。
// M2 取 max_T (|fxx| + 2|fxy| + |fyy|) 的上界即可。
func Coeff(m2 float64) float64 { return m2 / 4 }

// Bound 返回边长为 h 时的误差上界 (M2/4)·h²。
func Bound(m2, h float64) float64 { return Coeff(m2) * h * h }

// HForError 由目标误差 eps 反算允许的最大边长 h = 2·sqrt(eps/M2)。
func HForError(m2, eps float64) float64 { return 2 * math.Sqrt(eps/m2) }

// MeshForError 反算满足目标误差 eps 的最粗网格。
func MeshForError(m2, eps float64) Mesh {
	return NewMesh(int(math.Ceil(math.Sqrt2 / HForError(m2, eps))))
}

// MaxError 在 (samples+1)^2 个均匀采样点上测 |f − I_h f| 的最大值。
func (m Mesh) MaxError(f Field, samples int) float64 {
	nodal := m.Nodal(f)
	maxErr := 0.0
	for j := 0; j <= samples; j++ {
		y := float64(j) / float64(samples)
		for i := 0; i <= samples; i++ {
			x := float64(i) / float64(samples)
			if e := math.Abs(f(x, y) - m.Eval(nodal, x, y)); e > maxErr {
				maxErr = e
			}
		}
	}
	return maxErr
}
