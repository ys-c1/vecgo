package shadow

import (
	"errors"
	"math"

	"vecgo/geom"
	"vecgo/raster"
)

// Options 控制统一软/硬阴影渲染。
//
// 软硬并不走两套代码：渲染器始终在面光源上布点，把每个点光源的硬阴影
// 累加平均。Size=0（点光源）时只取光源中心一个点，于是同一条路径自然
// 退化为硬阴影——这正是 L→0 极限在实现上的对应。
type Options struct {
	Width, Height int
	// PxPerUnit 是世界单位到输出像素的换算 s。
	PxPerUnit float64
	// SamplesX/SamplesY 是光源截面上的均匀网格数；Size=0 时强制为 1×1。
	// 线性光源可令 SamplesY=1；面状光源两轴都 >1。
	SamplesX, SamplesY int
	// Supersample 是每像素每光源点的规则超采样倍数 K（K×K 子点）。
	Supersample int
	// Lit/Dark 是接收面全亮/全遮时的灰度。
	Lit, Dark uint8
}

func (o *Options) fillDefaults() error {
	if o.Width <= 0 || o.Height <= 0 {
		return errors.New("shadow: Width/Height 必须为正")
	}
	if !(o.PxPerUnit > 0) {
		return errors.New("shadow: PxPerUnit 必须为正")
	}
	if o.SamplesX <= 0 {
		o.SamplesX = 1
	}
	if o.SamplesY <= 0 {
		o.SamplesY = 1
	}
	if o.Supersample <= 0 {
		o.Supersample = 1
	}
	if o.Supersample > 16 {
		return errors.New("shadow: Supersample 过大")
	}
	return nil
}

// LightPositions 返回光源上的确定性均匀（单元格中心）布点。
// Size=0 时无论 n 为何都只返回原点——点光源。
func LightPositions(size float64, n int) []float64 {
	if size == 0 {
		return []float64{0}
	}
	if n <= 1 {
		return []float64{0}
	}
	out := make([]float64, n)
	for i := 0; i < n; i++ {
		out[i] = -size/2 + (float64(i)+0.5)*size/float64(n)
	}
	return out
}

// Render 用统一的“多点硬阴影平均”路径渲染面积光阴影。
//
// polys 是遮挡物在接收面上的中心投影多边形（像素坐标）。
// 光源点 (ellX, ellY) 按相似三角形把它平移 EdgeShift 个世界单位，
// 再乘 PxPerUnit 换成像素。返回的 Image 灰度越暗表示遮挡越充分。
func Render(polys [][]geom.Point, geo Geometry, opt Options) (*raster.Image, error) {
	if err := geo.Validate(); err != nil {
		return nil, err
	}
	if err := opt.fillDefaults(); err != nil {
		return nil, err
	}
	for _, p := range polys {
		if len(p) < 3 {
			return nil, errors.New("shadow: 多边形至少三个顶点")
		}
	}

	// 点光源极限：L=0 时只保留中心采样点，调用方给的采样数被忽略。
	var xs, ys []float64
	if geo.Size == 0 {
		xs = []float64{0}
		ys = []float64{0}
	} else {
		xs = LightPositions(geo.Size, opt.SamplesX)
		ys = LightPositions(geo.Size, opt.SamplesY)
	}
	r := geo.ratio()

	im, err := raster.New(opt.Width, opt.Height)
	if err != nil {
		return nil, err
	}

	// acc 记录每个像素被多少“光源点 × 子采样点”判为遮挡。
	acc := make([]int, opt.Width*opt.Height)
	k := opt.Supersample
	sub := k * k

	for _, ellY := range ys {
		shiftY := -ellY * r * opt.PxPerUnit
		for _, ellX := range xs {
			shiftX := -ellX * r * opt.PxPerUnit
			shifted := shiftPolys(polys, shiftX, shiftY)
			for row := 0; row < opt.Height; row++ {
				for col := 0; col < opt.Width; col++ {
					hit := 0
					for sj := 0; sj < k; sj++ {
						for si := 0; si < k; si++ {
							px := float64(col) + (float64(si)+0.5)/float64(k)
							py := float64(row) + (float64(sj)+0.5)/float64(k)
							for _, p := range shifted {
								if geom.Inside(p, px, py) {
									hit++
									break
								}
							}
						}
					}
					acc[row*opt.Width+col] += hit
				}
			}
		}
	}

	total := len(xs) * len(ys) * sub
	lit, dark := float64(opt.Lit), float64(opt.Dark)
	for i, a := range acc {
		blocked := float64(a) / float64(total)
		v := lit + (dark-lit)*blocked
		im.Pix[i] = uint8(math.Round(v))
	}
	return im, nil
}

func shiftPolys(polys [][]geom.Point, dx, dy float64) [][]geom.Point {
	out := make([][]geom.Point, len(polys))
	for i, p := range polys {
		q := make([]geom.Point, len(p))
		for j, v := range p {
			q[j] = geom.Point{X: v.X + dx, Y: v.Y + dy}
		}
		out[i] = q
	}
	return out
}
