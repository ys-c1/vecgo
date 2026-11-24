package dual

import "math"

// NativeFloatEngine 是未对齐基线：全程 float64，中间结果不量化、不截断，
// 复刻本仓库原始管线（pipe/raster/shading/post）的数值行为。
// 它与整型路径的差异即“边缘像素差一档”的来源；分阶段快照可定位首个分歧步骤。
type NativeFloatEngine struct{}

// Run 执行原生浮点渲染管线。
func (NativeFloatEngine) Run(scene [][][2]float64, o Options) (*Result, error) {
	if err := o.Validate(); err != nil {
		return nil, err
	}

	// 阶段 0：变换，不做任何量化。
	polys := make([][][2]float64, len(scene))
	vertQ := make([][2]int64, 0, len(scene)*4)
	c, s := math.Cos(o.Rotate), math.Sin(o.Rotate)
	for i, poly := range scene {
		pp := make([][2]float64, len(poly))
		for j, v := range poly {
			pp[j] = [2]float64{
				o.Scale * (v[0]*c - v[1]*s),
				o.Scale * (v[0]*s + v[1]*c),
			}
			// 仅用于报告展示：把原生顶点投影到 Q 网格，看它与对齐路径差了几档。
			vertQ = append(vertQ, [2]int64{
				int64(math.Round(pp[j][0] * math.Ldexp(1, int(o.CoordBits)))),
				int64(math.Round(pp[j][1] * math.Ldexp(1, int(o.CoordBits)))),
			})
		}
		polys[i] = pp
	}

	cov := nativeRaster(polys, o.Width, o.Height, o.Supersample)
	stages := []Stage{{Name: "coverage", Pix: cov}}

	shade := nativeShade(cov, o)
	stages = append(stages, Stage{Name: "shaded", Pix: shade})

	// gamma 表与对齐路径共用参考实现，表项同样截断，
	// 使本引擎与对齐路径的差异只来自前序坐标/颜色精度，不混入 LUT 分歧。
	lut := gammaLUT(o.Gamma, o.ColorBits)
	gammed := mapPix(shade, func(v uint8) uint8 { return lut[v] })
	stages = append(stages, Stage{Name: "gamma", Pix: gammed})

	final := floatBlur(gammed, o.Blur)
	stages = append(stages, Stage{Name: "blur", Pix: final})
	return &Result{Engine: "float-native", Stages: stages, Final: final, VertQ: vertQ}, nil
}

// nativeRaster 复刻 raster.Image.Fill 的原始浮点采样与覆盖度计算。
func nativeRaster(polys [][][2]float64, w, h, ss int) Pix {
	out := Pix{W: w, H: h, V: make([]uint8, w*h)}
	minX, minY, maxX, maxY := bboxF(polys)
	spanX, spanY := maxX-minX, maxY-minY
	if spanX == 0 {
		spanX = 1
	}
	if spanY == 0 {
		spanY = 1
	}
	den := ss * ss
	for row := 0; row < h; row++ {
		for col := 0; col < w; col++ {
			hit := 0
			for sj := 0; sj < ss; sj++ {
				for si := 0; si < ss; si++ {
					px := minX + (float64(col)+(float64(si)+0.5)/float64(ss))*spanX/float64(w)
					py := minY + (float64(row)+(float64(sj)+0.5)/float64(ss))*spanY/float64(h)
					if insideAnyF(polys, px, py) {
						hit++
					}
				}
			}
			out.V[row*w+col] = uint8(255 * hit / den)
		}
	}
	return out
}

// nativeShade 复刻 shading.Apply：flat 用 255*top 直接转 uint8（向零截断），
// gradient 按行插值。本引擎不在中间步骤做位深截断。
func nativeShade(cov Pix, o Options) Pix {
	out := Pix{W: cov.W, H: cov.H, V: make([]uint8, len(cov.V))}
	for row := 0; row < cov.H; row++ {
		var v uint8
		if o.Shading == "flat" {
			v = uint8(255 * o.Top)
		} else {
			t := o.Top
			if cov.H > 1 {
				t = o.Top + (o.Bottom-o.Top)*float64(row)/float64(cov.H-1)
			}
			v = uint8(255 * t)
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
