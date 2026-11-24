// Package pyramid 实现 2×2 均值（盒式）图像金字塔、逐层残差统计与
// “目标误差 → 最小层数”的解析求解。
//
// 金字塔 G_0 为原图，G_{k+1} 是 G_k 的 2×2 块均值（奇数尺寸取可用样本）。
// 细节层 D_k = G_k - Up2(G_{k+1})。用 n 层做由粗到精重建时，从最粗层
// 出发补入 n-1 个细节层；n=Depth 时逐像素精确还原。
//
// 均值金字塔加块常量重建等价于 Haar 多分辨率分解，各细节子带能量近似
// 正交，因此总重建误差满足
//
//	E(n)^2 ≈ Σ δ_j^2 ,
//
// 其中 δ_j 是按由粗到精编号的细节层 RMS。实测 δ_j 按固定比率 q 几何衰减
// （δ_j = d_1 q^{j-1}），代入平方尾和即可推出 E(n)=A q^n，进而反解最小
// 层数。详见 REPORT.md。
package pyramid

import (
	"errors"
	"math"

	"vecgo/raster"
)

// Pyramid 保存浮点亮度域的均值金字塔与细节残差。
type Pyramid struct {
	W, H  int           // 原图尺寸（G_0）
	Depth int           // 金字塔数组层数 L
	Ws    []int         // 各层宽
	Hs    []int         // 各层高
	G     [][]float64   // 均值层 G_k
	D     [][]float64   // 细节层 D_k，k=0..Depth-2；最粗层为 nil
	src   []float64     // 原始亮度，长度 W*H
	srcIm *raster.Image // 原始 8 位图
}

// MaxDepth 返回 w×h 图反复 2 倍均值直到 1×1 所需的金字塔层数（含原图）。
func MaxDepth(w, h int) int {
	if w <= 0 || h <= 0 {
		return 0
	}
	l := 1
	for w > 1 || h > 1 {
		w = (w + 1) / 2
		h = (h + 1) / 2
		l++
	}
	return l
}

// FromImage 等价于 Build(im, 0)：建到 1×1 的完整金字塔。
func FromImage(im *raster.Image) (*Pyramid, error) {
	return Build(im, 0)
}

// Build 由 8 位灰度图构建金字塔。depth<=0 表示取最大层数；
// depth 超过该尺寸允许的最大层数时返回错误（避免产生重复的 1×1 层）。
func Build(im *raster.Image, depth int) (*Pyramid, error) {
	if im == nil || im.W <= 0 || im.H <= 0 {
		return nil, errors.New("金字塔需要非空正尺寸图像")
	}
	maxD := MaxDepth(im.W, im.H)
	if depth <= 0 {
		depth = maxD
	}
	if depth < 1 {
		return nil, errors.New("金字塔层数至少为 1")
	}
	if depth > maxD {
		return nil, errDepthTooDeep(depth, maxD)
	}
	src := make([]float64, im.W*im.H)
	for i, v := range im.Pix {
		src[i] = float64(v)
	}
	ws := make([]int, depth)
	hs := make([]int, depth)
	copy(ws, repeatDims(im.W, depth))
	copy(hs, repeatDims(im.H, depth))

	G := make([][]float64, depth)
	G[0] = src
	ws[0], hs[0] = im.W, im.H
	for k := 1; k < depth; k++ {
		pw, ph := ws[k-1], hs[k-1]
		nw, nh := (pw+1)/2, (ph+1)/2
		ws[k], hs[k] = nw, nh
		prev := G[k-1]
		cur := make([]float64, nw*nh)
		for y := 0; y < nh; y++ {
			for x := 0; x < nw; x++ {
				x1 := minInt(2*x+1, pw-1)
				y1 := minInt(2*y+1, ph-1)
				var sum, cnt float64
				for yy := 2 * y; yy <= y1; yy++ {
					base := yy * pw
					for xx := 2 * x; xx <= x1; xx++ {
						sum += prev[base+xx]
						cnt++
					}
				}
				cur[y*nw+x] = sum / cnt
			}
		}
		G[k] = cur
	}

	D := make([][]float64, depth)
	for k := 0; k < depth-1; k++ {
		up := UpSample(G[k+1], ws[k+1], hs[k+1], ws[k], hs[k], false)
		d := make([]float64, ws[k]*hs[k])
		for i := range d {
			d[i] = G[k][i] - up[i]
		}
		D[k] = d
	}

	return &Pyramid{
		W: im.W, H: im.H, Depth: depth,
		Ws: ws, Hs: hs, G: G, D: D, src: src, srcIm: im,
	}, nil
}

func errDepthTooDeep(depth, maxD int) error {
	return &DepthError{Depth: depth, MaxDepth: maxD}
}

// DepthError 表示请求层数超过图像可支撑的最大层数。
type DepthError struct {
	Depth    int
	MaxDepth int
}

func (e *DepthError) Error() string {
	return "金字塔层数超出该尺寸上限"
}

func repeatDims(v, n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = v
		v = (v + 1) / 2
	}
	return out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// UpSample 把 w×h 的亮度层放大到 nw×nh。bi=false 用与 2×2 均值网格
// 对齐的最近邻复制（floor 映射）；bi=true 用半像素中心双线性。
func UpSample(a []float64, w, h, nw, nh int, bi bool) []float64 {
	out := make([]float64, nw*nh)
	if !bi {
		for y := 0; y < nh; y++ {
			sy := y * h / nh
			for x := 0; x < nw; x++ {
				sx := x * w / nw
				out[y*nw+x] = a[sy*w+sx]
			}
		}
		return out
	}
	for y := 0; y < nh; y++ {
		fy := (float64(y)+0.5)*float64(h)/float64(nh) - 0.5
		sy := int(math.Floor(fy))
		ty := fy - float64(sy)
		y0 := clampInt(sy, 0, h-1)
		y1 := clampInt(sy+1, 0, h-1)
		for x := 0; x < nw; x++ {
			fx := (float64(x)+0.5)*float64(w)/float64(nw) - 0.5
			sx := int(math.Floor(fx))
			tx := fx - float64(sx)
			x0 := clampInt(sx, 0, w-1)
			x1 := clampInt(sx+1, 0, w-1)
			v00, v10 := a[y0*w+x0], a[y0*w+x1]
			v01, v11 := a[y1*w+x0], a[y1*w+x1]
			top := v00*(1-tx) + v10*tx
			bot := v01*(1-tx) + v11*tx
			out[y*nw+x] = top*(1-ty) + bot*ty
		}
	}
	return out
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// ReconstructFloat 是固定层数路径的浮点实现：用恰好 levels 层做由粗到精
// 重建。levels=1 只保留最粗层，levels=Depth 精确还原。
func (p *Pyramid) ReconstructFloat(levels int) ([]float64, error) {
	if levels < 1 || levels > p.Depth {
		return nil, &DepthError{Depth: levels, MaxDepth: p.Depth}
	}
	k := p.Depth - 1
	cur := append([]float64(nil), p.G[k]...)
	cw, ch := p.Ws[k], p.Hs[k]
	used := 1
	for k = p.Depth - 2; k >= 0 && used < levels; k-- {
		up := UpSample(cur, cw, ch, p.Ws[k], p.Hs[k], false)
		d := p.D[k]
		for i := range up {
			up[i] += d[i]
		}
		cur, cw, ch = up, p.Ws[k], p.Hs[k]
		used++
	}
	if cw != p.W || ch != p.H {
		cur = UpSample(cur, cw, ch, p.W, p.H, false)
	}
	return cur, nil
}

// Reconstruct 是固定层数路径的 8 位图输出：在浮点重建上做四舍五入与
// [0,255] 截断。levels=Depth 时与原图逐字节一致。
func (p *Pyramid) Reconstruct(levels int) (*raster.Image, error) {
	f, err := p.ReconstructFloat(levels)
	if err != nil {
		return nil, err
	}
	im, err := raster.New(p.W, p.H)
	if err != nil {
		return nil, err
	}
	for i, v := range f {
		im.Pix[i] = clampUint8(math.Round(v))
	}
	return im, nil
}

func clampUint8(v float64) uint8 {
	if v <= 0 {
		return 0
	}
	if v >= 255 {
		return 255
	}
	return uint8(v)
}

// RMS 计算两段等长亮度样本的均方根误差（灰度级，0..255）。
func RMS(a, b []float64) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return math.NaN()
	}
	var s float64
	for i := range a {
		d := a[i] - b[i]
		s += d * d
	}
	return math.Sqrt(s / float64(len(a)))
}

// ImageRMSE 计算两张 8 位图之间的均方根灰度误差。
func ImageRMSE(a, b *raster.Image) (float64, error) {
	if a == nil || b == nil || a.W != b.W || a.H != b.H || len(a.Pix) != len(b.Pix) {
		return 0, errors.New("RMSE 需要同尺寸非空图像")
	}
	var s float64
	for i := range a.Pix {
		d := float64(a.Pix[i]) - float64(b.Pix[i])
		s += d * d
	}
	return math.Sqrt(s / float64(len(a.Pix))), nil
}

// DetailRMS 返回细节层 D_k 原生分辨率上的 RMS。
func (p *Pyramid) DetailRMS(k int) float64 {
	if k < 0 || k >= p.Depth-1 || p.D[k] == nil {
		return 0
	}
	return RMS(p.D[k], make([]float64, len(p.D[k])))
}

// LayerErrors 按“由粗到精”顺序返回细节层 RMS：δ_1 对应最粗的细节层
// D_{Depth-2}，δ_M（M=Depth-1）对应最细的 D_0。
func (p *Pyramid) LayerErrors() []float64 {
	m := p.Depth - 1
	out := make([]float64, m)
	for j := 1; j <= m; j++ {
		k := p.Depth - 1 - j
		out[j-1] = p.DetailRMS(k)
	}
	return out
}
