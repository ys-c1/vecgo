package dual

import (
	"errors"

	"vecgo/fx"
)

// IntEngine 是纯整型路径：除入口处把输入参数量化到 Q 网格外，全程不使用浮点。
type IntEngine struct{}

// Run 执行整型渲染管线，返回分阶段快照。
func (IntEngine) Run(scene [][][2]float64, o Options) (*Result, error) {
	if err := o.Validate(); err != nil {
		return nil, err
	}
	q := o.CoordBits
	one := int64(1) << q

	// 阶段 0：变换参数与顶点全部量化到 Q 网格（唯一一次接触浮点）。
	cQ := fx.FromFloat(cos(o.Rotate), q)
	sQ := fx.FromFloat(sin(o.Rotate), q)
	scaleQ := fx.FromFloat(o.Scale, q)
	polys := make([][][2]int64, len(scene))
	vertQ := make([][2]int64, 0, len(scene)*4)
	for i, poly := range scene {
		pp := make([][2]int64, len(poly))
		for j, v := range poly {
			x := fx.FromFloat(v[0], q)
			y := fx.FromFloat(v[1], q)
			// x' = scale*(x*c - y*s)，每个乘法按 Q 截断，与浮点路径同方向。
			xc := fx.MulShr(x, cQ, q)
			ys := fx.MulShr(y, sQ, q)
			xs := fx.MulShr(x, sQ, q)
			yc := fx.MulShr(y, cQ, q)
			rx := fx.MulShr(scaleQ, xc-ys, q)
			ry := fx.MulShr(scaleQ, xs+yc, q)
			pp[j] = [2]int64{int64(rx), int64(ry)}
			vertQ = append(vertQ, pp[j])
		}
		polys[i] = pp
	}

	// 阶段 1：光栅化覆盖度（0..255）。
	cov, err := intRaster(polys, o.Width, o.Height, o.Supersample, q)
	if err != nil {
		return nil, err
	}
	stages := []Stage{{Name: "coverage", Pix: cov}}

	// 阶段 2：着色，输出立即按 ColorBits 截断。
	shade := intShade(cov, o, q, one)
	stages = append(stages, Stage{Name: "shaded", Pix: shade})

	// 阶段 3：gamma（共享查找表，表项已按 ColorBits 截断）。
	lut := gammaLUT(o.Gamma, o.ColorBits)
	gammed := mapPix(shade, func(v uint8) uint8 { return lut[v] })
	stages = append(stages, Stage{Name: "gamma", Pix: gammed})

	// 阶段 4：盒式模糊（整数均值，向零截断）。
	final := intBlur(gammed, o.Blur)
	stages = append(stages, Stage{Name: "blur", Pix: final})
	return &Result{Engine: "integer", Stages: stages, Final: final, VertQ: vertQ}, nil
}

// intRaster 用定点采样坐标与 128 位叉积做点在多边形内判定。
func intRaster(polys [][][2]int64, w, h, ss int, q uint) (Pix, error) {
	out := Pix{W: w, H: h, V: make([]uint8, w*h)}
	minX, minY, maxX, maxY := bbox(polys)
	spanX, spanY := maxX-minX, maxY-minY
	one := int64(1) << q
	if spanX == 0 {
		spanX = one
	}
	if spanY == 0 {
		spanY = one
	}
	den := int64(ss * ss)

	for row := 0; row < h; row++ {
		for col := 0; col < w; col++ {
			hit := 0
			for sj := 0; sj < ss; sj++ {
				for si := 0; si < ss; si++ {
					// px = minx + (col + (si+0.5)/ss) * spanx / W
					// 偏移量 (col*ss + si + 0.5)/ss 先在 Q+1 精度上拼出半格，
					// 再与 span 做 128 位乘法后右移。
					oxNum := int64(col*ss+si)*2 + 1 // 分子（单位 1/(2*ss)）
					px, err := sampleCoord(minX, oxNum, int64(ss), spanX, int64(w), q)
					if err != nil {
						return Pix{}, err
					}
					oyNum := int64(row*ss+sj)*2 + 1
					py, err := sampleCoord(minY, oyNum, int64(ss), spanY, int64(h), q)
					if err != nil {
						return Pix{}, err
					}
					if insideAny(polys, px, py) {
						hit++
					}
				}
			}
			out.V[row*w+col] = uint8(255 * int64(hit) / den)
		}
	}
	return out, nil
}

// sampleCoord 计算 min + (num/(2*ssStep)) * span / dim 的定点结果。
//
// 目标（span 为 Q 定点）：
//
//	off = span * num / (2*ssStep*dim)
//
// 拆成两次 128 位除法避免单次溢出：
//  1. mid = span*num / (2*ssStep)，仍为 Q 定点；
//  2. off = mid / dim（同精度整数除法，向零截断）。
//
// 与浮点路径的 (col+(si+0.5)/ss)*span/W 逐操作对应，舍入方向同为向零。
func sampleCoord(min, num, ssStep, span, dim int64, q uint) (int64, error) {
	hi, lo := mul128(span, num)
	mid := fx.Div128(hi, lo, 2*ssStep) // span * num / (2*ss)，Q 精度
	off, err := safeDiv128(0, mid, dim)
	if err != nil {
		return 0, err
	}
	return min + off, nil
}

// insideAny 判定定点点 (px,py) 是否落在任一多边形内（射线法）。
func insideAny(polys [][][2]int64, px, py int64) bool {
	for _, poly := range polys {
		n := len(poly)
		inside := false
		for i := 0; i < n; i++ {
			ax, ay := poly[i][0], poly[i][1]
			bx, by := poly[(i+1)%n][0], poly[(i+1)%n][1]
			if (ay > py) == (by > py) {
				continue
			}
			// 原浮点式：ax + (bx-ax)*(py-ay)/(by-ay) > px
			// 两侧同乘 (by-ay)，按其符号决定不等号方向，全程叉积无除法。
			lhsHi, lhsLo := mul128(bx-ax, py-ay)
			// ax*(by-ay) 与左侧相加，构造 128 位和。
			ah, al := mul128(ax, by-ay)
			sumHi, sumLo := add128(ah, al, lhsHi, lhsLo)
			rhsHi, rhsLo := mul128(px, by-ay)
			cmp := cmp128(sumHi, sumLo, rhsHi, rhsLo)
			if by-ay > 0 {
				if cmp > 0 {
					inside = !inside
				}
			} else if cmp < 0 {
				inside = !inside
			}
		}
		if inside {
			return true
		}
	}
	return false
}

// intShade 是定点着色：flat 为常量；gradient 按行做定点插值。
func intShade(cov Pix, o Options, q uint, one int64) Pix {
	out := Pix{W: cov.W, H: cov.H, V: make([]uint8, len(cov.V))}
	topQ := fx.FromFloat(o.Top, q)
	botQ := fx.FromFloat(o.Bottom, q)
	for row := 0; row < cov.H; row++ {
		var v uint8
		if o.Shading == "flat" {
			v = truncate(uint8(fx.MulShr(fx.T(255), topQ, q)), o.ColorBits)
		} else {
			// t = top + (bottom-top) * row/(H-1)，插值用 128 位乘法防溢出。
			if cov.H > 1 {
				hi, lo := mul128(int64(botQ-topQ), int64(row))
				step := fx.Div128(hi, lo, int64(cov.H-1))
				tQ := topQ + fx.T(step)
				v = truncate(uint8(fx.MulShr(fx.T(255), tQ, q)), o.ColorBits)
			} else {
				v = truncate(uint8(fx.MulShr(fx.T(255), topQ, q)), o.ColorBits)
			}
		}
		for col := 0; col < cov.W; col++ {
			idx := row*cov.W + col
			if cov.V[idx] != 0 {
				out.V[idx] = v
			}
		}
	}
	return out
}

// intBlur 是整数盒式模糊，均值向零截断，边界按有效邻域计数。
func intBlur(im Pix, radius int) Pix {
	if radius == 0 {
		cp := Pix{W: im.W, H: im.H, V: append([]uint8(nil), im.V...)}
		return cp
	}
	out := Pix{W: im.W, H: im.H, V: make([]uint8, len(im.V))}
	for y := 0; y < im.H; y++ {
		for x := 0; x < im.W; x++ {
			sum, count := 0, 0
			for dy := -radius; dy <= radius; dy++ {
				for dx := -radius; dx <= radius; dx++ {
					ny, nx := y+dy, x+dx
					if ny >= 0 && ny < im.H && nx >= 0 && nx < im.W {
						sum += int(im.V[ny*im.W+nx])
						count++
					}
				}
			}
			out.V[y*im.W+x] = uint8(sum / count)
		}
	}
	return out
}

// mapPix 对每个像素套用同一变换。
func mapPix(im Pix, f func(uint8) uint8) Pix {
	out := Pix{W: im.W, H: im.H, V: make([]uint8, len(im.V))}
	for i, v := range im.V {
		out.V[i] = f(v)
	}
	return out
}

// safeDiv128 在商超出 int64 时返回错误，而不是 bits.Div68 的 panic。
func safeDiv128(hi, lo, d int64) (int64, error) {
	if d == 0 {
		return 0, errors.New("除零")
	}
	neg := hi < 0
	if neg {
		hi, lo = neg128(hi, lo)
	}
	if uint64(hi) >= uint64(d) {
		return 0, errors.New("定点中间结果溢出 int64，请降低 coordBits 或 scale")
	}
	q64, _ := div64(uint64(hi), uint64(lo), uint64(d))
	// 与 fx.Div128 相同：绝对值商直接取负即向零截断。
	if neg {
		q64 = -q64
	}
	return int64(q64), nil
}
