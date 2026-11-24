package pipe

import (
	"fmt"
	"math"

	"vecgo/config"
	"vecgo/geom"
	"vecgo/param"
	"vecgo/post"
	"vecgo/raster"
	"vecgo/shading"
)

// Transform 对折线做缩放与旋转。scale、rotate 须为有限数（scale 还须在
// (0, 100]），pts 坐标须有限；否则返回 *param.Error。
func Transform(pts []geom.Point, scale, rotate float64) ([]geom.Point, error) {
	if err := param.All(
		param.FloatInOpenLo("pipe.Transform.scale", scale, param.ScaleMin, param.ScaleMax),
		param.Finite("pipe.Transform.rotate", rotate),
		param.Points("pipe.Transform.pts", len(pts), 0, func(i int) (float64, float64) {
			return pts[i].X, pts[i].Y
		}),
	); err != nil {
		return nil, err
	}
	c, s := math.Cos(rotate), math.Sin(rotate)
	out := make([]geom.Point, len(pts))
	for i, p := range pts {
		out[i] = geom.Point{}
		out[i].X = scale * (p.X*c - p.Y*s)
		out[i].Y = scale * (p.X*s + p.Y*c)
	}
	return out, nil
}

// Run 执行完整管线：配置校验 → 画布 → 变换 → 填充 → 着色 → 伽马 → 模糊。
// 任何一步的非法参数都会以 *param.Error 返回，错误字段指明所属包与入口。
func Run(scene [][]geom.Point, cfg config.Config) (*raster.Image, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if err := param.AtLeast("pipe.Run.scene", len(scene), param.FillMinPolys); err != nil {
		return nil, err
	}
	for i, poly := range scene {
		if err := param.Polygon(fmt.Sprintf("pipe.Run.scene[%d]", i), len(poly), func(j int) (float64, float64) {
			return poly[j].X, poly[j].Y
		}); err != nil {
			return nil, err
		}
	}
	im, err := raster.New(cfg.Width, cfg.Height)
	if err != nil {
		return nil, err
	}
	moved := make([][]geom.Point, len(scene))
	for i, poly := range scene {
		moved[i], err = Transform(poly, cfg.Scale, cfg.Rotate)
		if err != nil {
			return nil, err
		}
	}
	if err := im.Fill(moved, cfg.Supersample); err != nil {
		return nil, err
	}
	if err := shading.Apply(im, cfg.Shading, 1.0, 0.0); err != nil {
		return nil, err
	}
	if err := post.Gamma(im, cfg.Gamma); err != nil {
		return nil, err
	}
	out, err := post.Blur(im, cfg.Blur)
	if err != nil {
		return nil, err
	}
	return out, nil
}
