// Package param 是 vecgo 全部公开参数校验的唯一入口。
//
// config、geom、raster、shading、post 五个包不再各自决定“拒绝/静默/夹取/兜底/忽略”，
// 而是统一调用本包。所有错误都是 *param.Error，Field 固定采用
//
//	<包>.<入口>.<字段>
//
// 的形式（例如 config.Width、raster.Fill.supersample、geom.Stroke.width），
// 调用方可以直接据此定位；点列中的单个点带下标，如 geom.Stroke.pts[3]。
//
// 本包只做判断，从不修改传入的值：非法值一律拒绝，不做夹取，也不提供默认值兜底。
package param

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
)

// 共享取值范围。config 字段与各包 API 参数使用同一套边界，避免“两套口径”。
const (
	SideMin, SideMax               = 1, 4096 // 画幅宽/高
	SupersampleMin, SupersampleMax = 1, 8    // 超采样倍数
	ScaleMin, ScaleMax             = 0, 100  // 缩放，下界开放：(0, 100]
	GammaMin, GammaMax             = 0.1, 10 // 伽马
	BlurMin, BlurMax               = 0, 16   // 模糊半径
	IntensityMin, IntensityMax     = 0, 1    // 着色强度
	SegmentsMin                    = 1       // 贝塞尔细分段数，无业务上限
	StrokeMinPoints                = 2       // 描边折线最少点数
	PolygonMinVertices             = 3       // 多边形最少顶点数
	FillMinPolys                   = 1       // 一次填充最少多边形数
)

// ShadingModes 是合法的着色模式。
var ShadingModes = []string{"flat", "gradient"}

// Error 表示某包某入口的某个参数非法。
type Error struct {
	Field string // "<包>.<入口>.<字段>"，切片元素带下标
	Value any    // 实际取值
	Rule  string // 合法取值要求
}

func (e *Error) Error() string {
	var v string
	switch x := e.Value.(type) {
	case string:
		v = fmt.Sprintf("%q", x)
	case text:
		v = string(x)
	default:
		v = fmt.Sprint(x)
	}
	return fmt.Sprintf("%s: 非法值 %s（要求 %s）", e.Field, v, e.Rule)
}

// text 让 Error 原样输出值，不加引号。
type text string

// Invalid 生成一条带自定义规则的错误，供各包描述无法用通用规则表达的约束。
func Invalid(field string, value any, rule string) error {
	return &Error{Field: field, Value: value, Rule: rule}
}

// ErrorList 汇总同一次调用中发现的多个非法参数。
type ErrorList struct {
	Errors []*Error
}

func (l *ErrorList) Error() string {
	parts := make([]string, len(l.Errors))
	for i, e := range l.Errors {
		parts[i] = e.Error()
	}
	return strings.Join(parts, "；")
}

// Unwrap 让 errors.Is/As 能访问到每个具体错误。
func (l *ErrorList) Unwrap() []error {
	out := make([]error, len(l.Errors))
	for i, e := range l.Errors {
		out[i] = e
	}
	return out
}

// All 汇总校验结果：全为 nil 时返回 nil；只有一个错误时原样返回该 *Error，
// 多个时返回 *ErrorList。调用方用 errors.As 均可取到 *Error。
func All(errs ...error) error {
	var list []*Error
	for _, err := range errs {
		if err == nil {
			continue
		}
		var pe *Error
		if !errors.As(err, &pe) {
			panic(fmt.Sprintf("param.All: 非 param 错误 %v", err))
		}
		list = append(list, pe)
	}
	switch len(list) {
	case 0:
		return nil
	case 1:
		return list[0]
	default:
		return &ErrorList{Errors: list}
	}
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

// IntIn 要求 v 落在闭区间 [lo, hi]。
func IntIn(field string, v, lo, hi int) error {
	if v < lo || v > hi {
		return &Error{Field: field, Value: v, Rule: fmt.Sprintf("%d..%d 之间的整数", lo, hi)}
	}
	return nil
}

// AtLeast 要求计数 v 不小于 min。
func AtLeast(field string, v, min int) error {
	if v < min {
		return &Error{Field: field, Value: v, Rule: fmt.Sprintf("至少 %d", min)}
	}
	return nil
}

// FloatIn 要求 v 是有限数且落在闭区间 [lo, hi]。NaN、±Inf 一律拒绝。
func FloatIn(field string, v, lo, hi float64) error {
	rule := fmt.Sprintf("[%g, %g] 内的有限数（拒绝 NaN/±Inf）", lo, hi)
	if !finite(v) || v < lo || v > hi {
		return &Error{Field: field, Value: v, Rule: rule}
	}
	return nil
}

// FloatInOpenLo 要求 v 是有限数且落在 (lo, hi]。NaN、±Inf 一律拒绝。
func FloatInOpenLo(field string, v, lo, hi float64) error {
	rule := fmt.Sprintf("(%g, %g] 内的有限数（拒绝 NaN/±Inf）", lo, hi)
	if !finite(v) || v <= lo || v > hi {
		return &Error{Field: field, Value: v, Rule: rule}
	}
	return nil
}

// FloatPositive 要求 v 是有限正数，不设上界。NaN、±Inf 一律拒绝。
func FloatPositive(field string, v float64) error {
	if !finite(v) || v <= 0 {
		return &Error{Field: field, Value: v, Rule: "有限正数（拒绝 NaN/±Inf/非正数）"}
	}
	return nil
}

// Finite 要求 v 是有限数，不设区间。
func Finite(field string, v float64) error {
	if !finite(v) {
		return &Error{Field: field, Value: v, Rule: "有限数（拒绝 NaN/±Inf）"}
	}
	return nil
}

// Choice 要求 v 是 allowed 之一。
func Choice(field, v string, allowed []string) error {
	for _, a := range allowed {
		if v == a {
			return nil
		}
	}
	quoted := make([]string, len(allowed))
	for i, a := range allowed {
		quoted[i] = fmt.Sprintf("%q", a)
	}
	return &Error{Field: field, Value: v, Rule: "取值之一: " + strings.Join(quoted, ", ")}
}

// NonEmpty 要求字符串非空。
func NonEmpty(field, v string) error {
	if v == "" {
		return &Error{Field: field, Value: v, Rule: "非空字符串"}
	}
	return nil
}

// NotNil 要求 v 不是 nil 指针/切片/映射/接口。
func NotNil(field string, v any) error {
	if v == nil {
		return &Error{Field: field, Value: text("nil"), Rule: "不能为 nil"}
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Ptr, reflect.Slice, reflect.Map, reflect.Interface, reflect.Func, reflect.Chan:
		if rv.IsNil() {
			return &Error{Field: field, Value: text("nil"), Rule: "不能为 nil"}
		}
	}
	return nil
}

// Coord 要求一个具名点的坐标是有限数。field 形如 geom.Flatten.p0。
func Coord(field string, x, y float64) error {
	if finite(x) && finite(y) {
		return nil
	}
	return &Error{
		Field: field,
		Value: text(fmt.Sprintf("(X=%v, Y=%v)", x, y)),
		Rule:  "坐标必须是有限数（拒绝 NaN/±Inf）",
	}
}

// Points 校验点列：至少 min 个点，且每个点坐标有限。
// at 返回第 i 个点的坐标；这样 param 不必依赖 geom，避免循环引用。
// 点列元素在错误里记作 <field>[i]。
func Points(field string, n, min int, at func(i int) (x, y float64)) error {
	if n < min {
		return &Error{Field: field, Value: n, Rule: fmt.Sprintf("至少 %d 个点", min)}
	}
	for i := 0; i < n; i++ {
		x, y := at(i)
		if err := Coord(fmt.Sprintf("%s[%d]", field, i), x, y); err != nil {
			return err
		}
	}
	return nil
}

// Polygon 校验多边形：至少 3 个顶点，且每个顶点坐标有限。
func Polygon(field string, n int, at func(i int) (x, y float64)) error {
	return Points(field, n, PolygonMinVertices, at)
}

// DuplicatePoint 报告折线上第 i 点与前一点重合。
func DuplicatePoint(field string, i int) error {
	return &Error{
		Field: fmt.Sprintf("%s[%d]", field, i),
		Value: text("与前一点重合"),
		Rule:  "折线不允许重复点",
	}
}
