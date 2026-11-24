package regress

import (
	"errors"

	"vecgo/raster"
)

// 差异归因的三条判定规则（按差异像素的聚集形态，逐条对应一类偏差）：
//
//	判定 1 —— 填充漏了（fill）：差异几乎纯单边（丢失与新增之比失衡，
//	  balance < 0.1）且聚成实心块（solidity >= 0.5）。
//	  某片区域整体没填上：墨只朝一个方向消失，且内部致密。
//	判定 2 —— 变换偏了（transform）：单点幅度大（meanMag >= 128）。
//	  图形整体移位，旧边丢墨、新边添墨，像素在 0 与 255 之间整个翻转。
//	  注意不能要求双侧对称：本管线按包围盒归一化，旋转会放大包围盒，
//	  净丢失天然多于净新增。
//	判定 3 —— 后处理糊了（post）：其余情况，即双侧且单点幅度小
//	  （meanMag < 128）。模糊/伽马类后处理在每条边缘周围留下
//	  一圈低幅度的薄晕，方向混杂、单点变化轻微。
const (
	CategoryNone      = "none"
	CategoryFill      = "fill"
	CategoryTransform = "transform"
	CategoryPost      = "post"
)

// Label 返回归因类别的中文说明。
func Label(category string) string {
	switch category {
	case CategoryNone:
		return "无差异"
	case CategoryFill:
		return "填充漏了"
	case CategoryTransform:
		return "变换偏了"
	case CategoryPost:
		return "后处理糊了"
	}
	return "未知"
}

// DiffPixel 记录一个差异像素的位置与前后取值。
type DiffPixel struct {
	X, Y      int
	Base, Cur uint8
}

// Report 是两图逐像素比较的结果。
type Report struct {
	Count    int         // 差异像素总数
	Lost     int         // 基准有墨、当前变浅的像素数
	Gained   int         // 基准无墨、当前变深的像素数
	MeanMag  float64     // 差异像素的平均幅度 |base-cur|
	Solidity float64     // 差异像素 8 邻域中同为差异的平均占比（1=实心块）
	Top      []DiffPixel // 前 10 处差异（行优先序）
	Category string      // 归因类别，见上方三条判定
}

// Diff 逐像素比较两幅图，返回差异统计、前 10 处位置与归因。
func Diff(base, cur *raster.Image) (*Report, error) {
	if base.W != cur.W || base.H != cur.H {
		return nil, errors.New("画幅不一致，无法差分")
	}
	rep := &Report{}
	mask := make([]bool, len(base.Pix))
	magSum := 0
	for i := range base.Pix {
		b, c := base.Pix[i], cur.Pix[i]
		if b == c {
			continue
		}
		rep.Count++
		d := int(b) - int(c)
		if d > 0 {
			rep.Lost++
		} else {
			rep.Gained++
			d = -d
		}
		magSum += d
		mask[i] = true
		if len(rep.Top) < 10 {
			rep.Top = append(rep.Top, DiffPixel{X: i % base.W, Y: i / base.W, Base: b, Cur: c})
		}
	}
	if rep.Count > 0 {
		rep.MeanMag = float64(magSum) / float64(rep.Count)
		rep.Solidity = solidity(mask, base.W, base.H)
	}
	rep.Category = Attribute(rep)
	return rep, nil
}

// Attribute 按包文档里的三条判定对差异报告归因。
func Attribute(rep *Report) string {
	if rep.Count == 0 {
		return CategoryNone
	}
	hi, lo := rep.Lost, rep.Gained
	if lo > hi {
		hi, lo = lo, hi
	}
	balance := float64(lo) / float64(hi) // hi > 0 因为 Count > 0
	// 判定 1：纯单边 + 实心块 → 填充漏了
	if balance < 0.1 && rep.Solidity >= 0.5 {
		return CategoryFill
	}
	// 判定 2：高幅度翻转 → 变换偏了
	if rep.MeanMag >= 128 {
		return CategoryTransform
	}
	// 判定 3：双侧 + 低幅度 → 后处理糊了
	return CategoryPost
}

// solidity 计算差异掩码的实心度：每个差异像素的 8 邻域中同为差异的平均占比。
func solidity(mask []bool, w, h int) float64 {
	sum := 0.0
	n := 0
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if !mask[y*w+x] {
				continue
			}
			n++
			on, total := 0, 0
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					if dx == 0 && dy == 0 {
						continue
					}
					nx, ny := x+dx, y+dy
					if nx < 0 || nx >= w || ny < 0 || ny >= h {
						continue
					}
					total++
					if mask[ny*w+nx] {
						on++
					}
				}
			}
			if total > 0 {
				sum += float64(on) / float64(total)
			}
		}
	}
	if n == 0 {
		return 0
	}
	return sum / float64(n)
}
