package pipe

import (
	"math"

	"vecgo/budget"
	"vecgo/config"
	"vecgo/geom"
	"vecgo/post"
	"vecgo/raster"
	"vecgo/shading"
)

func Transform(pts []geom.Point, scale, rotate float64) []geom.Point {
	c, s := math.Cos(rotate), math.Sin(rotate)
	out := make([]geom.Point, len(pts))
	for i, p := range pts {
		out[i] = geom.Point{}
		out[i].X = scale * (p.X*c - p.Y*s)
		out[i].Y = scale * (p.X*s + p.Y*c)
	}
	return out
}

func Run(scene [][]geom.Point, cfg config.Config) (*raster.Image, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	// 五处系数在灰度域统一分配余量, 所有位点只能乘用表里下发的系数。
	plan := budget.New(cfg.Gain, cfg.Gamma, 1.0)

	im, err := raster.New(cfg.Width, cfg.Height)
	if err != nil {
		return nil, err
	}
	moved := make([][]geom.Point, len(scene))
	for i, poly := range scene {
		// 几何变换只动坐标; 其灰度域系数登记在预算表的 pipe-transform 行。
		moved[i] = Transform(poly, cfg.Scale, cfg.Rotate)
	}
	// 填充触发逻辑保持原样, 填充系数由预算表分配 (默认 1.0, 超预算时与其余
	// 线性位点等比压缩)。
	if err := im.Fill(moved, cfg.Supersample); err != nil {
		return nil, err
	}
	// shading 会覆写非零像素, 因此把 config 增益、几何变换、填充三处的线性
	// 系数折叠进 shading 的触发入参; 触发公式本身 (255*top 与渐变形状) 不变。
	linear := plan.Coefficient(budget.ConfigGain) *
		plan.Coefficient(budget.Xform) *
		plan.Coefficient(budget.RasterFill) *
		plan.Coefficient(budget.Shading)
	if err := shading.Apply(im, cfg.Shading, linear, 0.0); err != nil {
		return nil, err
	}
	// 伽马触发逻辑与 LUT 保持原样, 其提亮所需余量已由预算表在上游压缩预留。
	if err := post.Gamma(im, cfg.Gamma); err != nil {
		return nil, err
	}
	out, err := post.Blur(im, cfg.Blur)
	if err != nil {
		return nil, err
	}
	return out, nil
}
