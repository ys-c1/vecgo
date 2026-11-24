package sum

import (
	"math"
	"strings"
	"testing"
)

// 手工左折叠的参考实现，用于核对固定顺序。
func refLeftFold(values []float64) float64 {
	acc := 0.0
	for _, v := range values {
		acc = acc + v
	}
	return acc
}

func TestFloat64FixedOrder(t *testing.T) {
	// 1e16 处 ULP 为 2：单个 1 加进去被舍掉，左折叠结果仍是 1e16；
	// 若重排为先合并小项 (1+1)=2 再加大项，则得到 1e16+2。
	values := []float64{1e16, 1.0, 1.0}
	got, err := Float64("weights", values)
	if err != nil {
		t.Fatal(err)
	}
	if got != 1e16 {
		t.Fatalf("固定左折叠结果错误: got %v want 1e16", got)
	}
	if got != refLeftFold(values) {
		t.Fatalf("结果与参考左折叠不一致: got %v want %v", got, refLeftFold(values))
	}
	// 显式断言重排会产生不同结果，证明该用例确实能区分累加顺序。
	reordered := values[0] + (values[1] + values[2])
	if reordered == got {
		t.Fatal("测试数据无法区分累加顺序，用例失效")
	}
}

func TestFloat64Single(t *testing.T) {
	got, err := Float64("x", []float64{42.5})
	if err != nil || got != 42.5 {
		t.Fatalf("单元素求和错误: %v %v", got, err)
	}
}

func TestFloat64EmptyReportsField(t *testing.T) {
	_, err := Float64("samples", nil)
	if err == nil || !strings.Contains(err.Error(), "samples") ||
		!strings.Contains(err.Error(), "空数组") {
		t.Fatalf("空数组应报出字段名: %v", err)
	}
}

func TestFloat64NaNReportsFieldAndIndex(t *testing.T) {
	_, err := Float64("coverage", []float64{1, math.NaN(), 2})
	if err == nil || !strings.Contains(err.Error(), "coverage") ||
		!strings.Contains(err.Error(), "NaN") ||
		!strings.Contains(err.Error(), "1") {
		t.Fatalf("NaN 应报出字段名与下标: %v", err)
	}
}

func TestDotFixedOrder(t *testing.T) {
	// 乘积序列为 [1e16, 1, 1]：左折叠里两个 1 先后被 ULP=2 的舍入吞掉，
	// 重排为先合并小项则会得到 1e16+2。
	xs := []float64{1e8, 1.0, 1.0}
	ys := []float64{1e8, 1.0, 1.0}
	got, err := Dot("xs", "ys", xs, ys)
	if err != nil {
		t.Fatal(err)
	}
	if got != 1e16 {
		t.Fatalf("点积固定顺序错误: got %v want 1e16", got)
	}
}

func TestDotEmptyReportsFields(t *testing.T) {
	if _, err := Dot("a", "b", nil, []float64{1}); err == nil ||
		!strings.Contains(err.Error(), "a") {
		t.Fatalf("空数组应报出字段名: %v", err)
	}
	if _, err := Dot("a", "b", []float64{1}, nil); err == nil ||
		!strings.Contains(err.Error(), "b") {
		t.Fatalf("空数组应报出字段名: %v", err)
	}
}

func TestDotLengthMismatchReportsFields(t *testing.T) {
	_, err := Dot("lhs", "rhs", []float64{1, 2}, []float64{1})
	if err == nil || !strings.Contains(err.Error(), "lhs") ||
		!strings.Contains(err.Error(), "rhs") ||
		!strings.Contains(err.Error(), "长度不匹配") {
		t.Fatalf("长度不匹配应报出两个字段名: %v", err)
	}
}

func TestDotNaNReportsFieldAndIndex(t *testing.T) {
	if _, err := Dot("px", "py", []float64{math.NaN()}, []float64{1}); err == nil ||
		!strings.Contains(err.Error(), "px") || !strings.Contains(err.Error(), "NaN") {
		t.Fatalf("xs 含 NaN 应报出字段名: %v", err)
	}
	if _, err := Dot("px", "py", []float64{1, 2}, []float64{1, math.NaN()}); err == nil ||
		!strings.Contains(err.Error(), "py") {
		t.Fatalf("ys 含 NaN 应报出字段名: %v", err)
	}
}

func TestRepeatableAcrossRuns(t *testing.T) {
	values := []float64{0.1, 0.2, 0.3, 1e16, 1.0, -0.5, 7.25, 3.75}
	xs := []float64{1.5, 2.5, 3.5, 4.5}
	ys := []float64{5.5, 6.5, 7.5, 8.5}
	first, err := Float64("values", values)
	if err != nil {
		t.Fatal(err)
	}
	firstDot, err := Dot("xs", "ys", xs, ys)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 50; i++ {
		got, err := Float64("values", values)
		if err != nil {
			t.Fatal(err)
		}
		if math.Float64bits(got) != math.Float64bits(first) {
			t.Fatalf("第 %d 次求和位模式不一致", i)
		}
		gotDot, err := Dot("xs", "ys", xs, ys)
		if err != nil {
			t.Fatal(err)
		}
		if math.Float64bits(gotDot) != math.Float64bits(firstDot) {
			t.Fatalf("第 %d 次点积位模式不一致", i)
		}
	}
}
