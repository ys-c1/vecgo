// Package pipe 串起 config → geom → raster → shading → post 五段。
//
// 五段的失败已经归一到 verr 编码（按包分段，见 vecgo/verr）。RunEx 不再在
// 第一处失败就整体放弃：某一小段失败只摘掉那一段的输出，其余段照常出图。
// “段”的划分与 verr.Stage 一一对应，所以错误编码和隔离粒度不会走样。
package pipe

import (
	"math"

	"vecgo/config"
	"vecgo/diag"
	"vecgo/geom"
	"vecgo/post"
	"vecgo/raster"
	"vecgo/shading"
	"vecgo/verr"
)

func Transform(pts []geom.Point, scale, rotate float64) []geom.Point {
	c, s := math.Cos(rotate), math.Sin(rotate)
	out := make([]geom.Point, len(pts))
	for i, p := range pts {
		out[i] = geom.Point{}
		out[i].X = scale * (p.X*c - p.Y*s)
		out[i].Y = scale * (p.X*s + p.Y*c)
	}
	return out
}

// Failure 是一次已归一的段失败。
type Failure struct {
	Kind  verr.Kind
	Stage verr.Stage
	Err   error
}

// Result 是一次渲染的结果。
//
// Im 永远非 nil：即使五段全倒，也会给一张同尺寸的全黑图顶上，
// 上层不必到处判空。Failures 按发生顺序记录每段失败（同一段可以多条，
// 例如 post 的 gamma 与 blur 各自失败）。Dropped 报告哪些段被摘掉。
type Result struct {
	Im       *raster.Image
	Failures []Failure
}

// Failed 报告是否有段失败。
func (r *Result) Failed() bool { return len(r.Failures) > 0 }

// Dropped 报告某段是否被摘掉。
func (r *Result) Dropped(s verr.Stage) bool {
	for _, f := range r.Failures {
		if f.Stage == s {
			return true
		}
	}
	return false
}

// Kinds 按发生顺序返回全部失败编码。
func (r *Result) Kinds() []verr.Kind {
	out := make([]verr.Kind, len(r.Failures))
	for i, f := range r.Failures {
		out[i] = f.Kind
	}
	return out
}

// Error 把全部段失败汇总成一条错误；没有失败时返回 nil。
func (r *Result) Error() error {
	if len(r.Failures) == 0 {
		return nil
	}
	return &StageErrors{Failures: r.Failures}
}

// StageErrors 汇总多段失败，每个底层错误都保留各自的 verr 编码。
type StageErrors struct {
	Failures []Failure
}

func (e *StageErrors) Error() string {
	b := make([]byte, 0, 24*len(e.Failures))
	b = append(b, "多段失败:"...)
	for _, f := range e.Failures {
		b = append(b, ' ')
		b = append(b, f.Stage.String()...)
		b = append(b, '[')
		b = append(b, f.Kind.Hex()...)
		b = append(b, ']')
	}
	return string(b)
}

// Unwrap 返回全部底层错误，让 errors.Is/As 能找到任意一段的编码。
func (e *StageErrors) Unwrap() []error {
	out := make([]error, 0, len(e.Failures))
	for _, f := range e.Failures {
		out = append(out, f.Err)
	}
	return out
}

// RunOpts 是可选的运行参数。
type RunOpts struct {
	// Diag 为 nil 时按关闭处理；传 diag.New(true, ...) 即常开诊断。
	// 常开时渲染路径不会因此多出任何堆分配（见 pipe_test.go 的差值测试）。
	Diag *diag.Recorder
}

// Run 是兼容旧签名的便捷版本：成功返回 (图, nil)；有段失败时仍返回
// 已经部分渲染出来的图，同时返回非 nil 错误（图与错误同时有效）。
func Run(scene [][]geom.Point, cfg config.Config) (*raster.Image, error) {
	res := RunEx(scene, cfg, RunOpts{})
	if err := res.Error(); err != nil {
		return res.Im, err
	}
	return res.Im, nil
}

// inject 是测试用的故障注入缝：给定编码就返回一条该编码的失败，
// 生产路径传 nil。它只决定“这一段的检查点报不报错”，恢复动作仍走
// 与真实失败完全相同的路径，所以隔离行为能被真实测到。
type inject func(verr.Kind) error

func (inj inject) get(k verr.Kind) error {
	if inj == nil {
		return nil
	}
	return inj(k)
}

// RunEx 按段执行，单段失败只摘单段，其余照常出图。
//
// 各段的失败语义：
//
//	config   配置非法 → 丢弃这一份配置，改用 config.Default() 继续渲染。
//	         这是唯一会改变整幅图参数的段（其余各段都从有效配置里取参数）。
//	geom     单个多边形不合法（顶点不足或坐标非有限）→ 只剔掉这个多边形，
//	         其余多边形照常光栅。几何构建函数（Flatten/Clip/Stroke）的编码
//	         同样落在 geom 段，供直接调用者使用。
//	raster   建图或光栅自身失败 → 用同尺寸全黑图顶上，着色/后处理照常跑。
//	shading  模式或强度非法 → 跳过着色，保留未着色的覆盖率图。
//	post     gamma 与 blur 各自独立：谁失败只跳谁，另一个照常执行。
func RunEx(scene [][]geom.Point, cfg config.Config, opts RunOpts) *Result {
	return runEx(scene, cfg, opts, nil)
}

func runEx(scene [][]geom.Point, cfg config.Config, opts RunOpts, inj inject) *Result {
	d := opts.Diag
	res := &Result{}

	record := func(err error) {
		k, _ := verr.Of(err)
		f := Failure{Kind: k, Stage: k.Stage(), Err: err}
		res.Failures = append(res.Failures, f)
		if d.On() {
			d.ObserveFail(k)
			d.ObserveDrop(f.Stage)
			d.AddStage(diag.EvStageDropped, f.Stage, k, int64(f.Stage), int64(len(res.Failures)))
		}
	}

	if d.On() {
		d.AddStage(diag.EvStart, verr.StageNone, 0, int64(len(scene)), int64(cfg.Width)*int64(cfg.Height))
	}

	// --- config ---
	// 有效配置：校验通过就用原配置，否则退回 Default()，渲染继续。
	effective := cfg
	if err := inj.get(verr.KindConfigWidth); err != nil {
		record(err)
		effective = config.Default()
	} else if err := cfg.Validate(); err != nil {
		record(err)
		effective = config.Default()
	} else if d.On() {
		d.AddStage(diag.EvStageOK, verr.StageConfig, 0, int64(effective.Width), int64(effective.Height))
	}

	// --- raster: 建图 ---
	var im *raster.Image
	if err := inj.get(verr.KindRasterSize); err != nil {
		record(err)
	} else {
		var nerr error
		im, nerr = raster.New(effective.Width, effective.Height)
		if nerr != nil {
			record(nerr)
			im = nil
		}
	}
	if im == nil {
		// 建图失败时仍给同尺寸的零值图：后续段有图可跑，上层不用判空。
		im = &raster.Image{W: effective.Width, H: effective.Height,
			Pix: make([]uint8, effective.Width*effective.Height)}
		if d.On() {
			d.AddStage(diag.EvStageDropped, verr.StageRaster, verr.KindRasterSize, int64(verr.StageRaster), 0)
		}
	} else if d.On() {
		d.AddStage(diag.EvStageOK, verr.StageRaster, verr.KindRasterSize, int64(im.W), int64(im.H))
	}

	// --- geom: 逐多边形校验，坏的只剔自己 ---
	valid := make([][]geom.Point, 0, len(scene))
	var geomGate error
	if err := inj.get(verr.KindGeomFewVerts); err != nil {
		geomGate = err
	} else {
		// 不做额外分配的分类扫描，只按顺序校验。
		for _, poly := range scene {
			if err := geom.ValidPolygon(poly); err != nil {
				record(err)
				continue
			}
			moved := Transform(poly, effective.Scale, effective.Rotate)
			if err := geom.ValidPolygon(moved); err != nil {
				record(err)
				continue
			}
			valid = append(valid, moved)
		}
	}
	if geomGate != nil {
		record(geomGate)
		valid = nil
	}
	if d.On() {
		d.AddStage(diag.EvStageOK, verr.StageGeom, 0, int64(len(valid)), int64(len(scene)))
	}

	// --- raster: 光栅 ---
	// 建图已失败时不再重复跑 Fill，避免把同一段报两次。
	if err := inj.get(verr.KindRasterSuper); err != nil {
		record(err)
		clearImage(im)
	} else if im.W > 0 && im.H > 0 && len(im.Pix) == im.W*im.H {
		if err := im.Fill(valid, effective.Supersample); err != nil {
			record(err)
			clearImage(im)
			if d.On() {
				d.AddStage(diag.EvStageDropped, verr.StageRaster, verr.KindRasterSize, int64(verr.StageRaster), 0)
			}
		} else if d.On() {
			// 覆盖率指纹：失败隔离后的图会和这条记录对不上，便于还原现场。
			d.AddStage(diag.EvStageOK, verr.StageRaster, 0, int64(im.CountOn(1)), int64(len(valid)))
		}
	}

	// --- shading ---
	if err := inj.get(verr.KindShadingMode); err != nil {
		record(err)
	} else if err := shading.Apply(im, effective.Shading, 1.0, 0.0); err != nil {
		record(err)
		if d.On() {
			d.AddStage(diag.EvStageDropped, verr.StageShading, verr.KindShadingMode, int64(verr.StageShading), 0)
		}
	} else if d.On() {
		d.AddStage(diag.EvStageOK, verr.StageShading, 0, 0, 0)
	}

	// --- post: gamma ---
	if err := inj.get(verr.KindPostGamma); err != nil {
		record(err)
	} else if err := post.Gamma(im, effective.Gamma); err != nil {
		record(err)
		if d.On() {
			d.AddStage(diag.EvStageDropped, verr.StagePost, verr.KindPostGamma, int64(verr.StagePost), 0)
		}
	} else if d.On() {
		d.AddStage(diag.EvStageOK, verr.StagePost, 0, 0, 1)
	}

	// --- post: blur（独立于 gamma，gamma 失败也照常尝试）---
	if err := inj.get(verr.KindPostBlurRadius); err != nil {
		record(err)
	} else if out, err := post.Blur(im, effective.Blur); err != nil {
		record(err)
		if d.On() {
			d.AddStage(diag.EvStageDropped, verr.StagePost, verr.KindPostBlurRadius, int64(verr.StagePost), 0)
		}
	} else {
		im = out
		if d.On() {
			d.AddStage(diag.EvStageOK, verr.StagePost, 0, 0, 2)
		}
	}
	res.Im = im

	if d.On() {
		d.AddStage(diag.EvFinish, verr.StageNone, 0, int64(len(res.Failures)), int64(im.CountOn(1)))
	}
	return res
}

// clearImage 把图清成全黑，用于光栅段失败后的占位。
func clearImage(im *raster.Image) {
	for i := range im.Pix {
		im.Pix[i] = 0
	}
}
