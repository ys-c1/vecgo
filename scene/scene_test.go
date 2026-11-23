package scene

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func read(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func codes(res *ParseResult) []string {
	out := make([]string, len(res.Diagnostics))
	for i, d := range res.Diagnostics {
		out[i] = string(d.Code)
	}
	return out
}

// 1. 单图形: 解析完整、config 正确、可渲染。
func TestSingleShape(t *testing.T) {
	res := ParseBytes("single.scene", read(t, "single.scene"))
	if res.Err != nil {
		t.Fatalf("不应报错: %v", res.Err)
	}
	if len(res.Scroll) != 1 || len(res.Scroll[0]) != 4 {
		t.Fatalf("图形数/顶点数异常: %d %v", len(res.Scroll), res.Scroll)
	}
	if res.Config.Width != 64 || res.Config.Shading != "flat" || res.Config.Output != "single.pgm" {
		t.Fatalf("config 异常: %+v", res.Config)
	}
	if err := res.Config.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(res.Groups) != 0 {
		t.Fatalf("单图形不应有分组: %d", len(res.Groups))
	}
}

// 2. 多图形: 顺序与数量保持。
func TestMultiShape(t *testing.T) {
	res := ParseBytes("multi.scene", read(t, "multi.scene"))
	if res.Err != nil {
		t.Fatalf("不应报错: %v", res.Err)
	}
	if len(res.Scroll) != 3 {
		t.Fatalf("应有 3 个图形, got %d", len(res.Scroll))
	}
	// 顺序: 第一条 x=0, 第二条 x=20, 第三条 x=40。
	if res.Scroll[0][0].X != 0 || res.Scroll[1][0].X != 20 || res.Scroll[2][0].X != 40 {
		t.Fatalf("图形顺序被打乱: %v", res.Scroll)
	}
}

// 3. 嵌套场景: group/end 结构与 Scroll 展开。
func TestNestedScene(t *testing.T) {
	res := ParseBytes("nested.scene", read(t, "nested.scene"))
	if res.Err != nil {
		t.Fatalf("不应报错: %v", res.Err)
	}
	if len(res.Scroll) != 4 {
		t.Fatalf("Scroll 应按文件顺序展开为 4 个图形, got %d", len(res.Scroll))
	}
	if len(res.Groups) != 1 {
		t.Fatalf("顶层分组应为 1, got %d", len(res.Groups))
	}
	outer := res.Groups[0]
	if outer.Name != "outer" || outer.Line != 9 || outer.EndLine != 15 {
		t.Fatalf("outer 分组位置异常: %+v", outer)
	}
	if len(outer.Children) != 1 || outer.Children[0].Name != "inner" {
		t.Fatalf("inner 子分组异常: %+v", outer.Children)
	}
	// outer 直接持有第 0、3 个 poly, inner 持有第 1 个; 第 4 个在分组外。
	if !reflect.DeepEqual(outer.PolyIndex, []int{0, 2}) {
		t.Fatalf("outer poly 下标异常: %v", outer.PolyIndex)
	}
	if !reflect.DeepEqual(outer.Children[0].PolyIndex, []int{1}) {
		t.Fatalf("inner poly 下标异常: %v", outer.Children[0].PolyIndex)
	}
}

// 3b. 未闭合分组: 报错但不丢已接受的图形, 分组按前缀补齐。
func TestUnclosedGroup(t *testing.T) {
	src := `vecgo-scene 1
canvas 8 8
transform 1 0
shading flat
gamma 1
blur 0
output x.pgm
group a
poly 0 0 1 0 1 1
`
	res := ParseBytes("unclosed.scene", []byte(src))
	if !hasCode(res, CodeSyntax) {
		t.Fatalf("未闭合分组应报 E_SYNTAX: %v", codes(res))
	}
	if len(res.Scroll) != 1 {
		t.Fatalf("已接受的图形不应丢失: %d", len(res.Scroll))
	}
	if len(res.Groups) != 1 || res.Groups[0].EndLine != 9 {
		t.Fatalf("未闭合分组应以末行补齐: %+v", res.Groups)
	}
}

// 4. 坏文件: 版本/未知字段/缺失/越界/坏 poly 一次性全部报告, 且互不中断。
func TestBadFileReportsAll(t *testing.T) {
	res := ParseBytes("bad.scene", read(t, "bad.scene"))
	if res.Err == nil {
		t.Fatal("坏文件应返回错误")
	}
	want := []string{"E_VERSION", "E_UNKNOWN_FIELD", "E_RANGE", "E_TOO_FEW_POINTS", "E_MISSING"}
	got := codes(res)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("错误码/顺序异常:\n got %v\nwant %v", got, want)
	}
	// 版本不符后 canvas/transform/shading/blur 仍被解析。
	if res.Config.Width != 64 || res.Config.Scale != 1 || res.Config.Shading != "flat" {
		t.Fatalf("版本错误后后续段未继续解析: %+v", res.Config)
	}
	// 越界字段回滚为默认值。
	if res.Config.Gamma != 1 {
		t.Fatalf("越界 gamma 应回滚为默认 1, got %v", res.Config.Gamma)
	}
	// 缺失字段回滚为默认值。
	if res.Config.Output != "out.pgm" {
		t.Fatalf("缺失 output 应回滚为默认, got %q", res.Config.Output)
	}
	// 坏 poly 整条丢弃; 前一条好 poly 保留。
	if len(res.Scroll) != 1 {
		t.Fatalf("应只回收 1 条好 poly, got %d", len(res.Scroll))
	}
	// 行号与字段名。
	byCode := map[Code]Diagnostic{}
	for _, d := range res.Diagnostics {
		byCode[d.Code] = d
	}
	if d := byCode[CodeVersion]; d.Line != 2 || d.Field != "vecgo-scene" {
		t.Fatalf("版本诊断定位异常: %+v", d)
	}
	if d := byCode[CodeRange]; d.Line != 7 || d.Field != "gamma" {
		t.Fatalf("越界诊断定位异常: %+v", d)
	}
	if d := byCode[CodeTooFewPoints]; d.Line != 9 || d.Field != "poly" {
		t.Fatalf("坏 poly 诊断定位异常: %+v", d)
	}
	// 原始文本与行号范围。
	if d := byCode[CodeVersion]; d.Snippet != "vecgo-scene 2" || d.EndLine != 2 {
		t.Fatalf("版本段原文异常: %q", d.Snippet)
	}
	if d := byCode[CodeUnknownField]; d.Snippet != "colour red" {
		t.Fatalf("未知字段段原文异常: %q", d.Snippet)
	}
	// 段落文本只覆盖有原文的诊断; 缺失字段没有可比对的行, 不得伪造。
	for _, s := range res.Segments {
		if s.Text == "" {
			t.Fatalf("SegmentText 原文不应为空: %+v", s)
		}
	}
	if len(res.Segments) != len(res.Diagnostics)-1 {
		t.Fatalf("应恰有一类诊断(E_MISSING)无原文: %d vs %d", len(res.Segments), len(res.Diagnostics))
	}
	// errors.Is 可按类别判断。
	if !errors.Is(res.Err, ErrRange) || !errors.Is(res.Err, ErrMissingField) {
		t.Fatalf("errors.Is 应可判定: %v", res.Err)
	}
	// 回收后的 config 仍可校验。
	if err := res.Config.Validate(); err != nil {
		t.Fatalf("回收 config 应可校验: %v", err)
	}
}

// 5. 超长字段: 报 E_FIELD_TOO_LONG 且不中断后续段。
func TestOversizeField(t *testing.T) {
	huge := "poly " + strings.Repeat("1 ", 3000) + "1"
	if len(huge) <= MaxFieldLen {
		t.Fatalf("构造的超长行不够长: %d", len(huge))
	}
	src := "vecgo-scene 1\ncanvas 8 8\ntransform 1 0\nshading flat\ngamma 1\nblur 0\noutput x.pgm\n" +
		huge + "\npoly 0 0 1 0 1 1\n"
	res := ParseBytes("long.scene", []byte(src))
	if !hasCode(res, CodeTooLong) {
		t.Fatalf("超长字段应报 E_FIELD_TOO_LONG: %v", codes(res))
	}
	if len(res.Scroll) != 1 {
		t.Fatalf("超长行之后的好 poly 应被解析: %d", len(res.Scroll))
	}
	for _, d := range res.Diagnostics {
		if d.Code == CodeTooLong && d.Line != 8 {
			t.Fatalf("超长诊断行号异常: %+v", d)
		}
	}
}

// 四类指定输入: 版本不符 / 字段缺失 / 数值越界 / 空文件。
func TestFourErrorClasses(t *testing.T) {
	base := func(mut func([]string) []string) string {
		lines := []string{
			"vecgo-scene 1", "canvas 8 8", "transform 1 0", "shading flat",
			"gamma 1", "blur 0", "output x.pgm", "poly 0 0 1 0 1 1",
		}
		return strings.Join(mut(lines), "\n") + "\n"
	}
	cases := []struct {
		name  string
		src   string
		code  Code
		line  int
		field string
	}{
		{
			name: "版本不符",
			src:  base(func(l []string) []string { l[0] = "vecgo-scene 7"; return l }),
			code: CodeVersion, line: 1, field: "vecgo-scene",
		},
		{
			name: "字段缺失",
			src:  base(func(l []string) []string { return append(l[:6:6], l[7]) }),
			code: CodeMissing, line: 7, field: "output",
		},
		{
			name: "数值越界",
			src:  base(func(l []string) []string { l[4] = "gamma 42"; return l }),
			code: CodeRange, line: 5, field: "gamma",
		},
		{
			name: "空文件",
			src:  "",
			code: CodeEmpty, line: 0, field: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := ParseBytes(tc.name, []byte(tc.src))
			if res.Err == nil {
				t.Fatal("应返回错误")
			}
			var hit *Diagnostic
			for i := range res.Diagnostics {
				if res.Diagnostics[i].Code == tc.code {
					hit = &res.Diagnostics[i]
					break
				}
			}
			if hit == nil {
				t.Fatalf("缺错误码 %s, got %v", tc.code, codes(res))
			}
			if hit.Line != tc.line || hit.Field != tc.field {
				t.Fatalf("定位异常: got line=%d field=%q, want line=%d field=%q",
					hit.Line, hit.Field, tc.line, tc.field)
			}
			// 错误不中断后续段: 空文件之外其余三类都应回收出 1 个图形。
			if tc.code != CodeEmpty && len(res.Scroll) != 1 {
				t.Fatalf("后续段应继续解析: %d", len(res.Scroll))
			}
			if err := res.Config.Validate(); err != nil {
				t.Fatalf("回收 config 应可校验: %v", err)
			}
		})
	}
}

// 同一文件解析两次, 结构与诊断必须完全一致。
func TestDeterministic(t *testing.T) {
	for _, name := range []string{"single.scene", "multi.scene", "nested.scene", "bad.scene", "empty.scene"} {
		data := read(t, name)
		a := ParseBytes(name, data)
		b := ParseBytes(name, data)
		if !reflect.DeepEqual(a.Scene, b.Scene) {
			t.Fatalf("%s: 两次解析结构不一致", name)
		}
		if !reflect.DeepEqual(a.Diagnostics, b.Diagnostics) {
			t.Fatalf("%s: 两次解析诊断不一致", name)
		}
		if (a.Err == nil) != (b.Err == nil) {
			t.Fatalf("%s: 两次解析错误状态不一致", name)
		}
	}
}

// 两档场景规模各报解析毫秒, 并记录到测试日志。
func TestScaleTimings(t *testing.T) {
	sizes := []struct {
		name string
		n    int
	}{
		{"small", 8},
		{"large", 2000},
	}
	for _, s := range sizes {
		var b strings.Builder
		b.WriteString("vecgo-scene 1\ncanvas 256 256\ntransform 1 0\nshading flat\ngamma 1\nblur 0\noutput x.pgm\n")
		for i := 0; i < s.n; i++ {
			fmt.Fprintf(&b, "poly %d 0 %d 10 %d 10\n", i%200, i%200, i%200)
		}
		start := time.Now()
		res := ParseBytes(s.name, []byte(b.String()))
		el := float64(time.Since(start).Microseconds()) / 1000
		if res.Err != nil {
			t.Fatalf("%s: %v", s.name, res.Err)
		}
		if len(res.Scroll) != s.n {
			t.Fatalf("%s: 图形数 %d, want %d", s.name, len(res.Scroll), s.n)
		}
		t.Logf("scale=%s polys=%d bytes=%d parse_ms=%.3f", s.name, s.n, b.Len(), el)
	}
}

// 解析失败时给出错段落的原始文本与行号范围。
func TestSegmentsOnFailure(t *testing.T) {
	res := ParseBytes("bad.scene", read(t, "bad.scene"))
	if len(res.Segments) == 0 {
		t.Fatal("失败时应保留 SegmentText")
	}
	for _, s := range res.Segments {
		if s.Line <= 0 {
			t.Fatalf("段应有起始行: %+v", s)
		}
		if s.EndLine < s.Line {
			t.Fatalf("行号范围非法: %+v", s)
		}
	}
	want := map[int]string{
		2: "vecgo-scene 2",
		4: "colour red",
		7: "gamma 99",
		9: "poly 0 0 10 0",
	}
	for line, text := range want {
		found := false
		for _, s := range res.Segments {
			if s.Line == line && s.Text == text {
				found = true
			}
		}
		if !found {
			t.Fatalf("缺段落 line=%d text=%q，实际 %+v", line, text, res.Segments)
		}
	}
}

// 缺版本行时仍继续解析其余字段。
func TestMissingVersionStillParses(t *testing.T) {
	src := "canvas 8 8\ntransform 1 0\nshading flat\ngamma 1\nblur 0\noutput x.pgm\npoly 0 0 1 0 1 1\n"
	res := ParseBytes("nover", []byte(src))
	if !hasCode(res, CodeVersion) {
		t.Fatalf("缺版本行应报 E_VERSION: %v", codes(res))
	}
	if res.Config.Width != 8 || len(res.Scroll) != 1 {
		t.Fatalf("其余字段应继续解析: %+v", res.Config)
	}
}

func hasCode(res *ParseResult, c Code) bool {
	for _, d := range res.Diagnostics {
		if d.Code == c {
			return true
		}
	}
	return false
}
