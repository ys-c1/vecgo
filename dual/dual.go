// Package dual 在同一场景上并行走整型（定点）与浮点两条渲染路径，
// 并通过两个可配项把中间结果对齐到同一精度网格：
//
//   - CoordBits：坐标定点小数位 Q。所有变换后的顶点、采样坐标都按 2^-Q 取整。
//   - ColorBits：颜色截断位深（1..8）。每一步产生的 8 位像素都按该位深截断。
//
// 三个引擎：
//   - IntEngine：全程 int64 定点（乘除用 128 位中间结果），无浮点参与。
//   - FloatEngine：float64 路径，但每个中间结果都量化到与整型路径相同的网格，
//     用于验证“同一精度取整 + 同一位深截断”后两条路径逐像素相同。
//   - NativeFloatEngine：不做任何中间量化的原生浮点路径，作为未对齐基线，
//     用来展示“边缘差一档”的差异并定位是哪一步精度不够。
package dual

import (
	"errors"
	"math"

	"vecgo/fx"
)

// Options 是双路径渲染的全部输入。
type Options struct {
	Width, Height int
	Scale         float64
	Rotate        float64
	Supersample   int
	Shading       string // "flat" 或 "gradient"
	Top, Bottom   float64
	Gamma         float64
	Blur          int
	// CoordBits 是坐标定点小数位 Q（建议 8..24），两条路径共用。
	CoordBits uint
	// ColorBits 是颜色截断位深（1..8），两条路径共用。
	ColorBits int
}

// Validate 校验参数范围。
func (o Options) Validate() error {
	if o.Width <= 0 || o.Height <= 0 {
		return errors.New("画幅必须为正")
	}
	if o.Scale <= 0 {
		return errors.New("scale 必须为正")
	}
	if o.Supersample < 1 || o.Supersample > 8 {
		return errors.New("supersample 超出范围")
	}
	if o.Shading != "flat" && o.Shading != "gradient" {
		return errors.New("shading 非法")
	}
	if o.Top < 0 || o.Top > 1 || o.Bottom < 0 || o.Bottom > 1 {
		return errors.New("着色强度超出范围")
	}
	if o.Gamma < 0.1 || o.Gamma > 10 {
		return errors.New("gamma 超出范围")
	}
	if o.Blur < 0 || o.Blur > 16 {
		return errors.New("blur 超出范围")
	}
	if o.CoordBits > 48 {
		return errors.New("coordBits 超出范围")
	}
	if o.ColorBits < 1 || o.ColorBits > 8 {
		return errors.New("colorBits 超出范围")
	}
	return nil
}

// Pix 是一张 8 位灰度图。
type Pix struct {
	W, H int
	V    []uint8
}

// Stage 记录一条路径在某一渲染阶段后的像素，用于逐步定位首个分歧点。
type Stage struct {
	Name string
	Pix  Pix
}

// Result 是一条路径的完整产物：最终像素、分阶段快照、变换后的顶点（Q 网格值）。
type Result struct {
	Engine string
	Stages []Stage
	Final  Pix
	// VertQ 是变换后顶点在 Q 网格上的整数表示，整型/对齐浮点路径应当一致。
	VertQ [][2]int64
}

// truncate 按位深截断像素：保留 ColorBits 个有效位，低位清零。
// 例如 ColorBits=6 时 255 -> 252，量化步长为 2^(8-ColorBits)。
func truncate(v uint8, colorBits int) uint8 {
	shift := uint(8 - colorBits)
	return (v >> shift) << shift
}

// snapFloat 把 float64 量化到 Q 网格再还原，模拟定点取整。
func snapFloat(v float64, q uint) float64 {
	return fx.FromFloat(v, q).Float(q)
}

// gammaLUT 构建 gamma 查找表，每个表项按 ColorBits 截断。
// gamma==1 时恒等；否则用 math.Pow 作为两条路径共同的参考实现。
func gammaLUT(gamma float64, colorBits int) [256]uint8 {
	var t [256]uint8
	for i := 0; i < 256; i++ {
		g := math.Pow(float64(i)/255, 1/gamma)
		t[i] = truncate(uint8(math.Round(255*g)), colorBits)
	}
	return t
}

// bbox 求多边形集合的包围盒。
func bbox(polys [][][2]int64) (minX, minY, maxX, maxY int64) {
	minX, minY = polys[0][0][0], polys[0][0][1]
	maxX, maxY = minX, minY
	for _, p := range polys {
		for _, q := range p {
			if q[0] < minX {
				minX = q[0]
			}
			if q[0] > maxX {
				maxX = q[0]
			}
			if q[1] < minY {
				minY = q[1]
			}
			if q[1] > maxY {
				maxY = q[1]
			}
		}
	}
	return
}
