package pipe

import (
	"fmt"
	"io"
	"runtime"
	"strings"
	"time"
)

// Entry 记录一次失败：来源包、记录点行号（pipe/cmd 中捕获该错误的源码行）、
// 原因码、原始错误信息。
type Entry struct {
	Pkg  string
	Line int
	Code string
	Msg  string
}

// Stage 记录一个处理阶段的耗时，仅诊断开启时采集。
type Stage struct {
	Name string
	Dur  time.Duration
}

// Diag 是诊断记录器。nil 的 *Diag 上所有方法都是 no-op，
// 因此诊断关闭时代码路径零分配、零计时。
type Diag struct {
	Entries []Entry
	Stages  []Stage
}

// reason 是包内已知失败点的注册表：错误信息子串 -> 原因码。
// 只收录从 cmd 管线可达的失败点；未接入的见 artifacts/diag-report.md。
var reason = map[string][]struct {
	code  string
	match string
}{
	"config": {
		{"CFG_WIDTH_RANGE", "width 超出范围"},
		{"CFG_HEIGHT_RANGE", "height 超出范围"},
		{"CFG_SCALE_RANGE", "scale 超出范围"},
		{"CFG_SUPERSAMPLE_RANGE", "supersample 超出范围"},
		{"CFG_GAMMA_RANGE", "gamma 超出范围"},
		{"CFG_BLUR_RANGE", "blur 超出范围"},
		{"CFG_SHADING_INVALID", "shading 非法"},
		{"CFG_OUTPUT_EMPTY", "output 不能为空"},
	},
	"raster": {
		{"RAST_DIMS_NONPOS", "画幅必须为正"},
		{"RAST_SUPERSAMPLE_NONPOS", "supersample 必须为正"},
		{"RAST_POLY_VERTS", "多边形至少三个顶点"},
		{"RAST_EXPORT_EMPTY", "空图不能导出"},
	},
	"shading": {
		{"SHAD_MODE_UNKNOWN", "未知着色模式"},
		{"SHAD_INTENSITY_RANGE", "强度超出范围"},
	},
	"post": {
		{"POST_GAMMA_RANGE", "gamma 超出范围"},
		{"POST_BLUR_NEGATIVE", "blur 不能为负"},
	},
}

// codeFor 按包名和错误信息查原因码，查不到给兜底码 PKG_UNMAPPED。
func codeFor(pkg, msg string) string {
	for _, r := range reason[pkg] {
		if strings.Contains(msg, r.match) {
			return r.code
		}
	}
	return strings.ToUpper(pkg) + "_UNMAPPED"
}

// Fail 记录一条失败并原样返回 err，便于 `return nil, d.Fail(...)`。
// 行号是调用 Fail 的源码行（pipe.go 或 main.go 中的捕获点）。
func (d *Diag) Fail(pkg string, err error) error {
	if d == nil || err == nil {
		return err
	}
	_, _, line, _ := runtime.Caller(1)
	d.Entries = append(d.Entries, Entry{Pkg: pkg, Line: line, Code: codeFor(pkg, err.Error()), Msg: err.Error()})
	return err
}

// Time 执行 fn 并在诊断开启时记录阶段耗时。
func (d *Diag) Time(name string, fn func() error) error {
	if d == nil {
		return fn()
	}
	t0 := time.Now()
	err := fn()
	d.Stages = append(d.Stages, Stage{Name: name, Dur: time.Since(t0)})
	return err
}

// Report 把失败条目和阶段耗时写到 w（诊断开启时由 cmd 调到 stderr）。
func (d *Diag) Report(w io.Writer) {
	for _, e := range d.Entries {
		fmt.Fprintf(w, "[diag] fail pkg=%s line=%d code=%s msg=%q\n", e.Pkg, e.Line, e.Code, e.Msg)
	}
	for _, s := range d.Stages {
		fmt.Fprintf(w, "[diag] stage=%s dur=%s\n", s.Name, s.Dur)
	}
}
