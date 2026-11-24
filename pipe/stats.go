package pipe

import (
	"math"

	"vecgo/config"
	"vecgo/geom"
	"vecgo/raster"
)

// Stats 是一次运行中填充阶段的观测结果：既含解析理论值，也含实测值。
// 它不改变成像路径；Enabled 为 false 时 Run 完全不调用统计代码。
type Stats struct {
	Enabled bool
	// KeepPerPixel 打开后 FillStats 会保留逐像素重复次数与峰值位置。
	KeepPerPixel bool

	Fill raster.FillStats

	Width, Height int
	Supersample   int

	// 解析几何量（场景坐标，变换之后）。面积口径说明：
	// Fill 把 [minx,maxx]×[miny,maxy] 均匀铺到 W×H 个像素上，故每像素
	// 代表 CanvasArea/(W*H) 的场景面积；采样点落在像素内，每个像素有 ss² 个样本。
	CanvasW, CanvasH float64 // 映射跨度；包围盒在该轴为零时按 1 处理，与 Fill 一致
	CanvasArea       float64 // CanvasW*CanvasH
	PixelArea        float64 // CanvasArea/(W*H)，每像素对应的场景面积
	SamplesPerPixel  int     // ss²

	ShapeArea    []float64
	ShapeSumArea float64 // Σ 各图形面积（按覆盖重数计）
	UnionArea    float64 // 并集面积
	OverlapArea  float64 // Σ 面积 − 并集面积：被 k 个图形覆盖按 k−1 计权，即“重复覆盖面积”
	MultiArea    float64 // 被 ≥2 个图形同时覆盖的面积

	// 以下为面积占比（分母均为画幅面积 CanvasArea）。
	ShapeFraction   float64 // 写入面积占比：Σ 面积 / 画幅面积
	UnionFraction   float64 // 覆盖占比：并集面积 / 画幅面积
	OverlapFraction float64 // 重叠占比：重复覆盖面积 / 画幅面积
}

// RunWithStats 与 Run 成像完全一致，并额外返回填充阶段的统计数据。
func RunWithStats(scene [][]geom.Point, cfg config.Config, st *Stats) (*raster.Image, error) {
	return run(scene, cfg, st)
}

// --- 理论值：全部由解析重叠面积按“面积占比 × 样本数”推算，不含采样估计 ---

// TheoryPostsPerPixel 是每像素理论写入次数：写入面积占比 × 每像素样本数。
func (s *Stats) TheoryPostsPerPixel() float64 {
	return s.ShapeFraction * float64(s.SamplesPerPixel)
}

// TheoryRepeatPerPixel 是每像素理论重复绘制次数：重叠占比 × 每像素样本数。
func (s *Stats) TheoryRepeatPerPixel() float64 {
	return s.OverlapFraction * float64(s.SamplesPerPixel)
}

// TheoryPostsTotal 是全程理论写入次数（子采样口径）。
func (s *Stats) TheoryPostsTotal() float64 {
	return s.ShapeFraction * float64(s.totalSamples())
}

// TheoryRepeatTotal 是全程理论重复绘制次数（子采样口径）。
func (s *Stats) TheoryRepeatTotal() float64 {
	return s.OverlapFraction * float64(s.totalSamples())
}

// TheoryCoveredTotal 是被覆盖样本数的理论值。
func (s *Stats) TheoryCoveredTotal() float64 {
	return s.UnionFraction * float64(s.totalSamples())
}

func (s *Stats) totalSamples() int {
	return s.Width * s.Height * s.SamplesPerPixel
}

// --- 实测值：全部来自逐像素计数，再按像素数平均；不按图形平均 ---

// MeasuredActualPostsPerPixel 是实际成像路径的每像素写入次数。
// 实际 Fill 在首个包含该样本的图形处短路，所以每个被覆盖样本只写一次。
func (s *Stats) MeasuredActualPostsPerPixel() float64 {
	return s.perPixel(s.Fill.ActualPosts)
}

// MeasuredNaivePostsPerPixel 是不做短路时的每像素写入次数：
// 每个样本在每个包含它的图形上各写一次。这才是与重叠面积可比的口径。
func (s *Stats) MeasuredNaivePostsPerPixel() float64 {
	return s.perPixel(s.Fill.Posts)
}

// MeasuredNaiveRepeatPerPixel 是不做短路时每像素的重复绘制次数。
func (s *Stats) MeasuredNaiveRepeatPerPixel() float64 {
	return s.perPixel(s.Fill.ExtraPosts)
}

// MeasuredActualRepeatPerPixel 是实际成像路径的每像素重复绘制次数。
func (s *Stats) MeasuredActualRepeatPerPixel() float64 {
	return s.perPixel(s.Fill.ActualPosts - s.Fill.CoveredSamples)
}

func (s *Stats) perPixel(total int) float64 {
	if s.Width <= 0 || s.Height <= 0 {
		return 0
	}
	return float64(total) / float64(s.Width*s.Height)
}

// --- 理论/实测共用的占比 ---

// MeasuredCoveredFraction 是被覆盖样本占全部样本的比例，可与 UnionFraction 对比。
func (s *Stats) MeasuredCoveredFraction() float64 {
	if s.totalSamples() == 0 {
		return 0
	}
	return float64(s.Fill.CoveredSamples) / float64(s.totalSamples())
}

// MeasuredOverlapFraction 是被 ≥2 个图形覆盖的样本占比，可与 MultiArea/CanvasArea 对比。
func (s *Stats) MeasuredOverlapFraction() float64 {
	if s.totalSamples() == 0 {
		return 0
	}
	return float64(s.Fill.OverlapSamples) / float64(s.totalSamples())
}

// MultiFraction 是被 ≥2 个图形同时覆盖的面积占比。
func (s *Stats) MultiFraction() float64 {
	if s.CanvasArea == 0 {
		return 0
	}
	return s.MultiArea / s.CanvasArea
}

// RelDiff 返回 (measured−theory)/theory；theory 为 0 时返回 0。
func RelDiff(measured, theory float64) float64 {
	if theory == 0 {
		return 0
	}
	return (measured - theory) / theory
}

// theorize 计算与实测像素格网一致的理论量。像素格网由 Transform 后的场景
// 包围盒决定：Fill 把 [minx,maxx]×[miny,maxy] 均匀铺到 W×H 个像素上，
// 因此每像素对应的场景面积是 CanvasArea/(W*H)，与形状无关。
func (st *Stats) theorize(moved [][]geom.Point, cfg config.Config) error {
	st.Width, st.Height = cfg.Width, cfg.Height
	st.Supersample = cfg.Supersample
	st.SamplesPerPixel = cfg.Supersample * cfg.Supersample

	st.CanvasW, st.CanvasH = 0, 0
	if len(moved) > 0 {
		minx, miny := math.Inf(1), math.Inf(1)
		maxx, maxy := math.Inf(-1), math.Inf(-1)
		for _, p := range moved {
			for _, q := range p {
				minx = math.Min(minx, q.X)
				miny = math.Min(miny, q.Y)
				maxx = math.Max(maxx, q.X)
				maxy = math.Max(maxy, q.Y)
			}
		}
		st.CanvasW, st.CanvasH = maxx-minx, maxy-miny
		// Fill 对零跨度轴按 1 处理，理论画幅必须与之一致。
		if st.CanvasW == 0 {
			st.CanvasW = 1
		}
		if st.CanvasH == 0 {
			st.CanvasH = 1
		}
	}
	st.CanvasArea = st.CanvasW * st.CanvasH
	st.PixelArea = st.CanvasArea / float64(cfg.Width*cfg.Height)

	cov, err := geom.Measure(moved)
	if err != nil {
		return err
	}
	st.ShapeArea = cov.ShapeArea
	st.ShapeSumArea = cov.Sum
	st.UnionArea = cov.Union
	st.OverlapArea = cov.Excess
	st.MultiArea = cov.Multi
	if st.CanvasArea > 0 {
		st.ShapeFraction = st.ShapeSumArea / st.CanvasArea
		st.UnionFraction = st.UnionArea / st.CanvasArea
		st.OverlapFraction = st.OverlapArea / st.CanvasArea
	}
	return nil
}
