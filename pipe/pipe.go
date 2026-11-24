package pipe

import (
	"math"

	"vecgo/config"
	"vecgo/geom"
	"vecgo/post"
	"vecgo/raster"
	"vecgo/shading"
)

// TransformInto 把 pts 缩放旋转后写入 dst（长度需不小于 len(pts)）。
// 这是几何变换的唯一实现；Transform 只是为它分配目标切片的薄包装。
func TransformInto(dst, pts []geom.Point, scale, rotate float64) {
	c, s := math.Cos(rotate), math.Sin(rotate)
	for i, p := range pts {
		dst[i].X = scale * (p.X*c - p.Y*s)
		dst[i].Y = scale * (p.X*s + p.Y*c)
	}
}

// Transform 返回变换后的新切片，保留原有调用方式。
func Transform(pts []geom.Point, scale, rotate float64) []geom.Point {
	out := make([]geom.Point, len(pts))
	TransformInto(out, pts, scale, rotate)
	return out
}

// Run 跑完整管线。所有中间结果都在同一张工作图上就地推进：
//   - 变换后的顶点写进一块扁平缓冲，逐多边形取子切片，避免每个多边形各分配一次；
//   - 填充、着色、gamma 本来就改这张图；
//   - 模糊改用 BlurInto 就地完成，不再新建第二张整帧，其行环向这张图的预留区借用。
//
// 因此整幅处理只有一次整帧分配（像素 + 预留的模糊行环），峰值从“两张整帧”降到“一张整帧”。
func Run(scene [][]geom.Point, cfg config.Config) (*raster.Image, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	// 预留一条 (2*blur+1) 行的环形缓冲，模糊时直接借用，不再额外分配。
	scratch := 0
	if cfg.Blur > 0 {
		scratch = (2*cfg.Blur + 2) * cfg.Width
	}
	im, err := raster.NewWithScratch(cfg.Width, cfg.Height, scratch)
	if err != nil {
		return nil, err
	}

	total := 0
	for _, poly := range scene {
		total += len(poly)
	}
	flat := make([]geom.Point, total)
	moved := make([][]geom.Point, len(scene))
	off := 0
	for i, poly := range scene {
		n := len(poly)
		moved[i] = flat[off : off+n]
		TransformInto(moved[i], poly, cfg.Scale, cfg.Rotate)
		off += n
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
	if err := post.BlurInto(im, cfg.Blur); err != nil {
		return nil, err
	}
	return im, nil
}
