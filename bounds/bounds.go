// Package bounds 是 vecgo 坐标越界判定的唯一入口。
//
// geom、raster、post 三个包不再各自决定“越界了怎么办”（几何静默截断成空、
// 光栅把退化包围盒夹成 1、后处理静默跳过），而是统一调用本包判断坐标是否
// 落在调用方声明的域内。规则只有一条：
//
//   - 坐标落在域内（含四条边）→ 接受；
//   - 负坐标、超画幅、非有限数、区域倒置/退化 → 拒绝。
//
// 拒绝时返回 *Error，里面带包名、入口名、点下标和坐标，例如
//
//	raster.FillAt: polys[1][2] = (17, 8) 越界（超画幅：x=17 > 16）；允许范围 [0,16]×[0,16]
//
// 本包只做判断，不修改坐标，也不夹取、不截断、不跳过。
package bounds

import (
	"fmt"
	"math"
)

// Point 是一个二维坐标。geom、raster 各有自己的点类型，传入前转换为本类型，
// 避免包之间互相引用。
type Point struct{ X, Y float64 }

// Frame 是闭区间坐标域 [X0,X1]×[Y0,Y1]，四条边都算界内。
// “恰好等于边界”的坐标因此属于 OnEdge，接受。
type Frame struct {
	X0, Y0, X1, Y1 float64
}

// PixelFrame 返回 w×h 画幅在连续坐标下的 Frame：[0,w]×[0,h]。
func PixelFrame(w, h int) Frame {
	return Frame{X0: 0, Y0: 0, X1: float64(w), Y1: float64(h)}
}

// FrameOf 返回一组点的包围盒。空点集返回零值 Frame（Valid 为 false）。
// 坐标含 NaN 时包围盒无意义；调用方应先用 FinitePoints 校验。
func FrameOf(pts []Point) Frame {
	if len(pts) == 0 {
		return Frame{}
	}
	f := Frame{X0: pts[0].X, Y0: pts[0].Y, X1: pts[0].X, Y1: pts[0].Y}
	for _, p := range pts[1:] {
		f.X0 = math.Min(f.X0, p.X)
		f.Y0 = math.Min(f.Y0, p.Y)
		f.X1 = math.Max(f.X1, p.X)
		f.Y1 = math.Max(f.Y1, p.Y)
	}
	return f
}

// Valid 表示 Frame 本身形状合法：四边有限，且宽高都严格为正。
func (f Frame) Valid() bool {
	return finite(f.X0) && finite(f.Y0) && finite(f.X1) && finite(f.Y1) &&
		f.X1 > f.X0 && f.Y1 > f.Y0
}

// Size 返回宽、高。
func (f Frame) Size() (w, h float64) { return f.X1 - f.X0, f.Y1 - f.Y0 }

func (f Frame) String() string {
	return fmt.Sprintf("[%s,%s]×[%s,%s]", num(f.X0), num(f.X1), num(f.Y0), num(f.Y1))
}

// Contains 判断点是否在 Frame 内（含边界）。Frame 非法时一律返回 false。
func (f Frame) Contains(x, y float64) bool {
	return f.Valid() && Classify(x, y, f).OK()
}

// ContainsPixel 判断整数像素坐标是否落在 w×h 画幅内（半开区间）。
// 这是像素索引判定，不是连续坐标域判定：x 从 0 到 w-1 合法。
// 光栅采样与后处理卷积的邻域判定都走这一个函数。
func ContainsPixel(x, y, w, h int) bool {
	return x >= 0 && x < w && y >= 0 && y < h
}

// Verdict 是越界判定的结论。
type Verdict uint8

const (
	Inside     Verdict = iota // 严格在 Frame 内
	OnEdge                    // 恰好落在 Frame 的某条边上（接受）
	Negative                  // 在 Frame 的左侧或上方（负坐标方向）
	Beyond                    // 在 Frame 的右侧或下方（超画幅方向）
	NonFinite                 // NaN / ±Inf
	Inverted                  // 区域倒置或为空
	Degenerate                // 坐标范围退化为零宽/零高
)

// OK 表示该结论可以继续参与计算。贴边算界内。
func (v Verdict) OK() bool { return v == Inside || v == OnEdge }

func (v Verdict) String() string {
	switch v {
	case Inside:
		return "内部"
	case OnEdge:
		return "贴边"
	case Negative:
		return "负坐标"
	case Beyond:
		return "超画幅"
	case NonFinite:
		return "非有限数"
	case Inverted:
		return "区域倒置"
	case Degenerate:
		return "区域退化"
	default:
		return "未知"
	}
}

// Classify 判定点相对 Frame 的位置。非有限数优先；边界算贴边。
func Classify(x, y float64, f Frame) Verdict {
	if !finite(x) || !finite(y) {
		return NonFinite
	}
	if x < f.X0 || y < f.Y0 {
		return Negative
	}
	if x > f.X1 || y > f.Y1 {
		return Beyond
	}
	if x == f.X0 || x == f.X1 || y == f.Y0 || y == f.Y1 {
		return OnEdge
	}
	return Inside
}

// Error 是一条越界记录。Pkg 与 Fn 合起来就是“哪个包哪个入口”，
// Point 是出错点的名字（带下标），X/Y 是该点的坐标。
type Error struct {
	Pkg     string
	Fn      string
	Point   string
	X, Y    float64
	Frame   Frame
	Verdict Verdict
	Note    string // 无法用坐标差值描述时的人工说明
}

func (e *Error) Error() string {
	switch e.Verdict {
	case NonFinite:
		if e.Note != "" {
			return fmt.Sprintf("%s.%s: %s 非法（%s）", e.Pkg, e.Fn, e.Point, e.Note)
		}
		return fmt.Sprintf("%s.%s: %s 非法（非有限数，NaN/±Inf 不接受）", e.Pkg, e.Fn, e.Point)
	case Inverted, Degenerate:
		if e.Note != "" {
			return fmt.Sprintf("%s.%s: %s 非法（%s）", e.Pkg, e.Fn, e.Point, e.Note)
		}
	}
	msg := fmt.Sprintf("%s.%s: %s = (%s, %s) 越界（%s）",
		e.Pkg, e.Fn, e.Point, num(e.X), num(e.Y), e.detail())
	if e.Frame.Valid() {
		msg += "；允许范围 " + e.Frame.String()
	}
	return msg
}

func (e *Error) detail() string {
	if e.Note != "" {
		return e.Note
	}
	switch e.Verdict {
	case NonFinite:
		return "非有限数"
	case Negative:
		return "负坐标：" + e.edgeParts(false)
	case Beyond:
		return "超画幅：" + e.edgeParts(true)
	case Inverted:
		return "区域倒置或为空"
	case Degenerate:
		return "坐标范围退化"
	default:
		return e.Verdict.String()
	}
}

// edgeParts 列出具体越界的那一维，避免只说“越界”不说方向。
func (e *Error) edgeParts(high bool) string {
	var parts []string
	if high {
		if e.X > e.Frame.X1 {
			parts = append(parts, fmt.Sprintf("x=%s > %s", num(e.X), num(e.Frame.X1)))
		}
		if e.Y > e.Frame.Y1 {
			parts = append(parts, fmt.Sprintf("y=%s > %s", num(e.Y), num(e.Frame.Y1)))
		}
	} else {
		if e.X < e.Frame.X0 {
			parts = append(parts, fmt.Sprintf("x=%s < %s", num(e.X), num(e.Frame.X0)))
		}
		if e.Y < e.Frame.Y0 {
			parts = append(parts, fmt.Sprintf("y=%s < %s", num(e.Y), num(e.Frame.Y0)))
		}
	}
	return join(parts)
}

// Fail 生成一条带人工说明的错误，供需要自定义 Note 的调用方使用。
func Fail(pkg, fn, point string, x, y float64, f Frame, v Verdict, note string) error {
	return &Error{Pkg: pkg, Fn: fn, Point: point, X: x, Y: y, Frame: f, Verdict: v, Note: note}
}

func newError(pkg, fn, point string, x, y float64, f Frame, v Verdict) *Error {
	return &Error{Pkg: pkg, Fn: fn, Point: point, X: x, Y: y, Frame: f, Verdict: v}
}

// CheckFrame 判定判定域本身是否合法（宽高严格为正、四边有限）。
func CheckFrame(pkg, fn string, f Frame) error {
	if f.Valid() {
		return nil
	}
	return &Error{
		Pkg: pkg, Fn: fn, Point: "frame",
		X: f.X0, Y: f.Y0, Frame: f, Verdict: Inverted,
		Note: fmt.Sprintf("判定域本身非法（宽 %s、高 %s 必须为正）", num(f.X1-f.X0), num(f.Y1-f.Y0)),
	}
}

// Finite 判定一个标量参数（宽度、半径等）是否为有限数。
func Finite(pkg, fn, name string, v float64) error {
	if finite(v) {
		return nil
	}
	return newError(pkg, fn, name, v, 0, Frame{}, NonFinite)
}

// FinitePoints 只判定一组点是否都是有限数，不检查域。
func FinitePoints(pkg, fn, name string, pts []Point) error {
	for i, p := range pts {
		if finite(p.X) && finite(p.Y) {
			continue
		}
		return newError(pkg, fn, fmt.Sprintf("%s[%d]", name, i), p.X, p.Y, Frame{}, NonFinite)
	}
	return nil
}

// FiniteGroups 只判定多组点是否都是有限数，不检查域。
func FiniteGroups(pkg, fn, name string, groups [][]Point) error {
	for i, pts := range groups {
		for j, p := range pts {
			if finite(p.X) && finite(p.Y) {
				continue
			}
			return newError(pkg, fn, fmt.Sprintf("%s[%d][%d]", name, i, j), p.X, p.Y, Frame{}, NonFinite)
		}
	}
	return nil
}

// Points 判定一组点是否都是有限数并落在 Frame 内。name 是点列在调用方入口
// 里的名字（如 poly、window），错误里写成 name[i]。只报告第一处越界。
func Points(pkg, fn, name string, f Frame, pts []Point) error {
	if err := CheckFrame(pkg, fn, f); err != nil {
		return err
	}
	for i, p := range pts {
		v := Classify(p.X, p.Y, f)
		if v.OK() {
			continue
		}
		return newError(pkg, fn, fmt.Sprintf("%s[%d]", name, i), p.X, p.Y, f, v)
	}
	return nil
}

// Groups 判定多组点（例如 raster.FillAt 的多边形列表）是否都在 Frame 内，
// 错误里的点名为 name[i][j]。只报告第一处越界。
func Groups(pkg, fn, name string, f Frame, groups [][]Point) error {
	if err := CheckFrame(pkg, fn, f); err != nil {
		return err
	}
	for i, pts := range groups {
		for j, p := range pts {
			v := Classify(p.X, p.Y, f)
			if v.OK() {
				continue
			}
			return newError(pkg, fn, fmt.Sprintf("%s[%d][%d]", name, i, j), p.X, p.Y, f, v)
		}
	}
	return nil
}

// Extent 判定包围盒宽高是否都严格为正。宽或高为 0 时，旧实现会静默改成 1
// 把退化输入拉伸成整幅图；这里改为拒绝。
func Extent(pkg, fn, name string, sx, sy float64) error {
	if sx > 0 && sy > 0 {
		return nil
	}
	return &Error{
		Pkg: pkg, Fn: fn, Point: name + " 的包围盒",
		X: sx, Y: sy, Verdict: Degenerate,
		Note: fmt.Sprintf("宽=%s，高=%s，必须都为正（旧实现会把 0 夹成 1）", num(sx), num(sy)),
	}
}

// Rect 是整数像素区域 [X0,X1)×[Y0,Y1)。post 的区域入口用它。
type Rect struct {
	X0, Y0, X1, Y1 int
}

// Full 返回整幅 w×h 画幅的区域。
func Full(w, h int) Rect { return Rect{X0: 0, Y0: 0, X1: w, Y1: h} }

// W 返回宽度（像素数）。
func (r Rect) W() int { return r.X1 - r.X0 }

// H 返回高度（像素数）。
func (r Rect) H() int { return r.Y1 - r.Y0 }

// Empty 表示区域没有像素。
func (r Rect) Empty() bool { return r.X1 <= r.X0 || r.Y1 <= r.Y0 }

// InBounds 表示区域非空且完全落在 w×h 画幅内。
func (r Rect) InBounds(w, h int) bool {
	return !r.Empty() && r.X0 >= 0 && r.Y0 >= 0 && r.X1 <= w && r.Y1 <= h
}

func (r Rect) String() string {
	return fmt.Sprintf("[%d,%d,%d,%d)", r.X0, r.Y0, r.X1, r.Y1)
}

// CheckRect 判定调用方给出的像素区域是否完全落在 w×h 画幅内。
// 区域倒置/为空、任一角越界都拒绝；四条边恰好贴画幅算界内
// （例如 {0,0,w,h} 合法）。错误里报出越界那个角及其坐标。
func CheckRect(pkg, fn, name string, w, h int, r Rect) error {
	f := PixelFrame(w, h)
	label := fmt.Sprintf("%s=%s", name, r)
	if r.Empty() {
		return &Error{
			Pkg: pkg, Fn: fn, Point: label, Verdict: Inverted,
			Note: "区域为空（X1<=X0 或 Y1<=Y0）",
		}
	}
	type corner struct {
		label string
		x, y  float64
	}
	corners := []corner{
		{"左上角", float64(r.X0), float64(r.Y0)},
		{"右上角", float64(r.X1), float64(r.Y0)},
		{"左下角", float64(r.X0), float64(r.Y1)},
		{"右下角", float64(r.X1), float64(r.Y1)},
	}
	for _, c := range corners {
		v := Classify(c.x, c.y, f)
		if v.OK() {
			continue
		}
		return newError(pkg, fn, label+" 的"+c.label, c.x, c.y, f, v)
	}
	return nil
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

func num(v float64) string { return fmt.Sprintf("%g", v) }

func join(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += "，"
		}
		out += p
	}
	return out
}
