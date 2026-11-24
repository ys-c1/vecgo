// Package sum 提供确定性浮点累加。
//
// 所有累加都严格按下标升序做左折叠，每一步加法都经过 //go:noinline
// 的 OrderedAdd：函数调用边界阻止编译器跨步骤做结合律重排、向量化
// 归约或融合，形成显式的“重排屏障”。配合 Go 规范规定的从左到右
// 求值顺序与 IEEE-754 默认舍入，同一输入在任何编译优化选项下都得到
// 逐位一致的结果。
package sum

import (
	"fmt"
	"math"
)

// OrderedAdd 是确定性累加的最小单元：一次严格的 IEEE-754 加法。
//
//go:noinline
func OrderedAdd(a, b float64) float64 {
	return a + b
}

// Float64 按固定下标顺序对 values 左折叠求和，不做任何重排。
// field 是调用方的字段名，会原样出现在错误信息中，便于定位输入。
// 空数组与 NaN 元素都会报错并带上字段名（NaN 另附下标）。
func Float64(field string, values []float64) (float64, error) {
	if len(values) == 0 {
		return 0, fmt.Errorf("字段 %q 为空数组", field)
	}
	acc := 0.0
	for i, v := range values {
		if math.IsNaN(v) {
			return 0, fmt.Errorf("字段 %q 下标 %d 为 NaN", field, i)
		}
		// acc 作为下一步的入参，构成串行依赖链，顺序无法被调换。
		acc = OrderedAdd(acc, v)
	}
	return acc, nil
}

// Dot 按固定下标顺序计算点积 Σ xs[i]*ys[i]，逐元素相乘后立即按
// 下标顺序左折叠，不做任何重排。
// 任一数组为空、两数组长度不匹配、含 NaN 均报错并带上字段名。
func Dot(xField, yField string, xs, ys []float64) (float64, error) {
	if len(xs) == 0 {
		return 0, fmt.Errorf("字段 %q 为空数组", xField)
	}
	if len(ys) == 0 {
		return 0, fmt.Errorf("字段 %q 为空数组", yField)
	}
	if len(xs) != len(ys) {
		return 0, fmt.Errorf("字段 %q 与 %q 长度不匹配: %d != %d",
			xField, yField, len(xs), len(ys))
	}
	acc := 0.0
	for i := range xs {
		if math.IsNaN(xs[i]) {
			return 0, fmt.Errorf("字段 %q 下标 %d 为 NaN", xField, i)
		}
		if math.IsNaN(ys[i]) {
			return 0, fmt.Errorf("字段 %q 下标 %d 为 NaN", yField, i)
		}
		acc = OrderedAdd(acc, xs[i]*ys[i])
	}
	return acc, nil
}
