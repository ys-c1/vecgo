package param

import (
	"errors"
	"math"
	"strings"
	"testing"
)

func TestErrorNamesField(t *testing.T) {
	err := IntIn("config.Width", 0, SideMin, SideMax)
	var pe *Error
	if !errors.As(err, &pe) {
		t.Fatalf("应为 *Error: %v", err)
	}
	if pe.Field != "config.Width" {
		t.Fatalf("字段错误: %s", pe.Field)
	}
	if !strings.Contains(err.Error(), "config.Width") {
		t.Fatalf("错误信息应含字段名: %v", err)
	}
}

func TestFloatRejectsNaNInf(t *testing.T) {
	for _, v := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if err := FloatIn("x", v, 0, 1); err == nil {
			t.Fatalf("%v 应被拒绝", v)
		}
		if err := FloatInOpenLo("x", v, 0, 1); err == nil {
			t.Fatalf("%v 应被拒绝", v)
		}
		if err := FloatPositive("x", v); err == nil {
			t.Fatalf("%v 应被拒绝", v)
		}
		if err := Finite("x", v); err == nil {
			t.Fatalf("%v 应被拒绝", v)
		}
	}
}

func TestOpenLowerBound(t *testing.T) {
	if err := FloatInOpenLo("x", 0, 0, 1); err == nil {
		t.Fatal("下界应为开放")
	}
	if err := FloatIn("x", 0, 0, 1); err != nil {
		t.Fatal("闭区间应接受下界")
	}
}

func TestAllAggregates(t *testing.T) {
	err := All(
		IntIn("a", 0, 1, 2),
		nil,
		IntIn("b", 9, 1, 2),
	)
	var list *ErrorList
	if !errors.As(err, &list) || len(list.Errors) != 2 {
		t.Fatalf("应汇总两个错误: %v", err)
	}
	if got := err.Error(); !strings.Contains(got, "a") || !strings.Contains(got, "b") {
		t.Fatalf("应包含两个字段: %s", got)
	}
}

func TestAllSingleReturnsPlainError(t *testing.T) {
	err := All(IntIn("a", 0, 1, 2))
	var pe *Error
	if !errors.As(err, &pe) || pe.Field != "a" {
		t.Fatalf("单个错误应原样返回: %v", err)
	}
	var list *ErrorList
	if errors.As(err, &list) {
		t.Fatal("单个错误不应包成 ErrorList")
	}
}

func TestPointsReportsIndex(t *testing.T) {
	pts := [][2]float64{{0, 0}, {math.NaN(), 1}, {2, 2}}
	err := Points("geom.Stroke.pts", len(pts), 2, func(i int) (float64, float64) {
		return pts[i][0], pts[i][1]
	})
	var pe *Error
	if !errors.As(err, &pe) || pe.Field != "geom.Stroke.pts[1]" {
		t.Fatalf("应指向第 2 个点: %v", err)
	}
}
