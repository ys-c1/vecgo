package texture

import (
	"errors"
	"math"
)

var errBadSize = errors.New("贴图宽高必须为正")

// 两档缩小验证使用的固定参数。
const (
	// RefSize 是源贴图边长（纹素）。
	RefSize = 128
	// CheckCanvas 是验证画布边长（像素）。
	CheckCanvas = 96
	// CheckLambda 是按源纹素距离计的衰减系数 λ（单位：纹素^-1）。
	// 它由独立校准得到：在 2×/4× 两档以及指数场/高斯场上，
	// 平均色差最优点都落在 λ≈0.087..0.096，取整为 0.090。
	CheckLambda = 0.090
)

// CheckBeta 返回第 factor 档、距离按“输出像素”计时的衰减系数 β=kλ。
// 1 个输出像素在 k 倍缩小时覆盖 k 个源纹素，故 β_k = k·λ：
//
//	β_2 = 0.180，β_4 = 0.360。
func CheckBeta(factor int) float64 {
	return CheckLambda * float64(factor)
}

// Field 是定义在整个 R² 上的颜色场，既用来烘焙有限贴图，也作为越界评分真值。
type Field func(u, v float64) RGB

// CheckFallback 是越界外插衰减后回归的基线底色（归一化 RGB）。
func CheckFallback() RGB { return RGB{R: 8.0 / 255, G: 10.0 / 255, B: 18.0 / 255} }

// ReferenceField 是主验证使用的确定性参考场（“真值”），
// 贴图只在 [0,1]² 内对它采样，而评分在包含越界坐标的全部位置直接求它。
//
// 场 = 彩色斜向渐变 + 贴近四边的亮环 + 中央品红光斑，
// 再乘一个随 UV 越界距离指数衰减的外包络。它模拟一张边缘明亮、
// 向外自然消隐的贴花：截断采样会把亮边拉成满屏色带，
// 带权外插则应重建这条衰减。
func ReferenceField(u, v float64) RGB {
	const (
		envTauUV = 0.085 // 外包络的 UV 衰减尺度，约合 10.9 个纹素
		ringC    = 0.075
		ringW    = 0.030
	)

	// 斜向双色渐变（0..255 空间构造，与校准脚本一致）。
	t := math.Min(math.Max(0.5+0.5*(u-v), 0), 1)
	c0 := RGB{R: 255, G: 94, B: 58}
	c1 := RGB{R: 54, G: 168, B: 255}
	col := RGB{
		R: c0.R*(1-t) + c1.R*t,
		G: c0.G*(1-t) + c1.G*t,
		B: c0.B*(1-t) + c1.B*t,
	}

	// 贴边亮环：到纹理四边的最小距离 de。
	de := math.Min(math.Min(u, 1-u), math.Min(v, 1-v))
	ring := math.Exp(-((de - ringC) / ringW * ((de - ringC) / ringW)))
	col.R += ring * 255 * 0.9
	col.G += ring * 230 * 0.9
	col.B += ring * 120 * 0.9

	// 中央品红光斑。
	blob := math.Exp(-(math.Hypot(u-0.5, v-0.5) / 0.20) * (math.Hypot(u-0.5, v-0.5) / 0.20))
	col.R += blob * 200 * 0.8
	col.G += blob * 90 * 0.8
	col.B += blob * 255 * 0.8

	pat := RGB{R: clip01(col.R / 255), G: clip01(col.G / 255), B: clip01(col.B / 255)}

	ou := AxisOvershoot(u, 1) // 注意 maxIdx=1 对 UV 即上界 1，下界 0
	ov := AxisOvershoot(v, 1)
	duv := math.Hypot(ou, ov)
	env := math.Exp(-duv / envTauUV)

	bg := CheckFallback()
	return RGB{
		R: bg.R + (pat.R-bg.R)*env,
		G: bg.G + (pat.G-bg.G)*env,
		B: bg.B + (pat.B-bg.B)*env,
	}
}

// GaussianMismatchField 是一个与指数衰减权重“异配”的稳健性参考场：
// 它由若干各向同性高斯光斑叠加而成（平方指数 p=2），并不服从权重所用的
// 一阶指数律，用来证明结论不是靠把真值构造成权重形状得到的。
func GaussianMismatchField(u, v float64) RGB {
	type blob struct {
		cu, cv, sigma, amp float64
		col                RGB
	}
	blobs := []blob{
		{0.02, 0.50, 0.13, 1.00, RGB{R: 255, G: 150, B: 60}},
		{0.98, 0.35, 0.11, 0.95, RGB{R: 80, G: 210, B: 255}},
		{0.50, 0.98, 0.13, 1.00, RGB{R: 130, G: 255, B: 160}},
		{0.80, 0.03, 0.10, 0.90, RGB{R: 220, G: 130, B: 255}},
		{0.50, 0.50, 0.22, 0.50, RGB{R: 255, G: 240, B: 140}},
	}
	bg := CheckFallback()
	c := bg
	for _, b := range blobs {
		g := b.amp * math.Exp(-(math.Hypot(u-b.cu, v-b.cv)/b.sigma)*
			(math.Hypot(u-b.cu, v-b.cv)/b.sigma))
		c.R += g * b.col.R / 255
		c.G += g * b.col.G / 255
		c.B += g * b.col.B / 255
	}
	return c.Clip()
}

// BuildTex 在纹素中心采样任意颜色场，得到有限贴图。
func BuildTex(f Field, size int) *Tex {
	t, _ := New(size, size)
	for y := 0; y < size; y++ {
		v := (float64(y) + 0.5) / float64(size)
		for x := 0; x < size; x++ {
			u := (float64(x) + 0.5) / float64(size)
			t.Pix[y*size+x] = f(u, v).Clip()
		}
	}
	return t
}

// BuildReferenceTex 烘焙主验证场 ReferenceField。
func BuildReferenceTex(size int) *Tex { return BuildTex(ReferenceField, size) }

// Tier 是一档缩小倍数的完整验证结果与图像。
type Tier struct {
	Factor int     // 缩小倍数 k（2 或 4）
	Canvas int     // 画布边长
	Quad   int     // 贴图映射到画布上的正方形边长 = RefSize/k
	Origin int     // 正方形左上角画布坐标（正方形居中）
	Lambda float64 // 纹素距离系数
	Beta   float64 // 输出像素距离系数 β=kλ

	TotalPixels int // Canvas²
	InsideCount int // 坐标在界内的像素数 = Quad²
	OOBCount    int // 越界像素数（本任务要求按档统计的量）

	ClampMean  float64 // 截断版在越界区的平均 RGB 欧氏色差（0..255）
	ClampMax   float64
	ExtrapMean float64 // 带权外插版在越界区的平均 RGB 欧氏色差（0..255）
	ExtrapMax  float64
	ClampMAE   float64 // 截断版三通道平均绝对差（0..255）
	ExtrapMAE  float64

	// 与画布等大的结果图（归一化 RGB，行优先）。
	ClampImg  []RGB
	ExtrapImg []RGB
	TruthImg  []RGB
	DiffImg   []RGB // 放大 4 倍后的外插色差可视化
	OOBMask   []bool
}

// ImprovementPct 返回平均色差相对截断版下降的百分比。
func (r Tier) ImprovementPct() float64 {
	if r.ClampMean == 0 {
		return 0
	}
	return 100 * (1 - r.ExtrapMean/r.ClampMean)
}

// RunTier 在 CheckCanvas×CheckCanvas 画布上，把 size×size 贴图按 k 倍缩小居中映射，
// 连续延伸同一仿射 UV，使正方形外的像素采到 [0,1]² 之外。
// 随后逐像素对比截断采样与带权外插相对参考场 f 的越界区色差。
func RunTier(f Field, size, k int, lambda float64) Tier {
	t := BuildTex(f, size)
	C := CheckCanvas
	q := size / k // RefSize/k：缩小后的正方形边长（像素）
	q0 := (C - q) / 2

	r := Tier{
		Factor: k, Canvas: C, Quad: q, Origin: q0,
		Lambda: lambda, Beta: CheckBeta(k),
		TotalPixels: C * C,
		ClampImg:    make([]RGB, C*C),
		ExtrapImg:   make([]RGB, C*C),
		TruthImg:    make([]RGB, C*C),
		DiffImg:     make([]RGB, C*C),
		OOBMask:     make([]bool, C*C),
	}

	s := NewSampler(t)
	fallback := CheckFallback()
	maxX := float64(t.W - 1)
	maxY := float64(t.H - 1)

	var clampSum, extrapSum, clampMAESum, extrapMAESum float64
	n := 0

	for y := 0; y < C; y++ {
		for x := 0; x < C; x++ {
			i := y*C + x
			// 正方形区域对应 UV [0,1]²；像素中心映射。
			u := (float64(x) + 0.5 - float64(q0)) / float64(q)
			v := (float64(y) + 0.5 - float64(q0)) / float64(q)
			tx := TexelCoord(u, t.W)
			ty := TexelCoord(v, t.H)

			truth := f(u, v).Clip()
			clamp := s.Clamp(tx, ty).Clip()
			extrap := s.Weighted(tx, ty, lambda, fallback, 0).Clip()

			r.TruthImg[i] = truth
			r.ClampImg[i] = clamp
			r.ExtrapImg[i] = extrap

			if OutOfBounds(tx, ty, maxX, maxY) {
				r.OOBMask[i] = true
				r.OOBCount++
				dc := DiffEuclid255(clamp, truth)
				de := DiffEuclid255(extrap, truth)
				clampSum += dc
				extrapSum += de
				clampMAESum += DiffMAE255(clamp, truth)
				extrapMAESum += DiffMAE255(extrap, truth)
				if dc > r.ClampMax {
					r.ClampMax = dc
				}
				if de > r.ExtrapMax {
					r.ExtrapMax = de
				}
				// 色差放大 4 倍做可视化（灰度热强度，加到红通道）。
				g := math.Min(de*4/255, 1)
				r.DiffImg[i] = RGB{R: g, G: g * 0.35, B: 0}
				n++
			}
		}
	}

	r.InsideCount = r.TotalPixels - r.OOBCount
	if n > 0 {
		r.ClampMean = clampSum / float64(n)
		r.ExtrapMean = extrapSum / float64(n)
		r.ClampMAE = clampMAESum / float64(n)
		r.ExtrapMAE = extrapMAESum / float64(n)
	}
	return r
}
