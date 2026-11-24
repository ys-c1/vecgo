package junction

import "math"

// BlendClaims 是接点处的面积守恒合成算子。
//
// 输入 c[3] 是三个图元在同一个接点像素上各自的覆盖率“索赔”（0..1），
// w 是按扇区面积守恒反解出的权重（Σw=1）。输出
//
//	out = clamp(Σ_i w_i·c_i, 0, 1)
//
// 当三份索赔相同 c 时 out = c·Σw = c：总覆盖既不变成 3c（朴素相加 → 亮一圈），
// 也不变成 c/3（再除以 3 → 留下缺口）。Σw=1 必须由 CheckWeightSum 在该接点核对。
//
// 该算子只用于三个图元共同主张的接点帽区；远离接点、只有一个图元覆盖的像素
// 仍应走普通的命中/并集路径，不能全局乘 w_i。
func BlendClaims(c [3]float64, w Weights) (float64, error) {
	if _, err := CheckWeightSum(w); err != nil {
		return 0, err
	}
	v := 0.0
	for i := 0; i < 3; i++ {
		if math.IsNaN(c[i]) || c[i] < 0 || c[i] > 1 {
			return 0, &AngleError{Index: i, Value: c[i], Msg: "覆盖率索赔必须在 [0,1]"}
		}
		v += w[i] * c[i]
	}
	return clamp(v, 0, 1), nil
}

// NaiveAdd 是对照用的错误合成：三份索赔直接相加并截断到 1。
// 三份全量索赔时结果恒为 1（8-bit 下 255），相对周围 ink<255 的像素偏亮。
func NaiveAdd(c [3]float64) float64 {
	return clamp(c[0]+c[1]+c[2], 0, 1)
}
