package dual

import (
	"math"

	"vecgo/fx"
)

// FloatEngine 是对齐后的浮点路径。
//
// 它与 IntEngine 保持逐操作同构：每个中间结果都通过 snapFloat 量化到
// CoordBits 精度网格（与定点取整方向、网格完全一致），每个颜色值都通过
// truncate 截断到 ColorBits。当参与运算的整数不超过 float64 的 53 位尾数时，
// 该路径与整型路径逐像素相同；一旦某步超出尾数精度，比对器会定位到具体阶段。
type FloatEngine struct{}

// Run 执行对齐浮点渲染管线。
func (FloatEngine) Run(scene [][][2]float64, o Options) (*Result, error) {
	if err := o.Validate(); err != nil {
		return nil, err
	}
	q := o.CoordBits

	// 阶段 0：变换系数与顶点先量化，再按与整型路径相同的顺序运算，
	// 每次乘法后立即 snap 到 Q 网格（对应 fx.MulShr 的向零截断）。
	c := snapFloat(math.Cos(o.Rotate), q)
	s := snapFloat(math.Sin(o.Rotate), q)
	scale := snapFloat(o.Scale, q)
	polys := make([][][2]float64, len(scene))
	vertQ := make([][2]int64, 0, len(scene)*4)
	for i, poly := range scene {
		pp := make([][2]float64, len(poly))
		for j, v := range poly {
			x := snapFloat(v[0], q)
			y := snapFloat(v[1], q)
			rx := snapFloat(scale*snapFloat(x*c-y*s, q), q)
			ry := snapFloat(scale*snapFloat(x*s+y*c, q), q)
			pp[j] = [2]float64{rx, ry}
			vertQ = append(vertQ, [2]int64{int64(fx.FromFloat(rx, q)), int64(fx.FromFloat(ry, q))})
		}
		polys[i] = pp
	}

	cov := floatRaster(polys, o.Width, o.Height, o.Supersample, q)
	stages := []Stage{{Name: "coverage", Pix: cov}}

	shade := floatShade(cov, o, q)
	stages = append(stages, Stage{Name: "shaded", Pix: shade})

	lut := gammaLUT(o.Gamma, o.ColorBits)
	gammed := mapPix(shade, func(v uint8) uint8 { return lut[v] })
	stages = append(stages, Stage{Name: "gamma", Pix: gammed})

	final := floatBlur(gammed, o.Blur)
	stages = append(stages, Stage{Name: "blur", Pix: final})
	return &Result{Engine: "float-aligned", Stages: stages, Final: final, VertQ: vertQ}, nil
}

// floatRaster 与 intRaster 逐操作对应。
//
// 关键对齐点：Q 网格上的每个量都以“整数 * 2^-Q”存在，因此这里直接在整数域
// 复现整型路径的两次向零除法（Go 整数除法即向零截断），点在多边形内判定也
// 直接调用整型 insideAny。绝不能对实数值用 math.Trunc——那会截断小数部分而非
// 网格整数，两条路径就此分叉。
func floatRaster(polys [][][2]float64, w, h, ss int, q uint) Pix {
	out := Pix{W: w, H: h, V: make([]uint8, w*h)}
	minX, minY, maxX, maxY := bboxF(polys)
	spanX := int64(math.Round((maxX - minX) * math.Ldexp(1, int(q))))
	spanY := int64(math.Round((maxY - minY) * math.Ldexp(1, int(q))))
	one := int64(1) << q
	if spanX == 0 {
		spanX = one
	}
	if spanY == 0 {
		spanY = one
	}
	iMinX := int64(math.Round(minX * math.Ldexp(1, int(q))))
	iMinY := int64(math.Round(minY * math.Ldexp(1, int(q))))
	den := ss * ss

	// 整型多边形的 Q 网格表示。点在多边形内判定直接复用整型 insideAny：
	// 网格值都是整数，128 位叉积精确；若改用浮点除法，采样点恰好落在边上时
	// 叉积的 0 会被舍入成 1e-14 量级的正数，tie-break 与整型路径分叉。
	iPolys := make([][][2]int64, len(polys))
	for i, poly := range polys {
		ip := make([][2]int64, len(poly))
		for j, v := range poly {
			ip[j] = [2]int64{
				int64(math.Round(v[0] * math.Ldexp(1, int(q)))),
				int64(math.Round(v[1] * math.Ldexp(1, int(q)))),
			}
		}
		iPolys[i] = ip
	}

	for row := 0; row < h; row++ {
		for col := 0; col < w; col++ {
			hit := 0
			for sj := 0; sj < ss; sj++ {
				for si := 0; si < ss; si++ {
					// 与 sampleCoord 完全相同的整数运算顺序：
					// mid=span*num/(2*ss)，off=mid/dim，均向零截断。
					oxNum := int64(col*ss+si)*2 + 1
					oyNum := int64(row*ss+sj)*2 + 1
					midX := spanX * oxNum / int64(2*ss)
					midY := spanY * oyNum / int64(2*ss)
					pxQ := iMinX + midX/int64(w)
					pyQ := iMinY + midY/int64(h)
					if insideAny(iPolys, pxQ, pyQ) {
						hit++
					}
				}
			}
			out.V[row*w+col] = uint8(255 * hit / den)
		}
	}
	return out
}

// insideAnyF 是射线法的浮点版本，判定式与整型叉积版完全等价。
func insideAnyF(polys [][][2]float64, px, py float64) bool {
	for _, poly := range polys {
		n := len(poly)
		inside := false
		for i := 0; i < n; i++ {
			ax, ay := poly[i][0], poly[i][1]
			bx, by := poly[(i+1)%n][0], poly[(i+1)%n][1]
			if (ay > py) == (by > py) {
				continue
			}
			t := (py - ay) / (by - ay)
			if ax+(bx-ax)*t > px {
				inside = !inside
			}
		}
		if inside {
			return true
		}
	}
	return false
}

// floatShade 与 intShade 同构：插值在 Q 网格上按“先乘后除、向零截断”
// 的同一运算顺序进行，保证两条路径逐值相同。
func floatShade(cov Pix, o Options, q uint) Pix {
	out := Pix{W: cov.W, H: cov.H, V: make([]uint8, len(cov.V))}
	topQ := fx.FromFloat(o.Top, q)
	botQ := fx.FromFloat(o.Bottom, q)
	for row := 0; row < cov.H; row++ {
		var v uint8
		if o.Shading == "flat" {
			v = truncate(uint8(math.Trunc(255*topQ.Float(q))), o.ColorBits)
		} else {
			if cov.H > 1 {
				// step = (botQ-topQ)*row/(H-1)，与 intShade 同一整数运算顺序。
				step := math.Trunc(float64(int64(botQ)-int64(topQ)) * float64(row) / float64(cov.H-1))
				tQ := float64(int64(topQ)) + step
				v = truncate(uint8(math.Trunc(255*tQ/math.Ldexp(1, int(q)))), o.ColorBits)
			} else {
				v = truncate(uint8(math.Trunc(255*topQ.Float(q))), o.ColorBits)
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

// floatBlur 与 intBlur 同构，均值用 Trunc 向零取整。
func floatBlur(im Pix, radius int) Pix {
	if radius == 0 {
		return Pix{W: im.W, H: im.H, V: append([]uint8(nil), im.V...)}
	}
	out := Pix{W: im.W, H: im.H, V: make([]uint8, len(im.V))}
	for y := 0; y < im.H; y++ {
		for x := 0; x < im.W; x++ {
			sum, count := 0.0, 0
			for dy := -radius; dy <= radius; dy++ {
				for dx := -radius; dx <= radius; dx++ {
					ny, nx := y+dy, x+dx
					if ny >= 0 && ny < im.H && nx >= 0 && nx < im.W {
						sum += float64(im.V[ny*im.W+nx])
						count++
					}
				}
			}
			out.V[y*im.W+x] = uint8(math.Trunc(sum / float64(count)))
		}
	}
	return out
}

// bboxF 求浮点多边形集合的包围盒。
func bboxF(polys [][][2]float64) (minX, minY, maxX, maxY float64) {
	minX, minY = polys[0][0][0], polys[0][0][1]
	maxX, maxY = minX, minY
	for _, p := range polys {
		for _, q := range p {
			if q[0] < minX {
				minX = q[0]
			}
			if q[0] > maxX {
				maxX = q[0]
			}
			if q[1] < minY {
				minY = q[1]
			}
			if q[1] > maxY {
				maxY = q[1]
			}
		}
	}
	return
}
