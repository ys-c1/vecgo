package affine

import (
	"errors"
	"fmt"
	"math"

	"vecgo/geom"
)

// TolerancePixel 是题目给定的重投影残差上限（欧氏距离，像素）。
const TolerancePixel = 0.5

// Matrix 是二维仿射变换：
//
//	X = A*x + B*y + E
//	Y = C*x + D*y + F
//
// 按数学习惯写成 [[A,B,E],[C,D,F]]。
type Matrix struct {
	A, B, C, D, E, F float64
}

// Values 按 A,B,C,D,E,F 的顺序逐个返回六个参数。
func (m Matrix) Values() [6]float64 {
	return [6]float64{m.A, m.B, m.C, m.D, m.E, m.F}
}

// Apply 把源坐标 p 重投影到目标图像坐标。
func (m Matrix) Apply(p geom.Point) geom.Point {
	return geom.Point{
		X: m.A*p.X + m.B*p.Y + m.E,
		Y: m.C*p.X + m.D*p.Y + m.F,
	}
}

// Pair 是一个编号交点的“源角点 -> 目标落点”对应关系。
type Pair struct {
	ID  int
	Src geom.Point
	Dst geom.Point
}

// Residual 是一个角点回代后的残差明细。
type Residual struct {
	ID       int
	Want     geom.Point
	Got      geom.Point
	DX       float64
	DY       float64
	Distance float64
}

// DiagonalConflict 描述平行四边形对角中点约束的冲突。
//
// 对按 1=左上、2=右上、3=右下、4=左下 编号的矩形四角，仿射变换必须保持
// 对角线共中点，即 D1+D3 == D2+D4。冲突在两条对角交点对之间。
type DiagonalConflict struct {
	PairA       [2]int
	PairB       [2]int
	Closure     geom.Point
	MidpointGap float64
}

// Fit 保存最小二乘仿射解、四角残差和诊断结论。
type Fit struct {
	Matrix       Matrix
	Residuals    []Residual
	MaxResidual  float64
	Tolerance    float64
	OK           bool
	BadIDs       []int
	Diagonal     DiagonalConflict
	HasDiagonal  bool
	NormalMatrix [3][3]float64
	XRight       [3]float64
	YRight       [3]float64
}

// Solve 用全部对应点建立超定方程，并通过法方程求六个仿射参数。
// X/Y 两组参数共用同一个设计矩阵 Z，因此分别解两个 3x3 法方程即可。
func Solve(pairs []Pair) (*Fit, error) {
	if len(pairs) < 3 {
		return nil, errors.New("仿射求解至少需要 3 个不共线对应点")
	}
	seen := make(map[int]bool, len(pairs))
	for _, p := range pairs {
		if p.ID <= 0 {
			return nil, fmt.Errorf("交点编号必须为正: %+v", p)
		}
		if seen[p.ID] {
			return nil, fmt.Errorf("交点编号重复: %d", p.ID)
		}
		seen[p.ID] = true
		if math.IsNaN(p.Src.X) || math.IsNaN(p.Src.Y) || math.IsNaN(p.Dst.X) || math.IsNaN(p.Dst.Y) {
			return nil, fmt.Errorf("交点 %d 含 NaN/Inf 坐标", p.ID)
		}
	}

	var ztz [3][3]float64
	var ztx, zty [3]float64
	for _, p := range pairs {
		z := [3]float64{p.Src.X, p.Src.Y, 1}
		for i := 0; i < 3; i++ {
			for j := 0; j < 3; j++ {
				ztz[i][j] += z[i] * z[j]
			}
			ztx[i] += z[i] * p.Dst.X
			zty[i] += z[i] * p.Dst.Y
		}
	}

	ax, err := solve3(ztz, ztx)
	if err != nil {
		return nil, fmt.Errorf("X 参数方程退化（源角点可能共线）: %w", err)
	}
	ay, err := solve3(ztz, zty)
	if err != nil {
		return nil, fmt.Errorf("Y 参数方程退化（源角点可能共线）: %w", err)
	}

	// ax = (A,B,E), ay = (C,D,F)
	m := Matrix{A: ax[0], B: ax[1], E: ax[2], C: ay[0], D: ay[1], F: ay[2]}
	out := &Fit{
		Matrix:       m,
		Tolerance:    TolerancePixel,
		NormalMatrix: ztz,
		XRight:       ztx,
		YRight:       zty,
	}

	for _, p := range pairs {
		got := m.Apply(p.Src)
		dx := got.X - p.Dst.X
		dy := got.Y - p.Dst.Y
		r := math.Hypot(dx, dy)
		out.Residuals = append(out.Residuals, Residual{
			ID: p.ID, Want: p.Dst, Got: got, DX: dx, DY: dy, Distance: r,
		})
		if r > out.MaxResidual {
			out.MaxResidual = r
		}
	}
	out.OK = out.MaxResidual <= out.Tolerance

	if len(pairs) == 4 {
		// 输入顺序即 1,2,3,4；不依赖 ID 的数值大小。
		a, b, c, d := pairs[0], pairs[1], pairs[2], pairs[3]
		closure := geom.Point{
			X: a.Dst.X + c.Dst.X - b.Dst.X - d.Dst.X,
			Y: a.Dst.Y + c.Dst.Y - b.Dst.Y - d.Dst.Y,
		}
		out.Diagonal = DiagonalConflict{
			PairA:       [2]int{a.ID, c.ID},
			PairB:       [2]int{b.ID, d.ID},
			Closure:     closure,
			MidpointGap: math.Hypot(closure.X, closure.Y) / 2,
		}
		out.HasDiagonal = true
	}
	if !out.OK {
		for _, r := range out.Residuals {
			if r.Distance > out.Tolerance {
				out.BadIDs = append(out.BadIDs, r.ID)
			}
		}
	}
	return out, nil
}

// solve3 用带部分选主元的高斯消元解 3x3 线性方程组。
func solve3(a [3][3]float64, b [3]float64) ([3]float64, error) {
	const eps = 1e-12
	for col := 0; col < 3; col++ {
		pivot := col
		for row := col + 1; row < 3; row++ {
			if math.Abs(a[row][col]) > math.Abs(a[pivot][col]) {
				pivot = row
			}
		}
		if math.Abs(a[pivot][col]) < eps {
			return [3]float64{}, errors.New("系数矩阵奇异")
		}
		if pivot != col {
			a[col], a[pivot] = a[pivot], a[col]
			b[col], b[pivot] = b[pivot], b[col]
		}
		for row := col + 1; row < 3; row++ {
			q := a[row][col] / a[col][col]
			for j := col; j < 3; j++ {
				a[row][j] -= q * a[col][j]
			}
			b[row] -= q * b[col]
		}
	}
	var x [3]float64
	for i := 2; i >= 0; i-- {
		s := b[i]
		for j := i + 1; j < 3; j++ {
			s -= a[i][j] * x[j]
		}
		x[i] = s / a[i][i]
	}
	return x, nil
}
