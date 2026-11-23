package raster

import (
	"errors"
	"fmt"
	"math"

	"vecgo/geom"
)

// View 把像素网格映射到场景坐标的一个矩形视口。
type View struct {
	MinX, MinY float64
	MaxX, MaxY float64
}

func (v View) valid() bool {
	if !(v.MaxX > v.MinX) || !(v.MaxY > v.MinY) {
		return false
	}
	for _, f := range []float64{v.MinX, v.MinY, v.MaxX, v.MaxY} {
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return false
		}
	}
	return true
}

// AdaptiveStats 记录一次自适应采样的样本账。
type AdaptiveStats struct {
	ProbeSamples int // 梯度探测消耗的样本数
	PixelSamples int // 像素合成消耗的样本数
	MaxPerPixel  int // 单像素实际用到的最大样本数
}

// Total 返回总样本数（探测 + 合成）。
func (s AdaptiveStats) Total() int { return s.ProbeSamples + s.PixelSamples }

func cover(polys [][]geom.Point, x, y float64) int {
	for _, p := range polys {
		if geom.Inside(p, x, y) {
			return 1
		}
	}
	return 0
}

func checkScene(polys [][]geom.Point) error {
	if len(polys) == 0 {
		return errors.New("图是空的：一个多边形也没有")
	}
	for i, p := range polys {
		if len(p) < 3 {
			return fmt.Errorf("图是空的：多边形 %d 只有 %d 个顶点", i, len(p))
		}
		for j, q := range p {
			if math.IsNaN(q.X) || math.IsNaN(q.Y) || math.IsInf(q.X, 0) || math.IsInf(q.Y, 0) {
				return fmt.Errorf("起点非法：多边形 %d 的顶点 %d 坐标 (%v, %v) 不是有限数", i, j, q.X, q.Y)
			}
		}
	}
	return nil
}

func checkCanvas(im *Image) error {
	if im == nil || im.W <= 0 || im.H <= 0 || len(im.Pix) < im.W*im.H {
		return errors.New("空图不能采样")
	}
	return nil
}

// FillUniform 对视口内每个像素做 ss×ss 统一采样，作为自适应的基准。
// 原有 Fill（按多边形包围盒映射）保持不变，这里用显式视口。
func (im *Image) FillUniform(polys [][]geom.Point, v View, ss int) error {
	if err := checkCanvas(im); err != nil {
		return err
	}
	if err := checkScene(polys); err != nil {
		return err
	}
	if !v.valid() {
		return fmt.Errorf("视口非法: %+v", v)
	}
	if ss < 1 {
		return errors.New("supersample 必须为正")
	}
	den := ss * ss
	sx := (v.MaxX - v.MinX) / float64(im.W)
	sy := (v.MaxY - v.MinY) / float64(im.H)
	for row := 0; row < im.H; row++ {
		for col := 0; col < im.W; col++ {
			hit := 0
			for sj := 0; sj < ss; sj++ {
				for si := 0; si < ss; si++ {
					px := v.MinX + (float64(col)+(float64(si)+0.5)/float64(ss))*sx
					py := v.MinY + (float64(row)+(float64(sj)+0.5)/float64(ss))*sy
					hit += cover(polys, px, py)
				}
			}
			im.Pix[row*im.W+col] = uint8(255 * hit / den)
		}
	}
	return nil
}

// FillAdaptive 按局部梯度自适应分配样本，再合成像素值。
//
// 梯度→样本数换算式推导：设像素内覆盖率的变化范围为 g（用半像素探测
// 网格上该像素 3×3 探测点的极差估计，它近似 |∇c|·像素尺寸）。把像素
// 覆盖率估计看作 n 个独立样本的均值；由 Popoviciu 不等式，取值范围
// 为 g 的随机变量方差至多 g²/4，故均值的标准误 ≤ g/(2√n)。要求标准误
// 不超过容差 tol，解得
//
//	n ≥ (g / (2·tol))²
//
// 为保持分层网格形状，把 n 向上取成平方数 k²，即 k = ceil(g/(2·tol))，
// 并夹在 [1, kmax]，kmax = floor(√maxPerPixel)。g=0 的平坦像素退化为
// k=1，只采 1 个样本。
//
// 采样位置用旋转分层格（n-rooks）：第 (si,sj) 个样本取
//
//	x = (si·k + sj + 0.5)/k²,  y = (sj·k + si + 0.5)/k²   （像素内归一化）
//
// x、y 方向各占满 k² 个不同坐标，于是轴对齐边（规则 k×k 网格的退化情形，
// 法向只有 k 个不同坐标、最坏误差 1/k）的最坏误差降到 1/n，斜边仍保持
// 分层覆盖。
func (im *Image) FillAdaptive(polys [][]geom.Point, v View, tol float64, maxPerPixel int) (*AdaptiveStats, error) {
	if err := checkCanvas(im); err != nil {
		return nil, err
	}
	if err := checkScene(polys); err != nil {
		return nil, err
	}
	if !v.valid() {
		return nil, fmt.Errorf("视口非法: %+v", v)
	}
	if tol <= 0 || math.IsNaN(tol) || math.IsInf(tol, 0) {
		return nil, fmt.Errorf("容差必须为正: %v", tol)
	}
	if maxPerPixel <= 0 {
		return nil, fmt.Errorf("样本上限为 %d：像素 (0,0) 连一个样本都分不到", maxPerPixel)
	}
	kmax := int(math.Sqrt(float64(maxPerPixel)))
	if kmax < 1 {
		kmax = 1
	}

	// 第一步：半像素探测网格 (2W+1)×(2H+1)，相邻像素共享探测点。
	pw, ph := 2*im.W+1, 2*im.H+1
	probe := make([]uint8, pw*ph)
	sx := (v.MaxX - v.MinX) / float64(im.W)
	sy := (v.MaxY - v.MinY) / float64(im.H)
	for j := 0; j < ph; j++ {
		py := v.MinY + float64(j)*0.5*sy
		for i := 0; i < pw; i++ {
			px := v.MinX + float64(i)*0.5*sx
			probe[j*pw+i] = uint8(cover(polys, px, py))
		}
	}

	// 第二步：每像素梯度 = 该像素 3×3 探测点的极差。
	grad := make([]float64, im.W*im.H)
	anyGradient := false
	for row := 0; row < im.H; row++ {
		for col := 0; col < im.W; col++ {
			lo, hi := uint8(1), uint8(0)
			for dj := 0; dj < 3; dj++ {
				for di := 0; di < 3; di++ {
					p := probe[(2*row+dj)*pw+2*col+di]
					if p < lo {
						lo = p
					}
					if p > hi {
						hi = p
					}
				}
			}
			g := float64(hi - lo)
			grad[row*im.W+col] = g
			if g > 0 {
				anyGradient = true
			}
		}
	}
	if !anyGradient {
		return nil, fmt.Errorf("梯度处处为零：从像素 (0,0) 到 (%d,%d) 局部梯度全为 0，自适应分配无从谈起", im.W-1, im.H-1)
	}

	// 第三步：按换算式定 k，用 k×k 分层网格分配采样位置并合成。
	stats := &AdaptiveStats{ProbeSamples: pw * ph}
	const eps = 1e-9
	for row := 0; row < im.H; row++ {
		for col := 0; col < im.W; col++ {
			k := 1
			if g := grad[row*im.W+col]; g > 0 {
				k = int(math.Ceil(g / (2 * tol)))
				if k < 1 {
					k = 1
				}
				if k > kmax {
					k = kmax
				}
			}
			n := k * k
			if n > stats.MaxPerPixel {
				stats.MaxPerPixel = n
			}
			hit := 0
			for sj := 0; sj < k; sj++ {
				for si := 0; si < k; si++ {
					fx := (float64(si*k+sj) + 0.5) / float64(n)
					fy := (float64(sj*k+si) + 0.5) / float64(n)
					px := v.MinX + (float64(col)+fx)*sx
					py := v.MinY + (float64(row)+fy)*sy
					if px < v.MinX-eps || px > v.MaxX+eps || py < v.MinY-eps || py > v.MaxY+eps {
						return nil, fmt.Errorf("目标不可达：像素 (%d,%d) 的采样点 (%v, %v) 落在视口之外", col, row, px, py)
					}
					hit += cover(polys, px, py)
				}
			}
			stats.PixelSamples += n
			im.Pix[row*im.W+col] = uint8(255 * hit / n)
		}
	}
	return stats, nil
}
