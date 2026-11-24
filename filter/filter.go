// Package filter 实现基于采样点雅可比的各向异性椭圆滤波（EWA 思路）。
//
// 斜向缩小时，屏幕上一个像素在纹理空间对应的采样足迹不是圆，而是被
// 拉伸的椭圆。各向同性（圆）核为了不混叠，只能按最长轴取半径，于是
// 短轴方向被过度模糊、高频糊成一片。本包由屏幕→纹理雅可比 J 推出
// 椭圆足迹的轴比与朝向，沿长轴/短轴分别取核，并保留各向同性路径兜底。
package filter

import (
	"errors"
	"math"
)

// Jac 是屏幕坐标 (x,y) 对纹理坐标 (u,v) 的雅可比矩阵
//
//	| du/dx  du/dy |   | a b |
//	| dv/dx  dv/dy | = | c d |
//
// 四个分量显式保留，便于由分量直接复算椭圆轴比与朝向。
type Jac struct {
	A, B, C, D float64
}

// Ellipse 是雅可比在纹理空间推出的像素采样足迹椭圆。
type Ellipse struct {
	Major      float64 // 半长轴（J 的最大奇异值，纹理单位）
	Minor      float64 // 半短轴（J 的最小奇异值，纹理单位）
	Theta      float64 // 长轴朝向角（弧度，相对纹理 u 轴）
	Ratio      float64 // 轴比 = 长轴 / 短轴，>=1
	A, B, C, D float64 // 推出该椭圆的雅可比四个分量
}

// Footprint 由雅可比四个分量求各向异性椭圆的轴比与朝向。
//
// 取 Gram 矩阵 G = JᵀJ，其特征值 λ 即 J 奇异值的平方：
//
//	G = | a²+c²  ab+cd |   （对称矩阵）
//	    | ab+cd  b²+d² |
//
// 半长轴 = sqrt(λmax)，半短轴 = sqrt(λmin)，轴比 = 长轴/短轴；
// 长轴朝向 θ = atan2(2g01, g00-g11)/2，atan2 全象限取值，
// g00==g11 的退化（各向同性）情形回退到 π/4 约定。
func Footprint(j Jac) (Ellipse, error) {
	g00 := j.A*j.A + j.C*j.C
	g01 := j.A*j.B + j.C*j.D
	g11 := j.B*j.B + j.D*j.D
	tr := g00 + g11
	det := g00*g11 - g01*g01
	if det <= 0 {
		return Ellipse{}, errors.New("雅可比退化，无法构造椭圆足迹")
	}
	disc := math.Max(tr*tr-4*det, 0)
	root := math.Sqrt(disc)
	lamMax := (tr + root) / 2
	lamMin := (tr - root) / 2
	major := math.Sqrt(lamMax)
	minor := math.Sqrt(lamMin)
	theta := math.Pi / 4
	if d := g00 - g11; math.Abs(d) > 1e-12 || math.Abs(g01) > 1e-12 {
		theta = math.Atan2(2*g01, d) / 2
	}
	return Ellipse{
		Major: major,
		Minor: minor,
		Theta: theta,
		Ratio: major / minor,
		A:     j.A, B: j.B, C: j.C, D: j.D,
	}, nil
}

// Kernel 是单个轴上的一维高斯核，σ 与截断支撑宽度（σ 的倍数）可分别设置。
type Kernel struct {
	Sigma float64
	Taps  int // 单侧采样点数
}

func gaussianKernel(sigma, support float64) Kernel {
	if sigma <= 0 {
		return Kernel{Sigma: 0, Taps: 0}
	}
	return Kernel{Sigma: sigma, Taps: int(math.Ceil(support*sigma)) + 1}
}

// Filter 是椭圆足迹上的可分离高斯滤波器。
// 长轴核 KM 与短轴核 Km 相互独立，可分别设置 σ（核形状随椭圆朝向旋转）。
type Filter struct {
	KM, Km  Kernel
	Theta   float64
	iso     bool
	support float64
}

// NewAniso 沿长、短轴分别取核：σ 与各自半轴成正比，比例系数相同。
func NewAniso(e Ellipse, support float64) (*Filter, error) {
	if support <= 0 {
		return nil, errors.New("核支撑宽度必须为正")
	}
	return &Filter{
		KM:      gaussianKernel(e.Major, support),
		Km:      gaussianKernel(e.Minor, support),
		Theta:   e.Theta,
		iso:     false,
		support: support,
	}, nil
}

// NewIso 是各向同性路径：圆核半径取长轴（最保守、保证不混叠），
// 两个轴上的 σ 相同。该路径刻意保留，用于对照与兜底。
func NewIso(e Ellipse, support float64) (*Filter, error) {
	if support <= 0 {
		return nil, errors.New("核支撑宽度必须为正")
	}
	k := gaussianKernel(e.Major, support)
	return &Filter{
		KM:      k,
		Km:      k,
		Theta:   e.Theta,
		iso:     true,
		support: support,
	}, nil
}

// IsIso 报告当前滤波器是否走各向同性（圆核）路径。
func (f *Filter) IsIso() bool { return f.iso }

// sample 在纹理 tex（尺寸 n×n，周期回绕）上对纹理点 (u,v) 按椭圆核加权。
// 核在随椭圆旋转的局部坐标 (p,q)（p 沿长轴、q 沿短轴）中可分离：
//
//	w(p,q) = exp(-p²/(2σM²)) · exp(-q²/(2σm²))
func (f *Filter) sample(tex []float64, n int, u, v float64) float64 {
	ct, st := math.Cos(f.Theta), math.Sin(f.Theta)
	rMax := f.KM.Taps
	rMin := f.Km.Taps
	var sum, wsum float64
	for i := -rMax; i <= rMax; i++ {
		p := float64(i)
		wp := 1.0
		if f.KM.Sigma > 0 {
			wp = math.Exp(-(p * p) / (2 * f.KM.Sigma * f.KM.Sigma))
		} else if i != 0 {
			continue
		}
		for j := -rMin; j <= rMin; j++ {
			q := float64(j)
			wq := 1.0
			if f.Km.Sigma > 0 {
				wq = math.Exp(-(q * q) / (2 * f.Km.Sigma * f.Km.Sigma))
			} else if j != 0 {
				continue
			}
			// 局部 (p,q) 旋转回纹理坐标：R(θ)·(p,q)
			du := ct*p - st*q
			dv := st*p + ct*q
			tu := wrap(int(math.Round(u+du)), n)
			tv := wrap(int(math.Round(v+dv)), n)
			w := wp * wq
			sum += w * tex[tv*n+tu]
			wsum += w
		}
	}
	if wsum == 0 {
		return tex[wrap(int(math.Round(v)), n)*n+wrap(int(math.Round(u)), n)]
	}
	return sum / wsum
}

func wrap(v, n int) int {
	v %= n
	if v < 0 {
		v += n
	}
	return v
}

// Warp 用给定滤波器把周期纹理 tex（n×n）按屏幕→纹理雅可比重采样到
// n×n 输出。屏幕像素 (x,y) 的纹理中心由 M^{-1} 给出，M 为纹理→屏幕
// 前向映射，平移分量不影响足迹，故取中心在原点。
func Warp(tex []float64, n int, j Jac, f *Filter) ([]float64, error) {
	if len(tex) != n*n {
		return nil, errors.New("纹理尺寸与数据长度不一致")
	}
	det := j.A*j.D - j.B*j.C
	if math.Abs(det) < 1e-12 {
		return nil, errors.New("雅可比不可逆")
	}
	out := make([]float64, n*n)
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			fx, fy := float64(x), float64(y)
			u := j.A*fx + j.B*fy
			v := j.C*fx + j.D*fy
			out[y*n+x] = f.sample(tex, n, u, v)
		}
	}
	return out, nil
}

// GroundTruth 用 ss×ss 超采样（盒式，无核宽）求每个屏幕像素的参考值，
// 作为 RMSE 与条纹（人工痕迹）判定的基准。
func GroundTruth(tex []float64, n int, j Jac, ss int) ([]float64, error) {
	if ss < 1 {
		return nil, errors.New("超采样数必须为正")
	}
	out := make([]float64, n*n)
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			var acc float64
			for sj := 0; sj < ss; sj++ {
				for si := 0; si < ss; si++ {
					fx := float64(x) + (float64(si)+0.5)/float64(ss)
					fy := float64(y) + (float64(sj)+0.5)/float64(ss)
					u := j.A*fx + j.B*fy
					v := j.C*fx + j.D*fy
					iu := wrap(int(math.Floor(u)), n)
					iv := wrap(int(math.Floor(v)), n)
					acc += tex[iv*n+iu]
				}
			}
			out[y*n+x] = acc / float64(ss*ss)
		}
	}
	return out, nil
}

// Checker 生成 n×n 双轴高频棋盘纹理，单元边长 cell 像素（周期 2·cell）。
func Checker(n, cell int) []float64 {
	tex := make([]float64, n*n)
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			if ((x/cell)+(y/cell))%2 == 0 {
				tex[y*n+x] = 255
			}
		}
	}
	return tex
}

// Energy 是高频能量度量（周期边界差分，避免边缘伪影）。
type Energy struct {
	Total float64 // 梯度总 RMS，高频能量标量
	X     float64 // 屏幕 x 方向梯度 RMS
	Y     float64 // 屏幕 y 方向梯度 RMS
	Ratio float64 // 方向能量比 X/Y，用于观察条纹（单方向异常偏高）
}

// HFEnergy 以相邻像素差分的均方根作为高频能量。
func HFEnergy(img []float64, n int) Energy {
	var sx, sy float64
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			dx := img[y*n+wrap(x+1, n)] - img[y*n+x]
			dy := img[wrap(y+1, n)*n+x] - img[y*n+x]
			sx += dx * dx
			sy += dy * dy
		}
	}
	m := float64(n * n)
	ex, ey := math.Sqrt(sx/m), math.Sqrt(sy/m)
	r := 0.0
	if ey > 1e-9 {
		r = ex / ey
	}
	return Energy{Total: math.Hypot(ex, ey), X: ex, Y: ey, Ratio: r}
}

// RMSE 计算两幅同尺寸图像的均方根误差，用于对照超采样基准量化条纹/伪影。
func RMSE(a, b []float64) float64 {
	var s float64
	for i := range a {
		d := a[i] - b[i]
		s += d * d
	}
	return math.Sqrt(s / float64(len(a)))
}
