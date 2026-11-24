package kernel

import (
	"math"

	"vecgo/raster"
)

// DiskCapFraction 返回单位圆盘在方向 z 上 “z ≥ h” 的球冠面积占比：
//
//	q(h) = (acos h - h·sqrt(1-h²)) / π,  h∈[-1,1]
//
// 它正是均匀椭圆核与直边卷积后的覆盖率：把椭圆沿长短轴仿射成单位圆，
// 直边到圆心的法向距离归一化为 h。
func DiskCapFraction(h float64) float64 {
	if h <= -1 {
		return 1
	}
	if h >= 1 {
		return 0
	}
	return (math.Acos(h) - h*math.Sqrt(1-h*h)) / math.Pi
}

// CapRectangleBound 是 DiskCapFraction(h)（h≥0 时的小球冠）的一个严格上界。
// 球冠高 k=1-h、最大半宽 sqrt(1-h²)，整个球冠可被
// 宽 2sqrt(1-h²)、高 k 的外接矩形包住，故
//
//	q(h) ≤ 2(1-h)sqrt(1-h²)/π。
func CapRectangleBound(h float64) float64 {
	if h <= 0 {
		return 0.5 // h=0 时球冠就是半圆
	}
	if h >= 1 {
		return 0
	}
	return 2 * (1 - h) * math.Sqrt(1-h*h) / math.Pi
}

// NormalRadius 是椭圆核沿法向角 normalAngle 的支撑半径：
//
//	w = sqrt(a² cos²(φ-α) + b² sin²(φ-α))
//
// 其中 φ 是长轴朝向，α 是边缘法线朝向。
func NormalRadius(e Ellipse, normalAngle float64) float64 {
	d := e.Angle - normalAngle
	c, s := math.Cos(d), math.Sin(d)
	return math.Sqrt(e.Long*e.Long*c*c + e.Short*e.Short*s*s)
}

// Coverage 给出法向有向距离 d（亮侧为正）处，椭圆核覆盖亮半平面的比例。
// edgeNormal 为亮侧法线方向角。
func Coverage(e Ellipse, edgeNormal, d float64) float64 {
	w := NormalRadius(e, edgeNormal)
	if w <= 1e-15 {
		if d >= 0 {
			return 1
		}
		return 0
	}
	// 亮条件 n·r ≥ -d，仿射到单位圆后等价于 z ≥ -d/w。
	return DiskCapFraction(-d / w)
}

// RenderVerticalEdge 用解析覆盖率渲染一条位于列 edgeX（像素中心坐标）的
// 竖直黑白直边：左侧 0、右侧 255。每个像素值为四舍五入后的 uint8。
// 结果是真实的 raster.Image，可直接 SavePGM。
func RenderVerticalEdge(w, h int, edge Ellipse, edgeX float64) (*raster.Image, error) {
	im, err := raster.New(w, h)
	if err != nil {
		return nil, err
	}
	const normal = 0 // x 轴为亮侧法线
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			d := float64(x) - edgeX
			cov := Coverage(edge, normal, d)
			v := math.Round(255 * cov)
			if v < 0 {
				v = 0
			} else if v > 255 {
				v = 255
			}
			im.Pix[y*w+x] = uint8(v)
		}
	}
	return im, nil
}

// EdgeMetrics 是直边两侧灰阶与灰雾面积的实测结果，全部从量化后的像素统计。
type EdgeMetrics struct {
	Width             int     `json:"width"`
	Height            int     `json:"height"`
	EdgeX             float64 `json:"edge_x"`
	NormalRadius      float64 `json:"normal_radius"`
	DarkProbeGray     float64 `json:"dark_probe_gray"`     // 暗侧平台探测列（距边 1.5px）平均灰
	BrightProbeGray   float64 `json:"bright_probe_gray"`   // 亮侧平台探测列平均灰
	DarkSideDev       float64 `json:"dark_side_dev"`       // 暗侧相对纯黑的偏差
	BrightSideDev     float64 `json:"bright_side_dev"`     // 亮侧相对纯白的偏差
	MaxSideDev        float64 `json:"max_side_dev"`        // 两侧最大灰度差（题目的核心指标）
	TwoSideGap        float64 `json:"two_side_gap"`        // 两侧残差的不对称差 |devDark-devBright|
	Contrast          float64 `json:"contrast"`            // 两平台之间的跨边对比度（应接近 255）
	FogPixels         int     `json:"fog_pixels"`          // 灰雾面积（像素数）
	FogLevelThreshold int     `json:"fog_level_threshold"` // 偏差超过几档算灰雾（2 => >2）
}

// MeasureEdge 从已渲染图像实测边缘指标。
//
// 灰雾的操作定义（事先写明、可复算）：落在正常抗锯齿边对（|d|<1px 的两列）
// 之外、却仍偏离本侧纯色超过 fogLevel 档的像素。正常抗锯齿边列不计入灰雾。
func MeasureEdge(im *raster.Image, edge Ellipse, edgeX float64, fogLevel int) EdgeMetrics {
	m := EdgeMetrics{
		Width: im.W, Height: im.H, EdgeX: edgeX,
		NormalRadius:      NormalRadius(edge, 0),
		FogLevelThreshold: fogLevel,
	}
	probeDark := int(math.Round(edgeX - 1.5))
	probeBright := int(math.Round(edgeX + 1.5))
	var sumDark, sumBright int
	for y := 0; y < im.H; y++ {
		if probeDark >= 0 && probeDark < im.W {
			sumDark += int(im.Pix[y*im.W+probeDark])
		}
		if probeBright >= 0 && probeBright < im.W {
			sumBright += int(im.Pix[y*im.W+probeBright])
		}
	}
	if im.H > 0 {
		m.DarkProbeGray = float64(sumDark) / float64(im.H)
		m.BrightProbeGray = float64(sumBright) / float64(im.H)
	}
	m.DarkSideDev = m.DarkProbeGray
	m.BrightSideDev = 255 - m.BrightProbeGray
	m.MaxSideDev = math.Max(m.DarkSideDev, m.BrightSideDev)
	m.TwoSideGap = math.Abs(m.DarkSideDev - m.BrightSideDev)
	m.Contrast = m.BrightProbeGray - m.DarkProbeGray

	for y := 0; y < im.H; y++ {
		for x := 0; x < im.W; x++ {
			d := math.Abs(float64(x) - edgeX)
			if d < 1.0 {
				continue // 正常抗锯齿边对
			}
			v := int(im.Pix[y*im.W+x])
			var dev int
			if float64(x) < edgeX {
				dev = v // 暗侧：越亮越是雾
			} else {
				dev = 255 - v // 亮侧：越暗越是雾
			}
			if dev > fogLevel {
				m.FogPixels++
			}
		}
	}
	return m
}

// GrayErrorBound 给出距边 d（d≥0）处暗侧灰阶污染的严格上界（0..255）。
// 用球冠外接矩形界：U = 255·2(1-d/w)sqrt(1-(d/w)²)/π，d≥w 时为 0。
func GrayErrorBound(d, radius float64) float64 {
	if radius <= 0 || d >= radius {
		return 0
	}
	h := d / radius
	return 255 * CapRectangleBound(h)
}
