package pipe

import (
	"math"

	"vecgo/config"
	"vecgo/geom"
	"vecgo/post"
	"vecgo/pyramid"
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

// Result 在图像之外回报金字塔选层信息，供命令行与测试核对。
type Result struct {
	Image *raster.Image
	// PyramidEnabled 表示是否进入了金字塔重建阶段。
	PyramidEnabled bool
	// SelectedLevels 是实际用于重建的固定层数（金字塔关闭时为 0）。
	SelectedLevels int
	// Adaptive 表示层数是由 TargetError 解析解出，而非固定配置。
	Adaptive bool
	// Model 是自适应路径在当前图上拟合的衰减模型（固定路径也可能为 nil）。
	Model *pyramid.Model
}

func Run(scene [][]geom.Point, cfg config.Config) (*raster.Image, error) {
	r, err := RunDetailed(scene, cfg)
	if err != nil {
		return nil, err
	}
	return r.Image, nil
}

func RunDetailed(scene [][]geom.Point, cfg config.Config) (*Result, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	im, err := raster.New(cfg.Width, cfg.Height)
	if err != nil {
		return nil, err
	}
	moved := make([][]geom.Point, len(scene))
	for i, poly := range scene {
		moved[i] = Transform(poly, cfg.Scale, cfg.Rotate)
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

	res := &Result{Image: out}

	// 金字塔阶段：默认两个开关都为零，完全保留原有渲染路径。
	if cfg.PyramidLevels > 0 || cfg.TargetError > 0 {
		pyr, err := pyramid.FromImage(out)
		if err != nil {
			return nil, err
		}
		n := cfg.PyramidLevels
		if n > 0 {
			// 固定层数路径：直接使用配置给定的 n（越界由 Reconstruct 报错）。
			res.Adaptive = false
		} else {
			// 自适应路径：在当前图上标定每层衰减，再解析求最小层数。
			model := pyramid.FitDecay(pyr.LayerErrors())
			model.Layers = pyr.Depth
			n = model.MinLevels(cfg.TargetError)
			res.Adaptive = true
			res.Model = &model
		}
		rebuilt, err := pyr.Reconstruct(n)
		if err != nil {
			return nil, err
		}
		out = rebuilt
		res.PyramidEnabled = true
		res.SelectedLevels = n
		res.Image = out
	}
	return res, nil
}
