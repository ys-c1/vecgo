// Package scene parses vecgo scene files.
//
// 格式版本 1 的文本文件, 以行为单位, "#" 起注释, 空行忽略:
//
//	vecgo-scene 1           必填, 必须是第一个有效行, 声明格式版本
//	canvas <w> <h>          必填, 画幅
//	transform <scale> <rot> 必填, 缩放与旋转
//	shading <mode>          必填, flat 或 gradient
//	gamma <v>               必填
//	blur <n>                必填
//	output <path>           必填
//	poly <x> <y> ...        图形段, 至少 3 个顶点, 可重复, 按出现顺序绘制
//	group <name>            分组开始, 可嵌套; 只影响诊断上下文, 不改变绘制顺序
//	end                     分组结束, 必须与最近的 group 配对
//
// 未知字段记一条诊断但不中断解析; 缺失字段在文件末尾统一补齐诊断;
// 任何诊断都不阻止后续段落继续解析。Scroll 永不为 nil, 便于坏文件
// 回收部分结果。
package scene

import (
	"errors"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"vecgo/config"
	"vecgo/geom"
)

// Version 是当前解析器支持的格式版本。
const Version = 1

// MaxFieldLen 是单行(单字段)允许的最大字节数, 超过记为 E_FIELD_TOO_LONG。
const MaxFieldLen = 4096

// Code 是稳定的机器可读错误码, 便于评审与脚本逐字比对。
type Code string

const (
	CodeEmpty        Code = "E_EMPTY"
	CodeTooLong      Code = "E_FIELD_TOO_LONG"
	CodeVersion      Code = "E_VERSION"
	CodeMissing      Code = "E_MISSING"
	CodeUnknownField Code = "E_UNKNOWN_FIELD"
	CodeSyntax       Code = "E_SYNTAX"
	CodeRange        Code = "E_RANGE"
	CodeBadValue     Code = "E_BAD_VALUE"
	CodeWrongArity   Code = "E_ARITY"
	CodeTooFewPoints Code = "E_TOO_FEW_POINTS"
)

// requiredFields 是必填字段, 按固定顺序报告。
var requiredFields = []string{
	"canvas", "transform", "shading", "gamma", "blur", "output",
}

// Diagnostic 是解析中发现的一个问题。
type Diagnostic struct {
	Code    Code
	Line    int    // 1-based 起始行; 0 表示无行号(空文件)
	EndLine int    // 1-based 结束行(含); 单行时等于 Line
	Field   string // 字段名, 非字段级问题为 ""
	Message string // 人类可读说明, 含违规值
	Snippet string // 出错段落原文, 逐字保留, 不含行尾换行
}

func (d Diagnostic) Error() string {
	loc := fmt.Sprintf("line %d", d.Line)
	if d.EndLine != 0 && d.EndLine != d.Line {
		loc = fmt.Sprintf("line %d-%d", d.Line, d.EndLine)
	}
	field := ""
	if d.Field != "" {
		field = " field=" + d.Field
	}
	return fmt.Sprintf("[%s] %s%s: %s", d.Code, loc, field, d.Message)
}

// Unwrap 暴露哨兵错误, 使 errors.Is 可按类别判断。
func (d Diagnostic) Unwrap() []error {
	if s := SentinelFor(d.Code); s != nil {
		return []error{s}
	}
	return nil
}

// Scene 是解析得到的场景。
//
// Scroll 始终是按文件顺序展开的图形列表, 也是交给渲染管线的输入;
// Groups 保留 group/end 的嵌套结构, 其中 PolyIndex 指向 Scroll 的下标。
// 分组只提供结构信息与诊断上下文, 不改变绘制顺序。
type Scene struct {
	Config config.Config
	Scroll [][]geom.Point
	Groups []*Group
}

// Group 是一个 group/end 块。
type Group struct {
	Name      string
	Line      int // group 行号
	EndLine   int // 配对 end 的行号; 未闭合时为文件末行
	PolyIndex []int
	Children  []*Group
}

// SegmentText 是出错段落的原始文本与行号范围, 供逐字比对。
type SegmentText struct {
	Line    int
	EndLine int
	Field   string
	Code    Code
	Text    string
}

// ParseResult 在 Err != nil 时仍携带部分结果。
//
// 回收规则:
//   - Config 始终是可校验的完整配置: 解析成功的字段保留解析值;
//     解析失败或被拒绝的字段回滚为 config.Default() 的同名字段。
//   - Scroll 只包含解析干净的 poly 段, 保持文件顺序; 坏 poly 整条丢弃,
//     不产生半个多边形, 也不影响之前已接受的 poly。
//   - ElapsedMS 与 Segments 不属于场景结构, 不参与一致性比较。
type ParseResult struct {
	Scene
	Path        string
	Diagnostics []Diagnostic
	Err         error
	ElapsedMS   float64
	Segments    []SegmentText
}

// Errors 返回逐条诊断, 便于 errors.Is/As 遍历。
func (r *ParseResult) Errors() []error {
	out := make([]error, len(r.Diagnostics))
	for i, d := range r.Diagnostics {
		out[i] = d
	}
	return out
}

// Map 把回收后的结果交给渲染管线; 若 Config 不合法则返回错误。
func (r *ParseResult) Map() ([][]geom.Point, config.Config, error) {
	if err := r.Config.Validate(); err != nil {
		return nil, r.Config, fmt.Errorf("recovered config invalid: %w", err)
	}
	return r.Scroll, r.Config, nil
}

var (
	ErrEmpty        = errors.New("scene file is empty")
	ErrVersion      = errors.New("unsupported scene version")
	ErrMissingField = errors.New("required field missing")
	ErrUnknownField = errors.New("unknown field")
	ErrRange        = errors.New("value out of range")
	ErrBadValue     = errors.New("malformed value")
	ErrFieldTooLong = errors.New("field exceeds maximum length")
)

// SentinelFor 把错误码映射到哨兵错误。
func SentinelFor(c Code) error {
	switch c {
	case CodeEmpty:
		return ErrEmpty
	case CodeVersion:
		return ErrVersion
	case CodeMissing:
		return ErrMissingField
	case CodeUnknownField:
		return ErrUnknownField
	case CodeRange:
		return ErrRange
	case CodeBadValue, CodeSyntax:
		return ErrBadValue
	case CodeTooLong:
		return ErrFieldTooLong
	}
	return nil
}

// Parse 读取并解析场景文件。结果永不为 nil; res.Err 非 nil 表示存在诊断。
func Parse(path string) *ParseResult {
	start := time.Now()
	data, err := os.ReadFile(path)
	if err != nil {
		return &ParseResult{
			Scene:     Scene{Config: config.Default()},
			Path:      path,
			Err:       fmt.Errorf("read scene %s: %w", path, err),
			ElapsedMS: msSince(start),
		}
	}
	res := ParseBytes(path, data)
	res.ElapsedMS = msSince(start)
	return res
}

// ParseBytes 解析 path 的内容。path 只用于诊断。
func ParseBytes(path string, data []byte) *ParseResult {
	start := time.Now()
	lines := splitLines(data)
	res := &ParseResult{Scene: Scene{Config: config.Default()}, Path: path}
	finish := func() *ParseResult {
		res.ElapsedMS = msSince(start)
		res.finish()
		return res
	}

	if len(lines) == 0 {
		res.report(Diagnostic{
			Code: CodeEmpty, Line: 0, EndLine: 0,
			Message: "empty file: no version line",
		})
		return finish()
	}

	c := res.Config // 工作副本; 仅在字段被接受时提交
	seen := map[string]bool{}
	first := true
	var stack []*Group
	// attach 把当前图形挂到最近的分组(若有)。
	attach := func(idx int) {
		if len(stack) > 0 {
			stack[len(stack)-1].PolyIndex = append(stack[len(stack)-1].PolyIndex, idx)
		}
	}
	closeGroup := func(endLine int) {
		if len(stack) == 0 {
			return
		}
		g := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		g.EndLine = endLine
		if len(stack) > 0 {
			stack[len(stack)-1].Children = append(stack[len(stack)-1].Children, g)
		} else {
			res.Groups = append(res.Groups, g)
		}
	}
	closeAll := func(endLine int) {
		for len(stack) > 0 {
			closeGroup(endLine)
		}
	}

	for _, ln := range lines {
		raw := ln.text
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		if len(raw) > MaxFieldLen {
			res.reportSegment(Diagnostic{
				Code: CodeTooLong, Line: ln.no, EndLine: ln.no,
				Message: fmt.Sprintf("line is %d bytes, limit is %d", len(raw), MaxFieldLen),
			}, raw)
			first = false
			continue
		}

		fields := strings.Fields(trimmed)
		key := strings.ToLower(fields[0])

		if first && key != "vecgo-scene" {
			// 版本段缺失, 但这一行仍是合法字段, 必须继续解析。
			res.reportSegment(Diagnostic{
				Code: CodeVersion, Line: ln.no, EndLine: ln.no, Field: "vecgo-scene",
				Message: "first non-blank line must be \"vecgo-scene <version>\"",
			}, raw)
		}
		first = false

		switch key {
		case "vecgo-scene":
			if seen["vecgo-scene"] {
				res.reportSegment(Diagnostic{
					Code: CodeSyntax, Line: ln.no, EndLine: ln.no, Field: "vecgo-scene",
					Message: "duplicate version line",
				}, raw)
				continue
			}
			seen["vecgo-scene"] = true
			if !expectArity(res, ln, raw, fields, 1, "version") {
				continue
			}
			v, ok := parseInt(res, ln, raw, fields[1], "version")
			if !ok {
				continue
			}
			if v != Version {
				res.reportSegment(Diagnostic{
					Code: CodeVersion, Line: ln.no, EndLine: ln.no, Field: "vecgo-scene",
					Message: fmt.Sprintf("unsupported version %d, expected %d", v, Version),
				}, raw)
				continue
			}

		case "canvas":
			seen["canvas"] = true
			if !expectArity(res, ln, raw, fields, 2, "width", "height") {
				continue
			}
			w, okw := parseInt(res, ln, raw, fields[1], "width")
			h, okh := parseInt(res, ln, raw, fields[2], "height")
			if !okw || !okh {
				continue
			}
			if w < 1 || w > 4096 {
				res.reportSegment(Diagnostic{
					Code: CodeRange, Line: ln.no, EndLine: ln.no, Field: "width",
					Message: fmt.Sprintf("width %d out of range [1,4096]", w),
				}, raw)
				continue
			}
			if h < 1 || h > 4096 {
				res.reportSegment(Diagnostic{
					Code: CodeRange, Line: ln.no, EndLine: ln.no, Field: "height",
					Message: fmt.Sprintf("height %d out of range [1,4096]", h),
				}, raw)
				continue
			}
			c.Width, c.Height = w, h

		case "transform":
			seen["transform"] = true
			if !expectArity(res, ln, raw, fields, 2, "scale", "rotate") {
				continue
			}
			scl, oks := parseFloat(res, ln, raw, fields[1], "scale")
			rot, okr := parseFloat(res, ln, raw, fields[2], "rotate")
			if !oks || !okr {
				continue
			}
			if scl <= 0 || scl > 100 {
				res.reportSegment(Diagnostic{
					Code: CodeRange, Line: ln.no, EndLine: ln.no, Field: "scale",
					Message: fmt.Sprintf("scale %v out of range (0,100]", scl),
				}, raw)
				continue
			}
			c.Scale, c.Rotate = scl, rot

		case "shading":
			seen["shading"] = true
			if !expectArity(res, ln, raw, fields, 1, "mode") {
				continue
			}
			if fields[1] != "flat" && fields[1] != "gradient" {
				res.reportSegment(Diagnostic{
					Code: CodeBadValue, Line: ln.no, EndLine: ln.no, Field: "shading",
					Message: fmt.Sprintf("unknown shading mode %q (want flat or gradient)", fields[1]),
				}, raw)
				continue
			}
			c.Shading = fields[1]

		case "gamma":
			seen["gamma"] = true
			if !expectArity(res, ln, raw, fields, 1, "v") {
				continue
			}
			g, ok := parseFloat(res, ln, raw, fields[1], "gamma")
			if !ok {
				continue
			}
			if g < 0.1 || g > 10 {
				res.reportSegment(Diagnostic{
					Code: CodeRange, Line: ln.no, EndLine: ln.no, Field: "gamma",
					Message: fmt.Sprintf("gamma %v out of range [0.1,10]", g),
				}, raw)
				continue
			}
			c.Gamma = g

		case "blur":
			seen["blur"] = true
			if !expectArity(res, ln, raw, fields, 1, "n") {
				continue
			}
			n, ok := parseInt(res, ln, raw, fields[1], "blur")
			if !ok {
				continue
			}
			if n < 0 || n > 16 {
				res.reportSegment(Diagnostic{
					Code: CodeRange, Line: ln.no, EndLine: ln.no, Field: "blur",
					Message: fmt.Sprintf("blur %d out of range [0,16]", n),
				}, raw)
				continue
			}
			c.Blur = n

		case "output":
			seen["output"] = true
			if !expectArity(res, ln, raw, fields, 1, "path") {
				continue
			}
			if fields[1] == "" {
				res.reportSegment(Diagnostic{
					Code: CodeBadValue, Line: ln.no, EndLine: ln.no, Field: "output",
					Message: "output path is empty",
				}, raw)
				continue
			}
			c.Output = fields[1]

		case "poly":
			seen["poly"] = true
			ncoord := len(fields) - 1
			if ncoord == 0 || ncoord%2 != 0 {
				res.reportSegment(Diagnostic{
					Code: CodeWrongArity, Line: ln.no, EndLine: ln.no, Field: "poly",
					Message: fmt.Sprintf("poly needs pairs of x y, got %d coordinates", ncoord),
				}, raw)
				continue
			}
			if ncoord/2 < 3 {
				res.reportSegment(Diagnostic{
					Code: CodeTooFewPoints, Line: ln.no, EndLine: ln.no, Field: "poly",
					Message: fmt.Sprintf("poly needs at least 3 vertices, got %d", ncoord/2),
				}, raw)
				continue
			}
			pts := make([]geom.Point, 0, ncoord/2)
			bad := false
			for k := 1; k < len(fields); k += 2 {
				x, okx := parseFloat(res, ln, raw, fields[k], "poly.x")
				y, oky := parseFloat(res, ln, raw, fields[k+1], "poly.y")
				if !okx || !oky {
					bad = true
					break
				}
				pts = append(pts, geom.Point{X: x, Y: y})
			}
			if bad {
				continue // 整条丢弃, 不产生半个多边形
			}
			attach(len(res.Scroll))
			res.Scroll = append(res.Scroll, pts)

		case "group":
			if !expectArity(res, ln, raw, fields, 1, "name") {
				continue
			}
			stack = append(stack, &Group{Name: fields[1], Line: ln.no})
			seen["group"] = true

		case "end":
			if !expectArity(res, ln, raw, fields, 0) {
				continue
			}
			if len(stack) == 0 {
				res.reportSegment(Diagnostic{
					Code: CodeSyntax, Line: ln.no, EndLine: ln.no, Field: "end",
					Message: "end without matching group",
				}, raw)
				continue
			}
			closeGroup(ln.no)

		default:
			res.reportSegment(Diagnostic{
				Code: CodeUnknownField, Line: ln.no, EndLine: ln.no, Field: fields[0],
				Message: fmt.Sprintf("unknown field %q", fields[0]),
			}, raw)
		}
	}

	eof := lastContentLine(lines)
	// 未闭合的 group 回滚: 以文件末行为 EndLine 收尾并报诊断,
	// Scroll 中已接受的图形保留, 分组结构按前缀补齐。
	unclosed := len(stack)
	closeAll(eof.no)
	if unclosed > 0 {
		res.reportSegment(Diagnostic{
			Code: CodeSyntax, Line: eof.no, EndLine: eof.no, Field: "group",
			Message: fmt.Sprintf("%d group(s) not closed by end", unclosed),
		}, eof.text)
	}
	for _, f := range requiredFields {
		if !seen[f] {
			// 缺失字段没有可逐字比对的原文; 行号锚定到最后一个有效行,
			// 段文本留空, 避免把末行误认成违规段。
			res.report(Diagnostic{
				Code: CodeMissing, Line: eof.no, EndLine: eof.no, Field: f,
				Message: fmt.Sprintf("required field %q missing", f),
			})
		}
	}
	if !seen["vecgo-scene"] {
		// 版本段完全缺席: 首行若不是版本行已在循环内报过 "must be first",
		// 这里只补一条缺失诊断, 不重复报同一码。
		dup := false
		for _, d := range res.Diagnostics {
			if d.Code == CodeVersion {
				dup = true
				break
			}
		}
		if !dup {
			res.reportSegment(Diagnostic{
				Code: CodeVersion, Line: lines[0].no, EndLine: lines[0].no, Field: "vecgo-scene",
				Message: "missing \"vecgo-scene\" version header",
			}, lines[0].text)
		}
	}

	res.Config = c
	return finish()
}

// ---- internals ----

type srcLine struct {
	no   int
	text string
}

func splitLines(data []byte) []srcLine {
	if len(data) == 0 {
		return nil
	}
	raw := strings.Split(string(data), "\n")
	if len(raw) > 0 && raw[len(raw)-1] == "" {
		raw = raw[:len(raw)-1]
	}
	out := make([]srcLine, len(raw))
	for i, s := range raw {
		out[i] = srcLine{no: i + 1, text: strings.TrimSuffix(s, "\r")}
	}
	return out
}

func (r *ParseResult) report(d Diagnostic) {
	if d.EndLine == 0 {
		d.EndLine = d.Line
	}
	r.Diagnostics = append(r.Diagnostics, d)
	// 只有真正对应某段原文的诊断才进 Segments; 缺失字段、空文件没有
	// 可逐字比对的段落, 不伪造原文。
	if d.Snippet != "" {
		r.Segments = append(r.Segments, SegmentText{
			Line: d.Line, EndLine: d.EndLine, Field: d.Field, Code: d.Code, Text: d.Snippet,
		})
	}
}

func (r *ParseResult) reportSegment(d Diagnostic, raw string) {
	d.Snippet = raw
	r.report(d)
}

func (r *ParseResult) finish() {
	if len(r.Diagnostics) == 0 {
		r.Err = nil
		r.Segments = nil
		return
	}
	sort.SliceStable(r.Diagnostics, func(i, j int) bool {
		if r.Diagnostics[i].Line != r.Diagnostics[j].Line {
			return r.Diagnostics[i].Line < r.Diagnostics[j].Line
		}
		return r.Diagnostics[i].Field < r.Diagnostics[j].Field
	})
	sort.SliceStable(r.Segments, func(i, j int) bool {
		return r.Segments[i].Line < r.Segments[j].Line
	})
	errs := make([]error, len(r.Diagnostics))
	for i, d := range r.Diagnostics {
		errs[i] = d
	}
	r.Err = errors.Join(errs...)
}

func expectArity(res *ParseResult, ln srcLine, raw string, fields []string, want int, names ...string) bool {
	if len(fields)-1 == want {
		return true
	}
	msg := fmt.Sprintf("want %d argument(s) (%s), got %d", want, strings.Join(names, " "), len(fields)-1)
	if want == 0 {
		msg = fmt.Sprintf("want no arguments, got %d", len(fields)-1)
	}
	res.reportSegment(Diagnostic{
		Code: CodeWrongArity, Line: ln.no, EndLine: ln.no, Field: strings.ToLower(fields[0]),
		Message: msg,
	}, raw)
	return false
}

func parseInt(res *ParseResult, ln srcLine, raw, s, field string) (int, bool) {
	v, err := strconv.Atoi(s)
	if err != nil {
		res.reportSegment(Diagnostic{
			Code: CodeBadValue, Line: ln.no, EndLine: ln.no, Field: field,
			Message: fmt.Sprintf("%s %q is not an integer", field, s),
		}, raw)
		return 0, false
	}
	return v, true
}

func parseFloat(res *ParseResult, ln srcLine, raw, s, field string) (float64, bool) {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		res.reportSegment(Diagnostic{
			Code: CodeBadValue, Line: ln.no, EndLine: ln.no, Field: field,
			Message: fmt.Sprintf("%s %q is not a finite number", field, s),
		}, raw)
		return 0, false
	}
	return v, true
}

func msSince(t time.Time) float64 {
	return float64(time.Since(t).Nanoseconds()) / 1e6
}

// lastContentLine 返回最后一条非空非注释行, 用于锚定 EOF 诊断;
// 全为空/注释时退回文件末行。
func lastContentLine(lines []srcLine) srcLine {
	for i := len(lines) - 1; i >= 0; i-- {
		s := strings.TrimSpace(lines[i].text)
		if s != "" && !strings.HasPrefix(s, "#") {
			return lines[i]
		}
	}
	return lines[len(lines)-1]
}
