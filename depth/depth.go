// Package depth 提供按像素深度消隐（z-buffer）的图层渲染。
//
// 为什么深度要按透视除法归一、不能直接用屏幕坐标比较：
// 屏幕坐标 (x, y) 是投影之后的结果，投影把深度维度压扁了——
// 不同远近的表面可以投到同一个像素，屏幕坐标本身不含任何远近信息，
// 在屏幕坐标里找不到可比较的量。裁剪空间里的 z 也不能直接比：
// 透视投影下 w 随距离逐顶点变化，两个顶点的 z 只有在各自的 w
// 下才有意义。只有经过透视除法 d = z/w 得到的归一化设备深度
// 才与视距单调对应，并且 d 在屏幕空间是线性的，可以按顶点插值
// 到每个像素。因此本包对每个顶点先做 d = z/w，再按屏幕位置
// 线性插值得到逐像素深度，按深度值决定保留哪一层；深度相同的
// 像素按图层号定序，小号图层在前。
package depth

import (
	"errors"
	"fmt"
	"math"

	"vecgo/geom"
	"vecgo/raster"
)

// Vertex 是带齐次深度的顶点，(X, Y) 为世界坐标，Z/W 为齐次深度。
type Vertex struct{ X, Y, Z, W float64 }

// Layer 是一个半透明图层：一个多边形 + 亮度 + 不透明度。
// Alpha 为 0 表示全透明，不参与深度竞争，也不遮挡后面的图层。
type Layer struct {
	Poly  []Vertex
	Shade uint8
	Alpha float64
}

// Validate 校验全部图层，任一图层非法即返回带图层号的错误。
func Validate(layers []Layer) error {
	for i, l := range layers {
		if len(l.Poly) < 3 {
			return fmt.Errorf("图层 %d 为空: 顶点数 %d 不足 3", i, len(l.Poly))
		}
		dmin, dmax := math.Inf(1), math.Inf(-1)
		for _, v := range l.Poly {
			d := v.Z / v.W
			if math.IsNaN(v.Z) || math.IsNaN(v.W) || math.IsNaN(d) {
				return fmt.Errorf("图层 %d 深度为 NaN", i)
			}
			if d < dmin {
				dmin = d
			}
			if d > dmax {
				dmax = d
			}
		}
		if dmax-dmin == 0 {
			return fmt.Errorf("图层 %d 深度范围为零", i)
		}
		if math.IsNaN(l.Alpha) || l.Alpha < 0 || l.Alpha > 1 {
			return fmt.Errorf("图层 %d alpha 超出范围: %v", i, l.Alpha)
		}
	}
	return nil
}

// plane 是屏幕空间里的线性深度函数 d = a*x + b*y + c。
// 归一化深度 d = z/w 在屏幕空间是线性的，所以用三个不共线
// 顶点拟合平面即可对任意像素精确求值。
type plane struct{ a, b, c float64 }

func (p plane) at(x, y float64) float64 { return p.a*x + p.b*y + p.c }

func fitPlane(poly []Vertex) plane {
	d := make([]float64, len(poly))
	for i, v := range poly {
		d[i] = v.Z / v.W
	}
	p0 := poly[0]
	for j := 1; j+1 < len(poly); j++ {
		for k := j + 1; k < len(poly); k++ {
			p1, p2 := poly[j], poly[k]
			det := (p1.X-p0.X)*(p2.Y-p0.Y) - (p2.X-p0.X)*(p1.Y-p0.Y)
			if math.Abs(det) < 1e-12 {
				continue
			}
			a := ((d[j]-d[0])*(p2.Y-p0.Y) - (d[k]-d[0])*(p1.Y-p0.Y)) / det
			b := ((p1.X-p0.X)*(d[k]-d[0]) - (p2.X-p0.X)*(d[j]-d[0])) / det
			return plane{a: a, b: b, c: d[0] - a*p0.X - b*p0.Y}
		}
	}
	// 顶点共线时多边形覆盖为零，深度函数不会被查询，给常量兜底。
	return plane{a: 0, b: 0, c: d[0]}
}

func toPoints(poly []Vertex) []geom.Point {
	pts := make([]geom.Point, len(poly))
	for i, v := range poly {
		pts[i] = geom.Point{X: v.X, Y: v.Y}
	}
	return pts
}

// setup 做校验并准备每个图层的平面与覆盖多边形。
func setup(layers []Layer) ([]plane, [][]geom.Point, error) {
	if err := Validate(layers); err != nil {
		return nil, nil, err
	}
	planes := make([]plane, len(layers))
	polys := make([][]geom.Point, len(layers))
	for i, l := range layers {
		planes[i] = fitPlane(l.Poly)
		polys[i] = toPoints(l.Poly)
	}
	return planes, polys, nil
}

func bounds(layers []Layer) (minx, miny, spanx, spany float64) {
	minx, miny = layers[0].Poly[0].X, layers[0].Poly[0].Y
	maxx, maxy := minx, miny
	for _, l := range layers {
		for _, v := range l.Poly {
			if v.X < minx {
				minx = v.X
			}
			if v.X > maxx {
				maxx = v.X
			}
			if v.Y < miny {
				miny = v.Y
			}
			if v.Y > maxy {
				maxy = v.Y
			}
		}
	}
	spanx, spany = maxx-minx, maxy-miny
	if spanx == 0 {
		spanx = 1
	}
	if spany == 0 {
		spany = 1
	}
	return minx, miny, spanx, spany
}

// Render 用 z-buffer 消隐把图层画进 im：每个采样点取归一化深度
// 最小的可见图层，深度相同取图层号小的。全透明图层（Alpha==0）
// 跳过。绘制顺序不影响结果。原有顺序绘制接口 raster.Image.Fill
// 不受影响。
func Render(im *raster.Image, layers []Layer, supersample int) error {
	if supersample < 1 {
		return errors.New("supersample 必须为正")
	}
	planes, polys, err := setup(layers)
	if err != nil {
		return err
	}
	if len(layers) == 0 {
		return nil
	}
	minx, miny, spanx, spany := bounds(layers)
	den := supersample * supersample
	for row := 0; row < im.H; row++ {
		for col := 0; col < im.W; col++ {
			sum := 0.0
			for sj := 0; sj < supersample; sj++ {
				for si := 0; si < supersample; si++ {
					px := minx + (float64(col)+(float64(si)+0.5)/float64(supersample))*spanx/float64(im.W)
					py := miny + (float64(row)+(float64(sj)+0.5)/float64(supersample))*spany/float64(im.H)
					best := -1
					bestD := 0.0
					for i := range layers {
						if layers[i].Alpha <= 0 {
							continue
						}
						if !geom.Inside(polys[i], px, py) {
							continue
						}
						d := planes[i].at(px, py)
						// 严格小于才替换：深度相同保留图层号小的。
						if best < 0 || d < bestD {
							best, bestD = i, d
						}
					}
					if best >= 0 {
						sum += float64(layers[best].Shade) * layers[best].Alpha
					}
				}
			}
			im.Pix[row*im.W+col] = uint8(sum/float64(den) + 0.5)
		}
	}
	return nil
}

// RenderReference 是逐像素排序参照：每个采样点把覆盖它的图层
// 按 (深度, 图层号) 排序后取最前者。实现刻意独立于 Render，
// 用于验证消隐结果完全一致。
func RenderReference(im *raster.Image, layers []Layer, supersample int) error {
	if supersample < 1 {
		return errors.New("supersample 必须为正")
	}
	planes, polys, err := setup(layers)
	if err != nil {
		return err
	}
	if len(layers) == 0 {
		return nil
	}
	minx, miny, spanx, spany := bounds(layers)
	den := supersample * supersample
	type hit struct {
		d   float64
		idx int
	}
	for row := 0; row < im.H; row++ {
		for col := 0; col < im.W; col++ {
			sum := 0.0
			for sj := 0; sj < supersample; sj++ {
				for si := 0; si < supersample; si++ {
					px := minx + (float64(col)+(float64(si)+0.5)/float64(supersample))*spanx/float64(im.W)
					py := miny + (float64(row)+(float64(sj)+0.5)/float64(supersample))*spany/float64(im.H)
					hits := make([]hit, 0, len(layers))
					for i := range layers {
						if layers[i].Alpha <= 0 {
							continue
						}
						if !geom.Inside(polys[i], px, py) {
							continue
						}
						hits = append(hits, hit{d: planes[i].at(px, py), idx: i})
					}
					for k := 1; k < len(hits); k++ {
						for m := k; m > 0 && (hits[m].d < hits[m-1].d ||
							(hits[m].d == hits[m-1].d && hits[m].idx < hits[m-1].idx)); m-- {
							hits[m], hits[m-1] = hits[m-1], hits[m]
						}
					}
					if len(hits) > 0 {
						sum += float64(layers[hits[0].idx].Shade) * layers[hits[0].idx].Alpha
					}
				}
			}
			im.Pix[row*im.W+col] = uint8(sum/float64(den) + 0.5)
		}
	}
	return nil
}

// Diff 返回两幅图不同像素的个数。
func Diff(a, b *raster.Image) (int, error) {
	if a.W != b.W || a.H != b.H {
		return 0, fmt.Errorf("画幅不一致: %dx%d 对 %dx%d", a.W, a.H, b.W, b.H)
	}
	n := 0
	for i := range a.Pix {
		if a.Pix[i] != b.Pix[i] {
			n++
		}
	}
	return n, nil
}

// Depths 返回图层每个顶点透视除法归一后的深度 d = z/w，用于报告。
func Depths(l Layer) []float64 {
	out := make([]float64, len(l.Poly))
	for i, v := range l.Poly {
		out[i] = v.Z / v.W
	}
	return out
}
