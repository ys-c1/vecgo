// Package mat 提供 3x3 仿射变换矩阵及其分解。
//
// 矩阵按行主序存储 [9]float64，作用于齐次列向量，即 p' = M·p：
//
//	[a b tx]   [x]
//	[c d ty] · [y]
//	[0 0  1]   [1]
//
// 复合顺序为 M = T·R·S·H，点先经受错切 H，再缩放 S、旋转 R、平移 T。
package mat

import (
	"errors"
	"fmt"
	"math"
)

// 元素位置常量，行主序索引 = row*3 + col。
const (
	A  = 0 // (0,0)
	B  = 1 // (0,1)
	TX = 2 // (0,2)
	C  = 3 // (1,0)
	D  = 4 // (1,1)
	TY = 5 // (1,2)
	G  = 6 // (2,0)
	H  = 7 // (2,1)
	I  = 8 // (2,2)
)

// Mat3 是行主序的 3x3 矩阵。
type Mat3 struct {
	V [9]float64
}

// New 按行主序构造矩阵。
func New(a, b, tx, c, d, ty, g, h, i float64) Mat3 {
	return Mat3{V: [9]float64{a, b, tx, c, d, ty, g, h, i}}
}

// Identity 返回单位矩阵。
func Identity() Mat3 {
	return Mat3{V: [9]float64{1, 0, 0, 0, 1, 0, 0, 0, 1}}
}

// Translate 构造平移矩阵。
func Translate(tx, ty float64) Mat3 {
	return Mat3{V: [9]float64{1, 0, tx, 0, 1, ty, 0, 0, 1}}
}

// Rotate 构造绕原点、角度为 theta（弧度）的旋转矩阵。
func Rotate(theta float64) Mat3 {
	cs, sn := math.Cos(theta), math.Sin(theta)
	return Mat3{V: [9]float64{cs, -sn, 0, sn, cs, 0, 0, 0, 1}}
}

// Scale 构造（可非均匀、可为负）缩放矩阵。
func Scale(sx, sy float64) Mat3 {
	return Mat3{V: [9]float64{sx, 0, 0, 0, sy, 0, 0, 0, 1}}
}

// Shear 构造 x 方向错切矩阵 [1 h; 0 1]。
func Shear(h float64) Mat3 {
	return Mat3{V: [9]float64{1, h, 0, 0, 1, 0, 0, 0, 1}}
}

// Mul 返回 m·n（先应用 n，再应用 m）。
func (m Mat3) Mul(n Mat3) Mat3 {
	var out Mat3
	for r := 0; r < 3; r++ {
		for c := 0; c < 3; c++ {
			s := 0.0
			for k := 0; k < 3; k++ {
				s += m.V[r*3+k] * n.V[k*3+c]
			}
			out.V[r*3+c] = s
		}
	}
	return out
}

// Apply 把矩阵作用于点 (x, y)。
func (m Mat3) Apply(x, y float64) (nx, ny float64) {
	return m.V[A]*x + m.V[B]*y + m.V[TX], m.V[C]*x + m.V[D]*y + m.V[TY]
}

// Det 返回行列式。
func (m Mat3) Det() float64 {
	return det(&m.V)
}

// det 按显式展开计算 3x3 行列式。
func det(v *[9]float64) float64 {
	return v[0]*(v[4]*v[8]-v[5]*v[7]) -
		v[1]*(v[3]*v[8]-v[5]*v[6]) +
		v[2]*(v[3]*v[7]-v[4]*v[6])
}

// String 按行输出矩阵。
func (m Mat3) String() string {
	v := &m.V
	return fmt.Sprintf("[[%.6g %.6g %.6g]\n [%.6g %.6g %.6g]\n [%.6g %.6g %.6g]]",
		v[0], v[1], v[2], v[3], v[4], v[5], v[6], v[7], v[8])
}

// PosError 携带出错元素的行列位置（从 0 起）。
type PosError struct {
	Row int
	Col int
	Val float64
	Msg string
}

func (e *PosError) Error() string {
	return fmt.Sprintf("%s：元素(%d,%d)=%g", e.Msg, e.Row, e.Col, e.Val)
}

// AsPosError 尝试把错误提取为 *PosError。
func AsPosError(err error) (*PosError, bool) {
	var pe *PosError
	if errors.As(err, &pe) {
		return pe, true
	}
	return nil, false
}

// Parts 是分解结果：M = T·R·S·H。
type Parts struct {
	Translate Mat3
	Rotate    Mat3
	Scale     Mat3
	Shear     Mat3
	Angle     float64 // 旋转角（弧度）
	SX        float64
	SY        float64
	SH        float64
	TX        float64
	TY        float64
	Reflected bool // det(L)<0，反射（负缩放）情形
}

// Recompose 按 T·R·S·H 乘回完整矩阵。
func (p Parts) Recompose() Mat3 {
	return p.Translate.Mul(p.Rotate).Mul(p.Scale).Mul(p.Shear)
}

const (
	// affineEps 是判定底行必须为 [0 0 1] 的容差。
	affineEps = 1e-12
	// identityEps 是判定某一分量是否为单位变换的容差。
	identityEps = 1e-10
)

// Decompose 把仿射矩阵分解为平移、旋转、缩放、错切四部分。
//
// 线性部分 L = R·S·H，其中 R 为旋转、S 为对角缩放、H=[1 h;0 1] 为错切，
// 分解为精确代数变形（仅一次 atan2 与除法），乘回后与原矩阵仅差浮点舍入。
//
// 错误依次为：含 NaN（报 NaN 元素位置）、非仿射底行（报越界元素位置）、
// 行列式为零、缩放为零（均报具体元素位置）。
func Decompose(m Mat3) (Parts, error) {
	v := &m.V

	// 1. NaN 检查：报出第一个 NaN 元素的位置。
	for i, x := range v {
		if math.IsNaN(x) {
			return Parts{}, &PosError{Row: i / 3, Col: i % 3, Val: x, Msg: "矩阵含 NaN"}
		}
	}

	// 2. 仿射底行检查：必须为 [0 0 1]。
	if math.Abs(v[G]) > affineEps || math.Abs(v[H]) > affineEps || math.Abs(v[I]-1) > affineEps {
		i := G
		if math.Abs(v[H]) > affineEps {
			i = H
		}
		if math.Abs(v[I]-1) > affineEps {
			i = I
		}
		return Parts{}, &PosError{Row: i / 3, Col: i % 3, Val: v[i], Msg: "非仿射矩阵：底行必须为 [0 0 1]"}
	}

	a, b, c, d := v[A], v[B], v[C], v[D]

	// 3. 行列式为零：线性部分不可逆，无法分解。
	//    先按零列结构区分“缩放为零”（单列被压没），其余归为通用奇异。
	detL := a*d - b*c
	if detL == 0 {
		switch {
		case a == 0 && c == 0:
			return Parts{}, &PosError{Row: A / 3, Col: A % 3, Val: a, Msg: "缩放为零：sx=0，退化变换"}
		case b == 0 && d == 0:
			return Parts{}, &PosError{Row: D / 3, Col: D % 3, Val: d, Msg: "缩放为零：sy=0，退化变换"}
		default:
			// 两列共线：报出线性块内真正为零的元素；
			// 若无零元素则报首列起点 A，并在信息中注明秩亏。
			i, linear := -1, [4]int{A, B, C, D}
			for _, idx := range linear {
				if v[idx] == 0 {
					i = idx
					break
				}
			}
			if i < 0 {
				i = A
				return Parts{}, &PosError{Row: i / 3, Col: i % 3, Val: v[i],
					Msg: "行列式为零：线性部分秩亏（两列共线）"}
			}
			return Parts{}, &PosError{Row: i / 3, Col: i % 3, Val: v[i],
				Msg: "行列式为零：线性部分不可逆"}
		}
	}

	// 4. 代数分解 L = R·S·H。
	//    R=[cs -sn; sn cs]，S=[sx 0;0 sy]，H=[1 h;0 1]
	//    L = [cs*sx, cs*sx*h-sn*sy]
	//        [sn*sx, sn*sx*h+cs*sy]
	//    令 k=sy/sx：a=cs*sx, c=sn*sx, b=a*h-sn*sy, d=c*h+cs*sy
	//    b*c = a*c*h - sn*c*sy ; a*d = a*c*h + a*cs*sy
	//    => a*d-b*c = sy*(a*cs+sn*c) = sy*sx => sy = detL/sx
	//    约定 sx>0：旋转由 R 唯一承担，反射（detL<0）体现为 sy<0，
	//    避免 sx 取负时 R、S 同时翻转的分解歧义。
	sx := math.Hypot(a, c)
	cs := a / sx
	sn := c / sx
	sy := detL / sx
	// b = a*h - sn*sy => h = (b + sn*sy)/a，a=0 时改用 d 行。
	var h float64
	if math.Abs(a) >= math.Abs(c) {
		h = (b + sn*sy) / a
	} else {
		// d = c*h + cs*sy => h = (d - cs*sy)/c
		h = (d - cs*sy) / c
	}

	angle := math.Atan2(sn, cs)
	p := Parts{
		Translate: Translate(v[TX], v[TY]),
		Rotate:    Rotate(angle),
		Scale:     Scale(sx, sy),
		Shear:     Shear(h),
		Angle:     angle,
		SX:        sx,
		SY:        sy,
		SH:        h,
		TX:        v[TX],
		TY:        v[TY],
		Reflected: detL < 0,
	}
	return p, nil
}

// MaxAbsDiff 返回两矩阵逐元素最大绝对差。
func MaxAbsDiff(m, n Mat3) float64 {
	max := 0.0
	for i := range m.V {
		if d := math.Abs(m.V[i] - n.V[i]); d > max {
			max = d
		}
	}
	return max
}
