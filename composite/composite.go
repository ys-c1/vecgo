// Package composite 实现半透明图层的 Porter-Duff "over" 合成。
//
// 设计要点（完整结合律证明与实测数据见 ASSOCIATIVITY.md）：
//   - 预乘 alpha 下 over 在实数域满足结合律，因为 (e,a) 两个通道的递推
//     e_o = e_t + (1-a_t)e_b,  a_o = a_t + (1-a_t)a_b
//     展开后是同一条与括号无关的幂次和；
//   - 非预乘（straight / unassociated）表示每合成一对都要做一次
//     除回颜色 c = e/a 的量化，中间结果被舍入，结合律在整数管线下失效；
//   - 仅"改成预乘"但每对合成后仍量化回 8-bit 同样不结合（见测试），
//     根因是中间量化，而不是 straight 公式本身；
//   - 生产路径 Composite 求值与括号无关的闭式幂次和，所有图层只贡献
//     一次、唯一一次舍入发生在最终输出。它采用 float64 快速累加 +
//     舍入边界处 math/big 精确回退的双路径，输出与纯精确路径逐像素
//     恒等，任意括号顺序结果相同。
package composite

import (
	"errors"
	"math"
	"math/big"
)

// RGBA 是一个非预乘（straight alpha）的 8-bit 像素，通道取值 0..255。
type RGBA struct {
	R, G, B, A uint8
}

// Layer 是一张与目标画幅同尺寸的半透明图，自底向上排列。
type Layer struct {
	W, H int
	Pix  []RGBA
}

// NewLayer 申请一张 w×h 的透明图层。
func NewLayer(w, h int) (*Layer, error) {
	if w <= 0 || h <= 0 {
		return nil, errors.New("图层画幅必须为正")
	}
	return &Layer{W: w, H: h, Pix: make([]RGBA, w*h)}, nil
}

const scale = 255

var bigScale = big.NewInt(scale)

// frac 是约分后的正有理数 num/den，用于精确累加，杜绝浮点结合性问题。
type frac struct {
	n, d *big.Int
}

func newFrac(num, den int64) frac {
	f := frac{n: big.NewInt(num), d: big.NewInt(den)}
	f.reduce()
	return f
}

func (f frac) reduce() {
	g := new(big.Int).GCD(nil, nil, f.n, f.d)
	f.n.Quo(f.n, g)
	f.d.Quo(f.d, g)
}

// add 返回 a+b。
func (a frac) add(b frac) frac {
	r := frac{n: new(big.Int), d: new(big.Int)}
	// a.n/a.d + b.n/b.d = (a.n*b.d + b.n*a.d) / (a.d*b.d)
	t1 := new(big.Int).Mul(a.n, b.d)
	t2 := new(big.Int).Mul(b.n, a.d)
	r.n.Add(t1, t2)
	r.d.Mul(a.d, b.d)
	r.reduce()
	return r
}

// mul 返回 a*b。
func (a frac) mul(b frac) frac {
	r := frac{n: new(big.Int).Mul(a.n, b.n), d: new(big.Int).Mul(a.d, b.d)}
	r.reduce()
	return r
}

// subOne 返回 1-a。
func (a frac) subOne() frac {
	r := frac{n: new(big.Int).Sub(a.d, a.n), d: new(big.Int).Set(a.d)}
	r.reduce()
	return r
}

// round255 把 [0,1] 有理数四舍五入到 0..255。
func (a frac) round255() uint8 {
	if a.n.Sign() <= 0 {
		return 0
	}
	// round(a * 255) = floor( (2*a.n*255 + a.d) / (2*a.d) )
	num := new(big.Int).Mul(a.n, bigScale)
	num.Mul(num, big.NewInt(2))
	num.Add(num, a.d)
	den := new(big.Int).Mul(a.d, big.NewInt(2))
	v := new(big.Int).Quo(num, den)
	if v.Cmp(bigScale) > 0 {
		return 255
	}
	return uint8(v.Int64())
}

// roundInt 把非负有理数四舍五入到最近整数（用于已经带 255 量纲的颜色）。
func (a frac) roundInt() uint8 {
	if a.n.Sign() <= 0 {
		return 0
	}
	num := new(big.Int).Mul(a.n, big.NewInt(2))
	num.Add(num, a.d)
	v := new(big.Int).Quo(num, new(big.Int).Mul(a.d, big.NewInt(2)))
	if v.Cmp(bigScale) > 0 {
		return 255
	}
	return uint8(v.Int64())
}

// acc 是一个像素的预乘累加状态：三个颜色能量通道与覆盖率，全部为 [0,1] 分数。
type acc struct {
	er, eg, eb, aa frac
}

func zeroAcc() acc {
	z := newFrac(0, 1)
	return acc{er: z, eg: z, eb: z, aa: z}
}

// layerFrac 把一个 8-bit straight 像素转成预乘分数 (e=a*C/255^2, a)。
func layerFrac(p RGBA) acc {
	af := newFrac(int64(p.A), scale)
	mk := func(c uint8) frac {
		return newFrac(int64(c)*int64(p.A), scale*scale)
	}
	return acc{er: mk(p.R), eg: mk(p.G), eb: mk(p.B), aa: af}
}

// overAcc 是预乘空间的精确 over：top 盖在 bot 上，全程无量化。
func overAcc(top, bot acc) acc {
	k := top.aa.subOne() // 1-a_top
	return acc{
		er: top.er.add(k.mul(bot.er)),
		eg: top.eg.add(k.mul(bot.eg)),
		eb: top.eb.add(k.mul(bot.eb)),
		aa: top.aa.add(k.mul(bot.aa)),
	}
}

// finalize 是整条管线中唯一一次除回 straight 颜色与舍入。
func (s acc) finalize() RGBA {
	if s.aa.n.Sign() <= 0 {
		return RGBA{}
	}
	// straight 颜色 = (e/a)*255，全程 big.Int，不经过 int64/浮点。
	mk := func(e frac) uint8 {
		v := e.mul(newFrac(scale, 1)).mul(frac{n: new(big.Int).Set(s.aa.d), d: new(big.Int).Set(s.aa.n)})
		return v.roundInt()
	}
	return RGBA{R: mk(s.er), G: mk(s.eg), B: mk(s.eb), A: s.aa.round255()}
}

// Composite 按 layers[0] 在最底、layers[len-1] 在最顶的顺序合成，
// 返回一张非预乘 8-bit 图。
//
// 合成在预乘空间求值结合律证明中的闭式幂次和，中间不做任何舍入。
// 实现为双路径：
//   - 快速路径用 float64 累加器（典型像素误差远小于一个色阶）；
//   - 仅当某通道结果落在舍入半值附近、浮点结合误差可能翻转整数结果时，
//     该像素回退到 math/big 精确有理数路径。
//
// 因此输出与纯精确路径逐像素一致（测试对拍保证），同时保留 float64 的速度，
// 且 ((A over B) over C) 与 (A over (B over C)) 逐像素恒等。
func Composite(layers []*Layer) (*Layer, error) {
	if len(layers) == 0 {
		return nil, errors.New("至少需要一个图层")
	}
	w, h := layers[0].W, layers[0].H
	for _, l := range layers {
		if l.W != w || l.H != h || len(l.Pix) != w*h {
			return nil, errors.New("所有图层画幅必须一致")
		}
	}
	out, err := NewLayer(w, h)
	if err != nil {
		return nil, err
	}
	// 逐像素判断是否需要精确回退；exactCtx 无状态，可在像素间安全复用。
	exact := newExactCtx()
	for i := range out.Pix {
		// 快速路径：预乘 float64 累加器，自顶向下折叠。
		var fr, fg, fb, fa float64
		for k := len(layers) - 1; k >= 0; k-- {
			p := layers[k].Pix[i]
			af := float64(p.A) / 255.0
			kr := 1 - af
			fr = float64(p.R)/255.0*af + kr*fr
			fg = float64(p.G)/255.0*af + kr*fg
			fb = float64(p.B)/255.0*af + kr*fb
			fa = af + kr*fa
		}
		if exact.nearBoundary(fr, fg, fb, fa) {
			s := exact.fold(layers, i)
			out.Pix[i] = s
			continue
		}
		out.Pix[i] = finalizeFloat(fr, fg, fb, fa)
	}
	return out, nil
}

// halfMargin 是快速路径判定"离舍入边界足够远"的安全距离（色阶单位）。
// float64 累加 n 层的误差上界约 n·ε，对 n≤1000 仍远小于 1e-10，
// 取 1e-6 留有充足裕量。
const halfMargin = 1e-6

// exactCtx 标记精确回退路径；每次 fold 独立构造约分分数，无共享可变状态。
type exactCtx struct{}

func newExactCtx() *exactCtx { return &exactCtx{} }

// nearBoundary 判断任一待舍入值是否在整数半值的 halfMargin 邻域内，
// 或 alpha 是否接近 0（需要精确判定透明）。
func (c *exactCtx) nearBoundary(er, eg, eb, aa float64) bool {
	if aa < 1e-12 {
		return true
	}
	check := func(e float64) bool {
		v := e / aa * 255.0 // straight 颜色（色阶）
		frac := v - math.Floor(v)
		return math.Abs(frac-0.5) < halfMargin
	}
	if check(er) || check(eg) || check(eb) {
		return true
	}
	av := aa * 255.0
	af := av - math.Floor(av)
	return math.Abs(af-0.5) < halfMargin
}

// fold 对单个像素走精确有理数路径。
func (c *exactCtx) fold(layers []*Layer, i int) RGBA {
	s := zeroAcc()
	for k := len(layers) - 1; k >= 0; k-- {
		s = overAcc(layerFrac(layers[k].Pix[i]), s)
	}
	return s.finalize()
}

// finalizeFloat 把 float64 预乘累加结果转回非预乘 8-bit 像素，
// 半值向上舍入（与精确路径 frac.roundInt 规则一致）。
func finalizeFloat(er, eg, eb, aa float64) RGBA {
	if aa <= 0 {
		return RGBA{}
	}
	mk := func(e float64) uint8 {
		v := math.Floor(e/aa*255.0 + 0.5)
		if v < 0 {
			return 0
		}
		if v > 255 {
			return 255
		}
		return uint8(v)
	}
	av := math.Floor(aa*255.0 + 0.5)
	var a8 uint8
	if av >= 255 {
		a8 = 255
	} else if av < 0 {
		a8 = 0
	} else {
		a8 = uint8(av)
	}
	return RGBA{R: mk(er), G: mk(eg), B: mk(eb), A: a8}
}

// compositePixelFoldRight 以右结合方式递归折叠同一组像素，专供测试对照：
// A over (B over (C over ...))。生产路径与它必须给出相同输出。
func compositePixelFoldRight(px []RGBA) RGBA {
	var fold func(k int) acc
	fold = func(k int) acc {
		cur := layerFrac(px[k])
		if k == len(px)-1 {
			return cur
		}
		return overAcc(cur, fold(k+1))
	}
	return fold(0).finalize()
}

// ---- 以下为对照实现：逐步量化的朴素管线，仅用于在测试中复现结合律反例 ----

// OverStraight 是非预乘 over：每合成一对就除回 straight 颜色并量化到 8-bit。
// 全程整数定点、半值向上舍入，与 scratch/search4.py 的 over_st_step 同构。
func OverStraight(top, bot RGBA) RGBA {
	at, ab := int(top.A), int(bot.A)
	ao := (at*255 + (255-at)*ab) / 255
	if ao == 0 {
		return RGBA{}
	}
	mk := func(cT, cB uint8) uint8 {
		// 能量分子（1/255²）：a_t*c_t + (255-a_t)*a_b*c_b/255
		eNum := (at*int(cT)*255 + (255-at)*ab*int(cB) + 127) / 255
		// straight 颜色 = eNum/255 / (ao/255) * 255，半值向上
		co := (eNum*255 + (ao*255)/2) / (ao * 255)
		if co > 255 {
			co = 255
		}
		return uint8(co)
	}
	return RGBA{R: mk(top.R, bot.R), G: mk(top.G, bot.G), B: mk(top.B, bot.B), A: uint8(ao)}
}

// OverPremultStep 是逐步量化的预乘 over（每对合成后舍入回 8-bit 预乘值），
// 用于证明"预乘"本身不够，必须避免中间量化。整数定点、半值向上。
func OverPremultStep(top, bot RGBA) RGBA {
	pt := premult(top)
	pb := premult(bot)
	k := 255 - int(top.A)
	mk := func(t, b uint8) uint8 {
		v := (int(t)*255 + k*int(b) + 127) / 255
		if v > 255 {
			v = 255
		}
		return uint8(v)
	}
	pre := RGBA{
		R: mk(pt.R, pb.R), G: mk(pt.G, pb.G), B: mk(pt.B, pb.B),
		A: uint8((int(pt.A)*255 + k*int(pb.A)) / 255),
	}
	return straightFromPremult(pre)
}

func premult(p RGBA) RGBA {
	mk := func(c uint8) uint8 {
		return uint8((int(c)*int(p.A) + 127) / 255)
	}
	return RGBA{R: mk(p.R), G: mk(p.G), B: mk(p.B), A: p.A}
}

func straightFromPremult(p RGBA) RGBA {
	if p.A == 0 {
		return RGBA{}
	}
	a := int(p.A)
	mk := func(c uint8) uint8 {
		v := (int(c)*255 + a/2) / a
		if v > 255 {
			v = 255
		}
		return uint8(v)
	}
	return RGBA{R: mk(p.R), G: mk(p.G), B: mk(p.B), A: p.A}
}
