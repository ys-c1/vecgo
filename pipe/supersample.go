package pipe

import "math"

// EffectiveSupersample 按旋转后的采样间距反解所需的子采样数 N(θ)。
//
// 反解公式（一行）：
//
//	N(θ) = ceil(N0 · (|cosθ| + |sinθ|)²)
//
// 几何依据：旋转 θ 后，一个输出像素单元在多边形坐标系中是边长
// p = span/W 的轴对齐正方形；其子采样网格点投影到任意倾斜边的法向
// 方向上，相邻点的最大间距为
//
//	g(θ) = p/N · (|cosθ| + |sinθ|)
//
// （两个轴方向投影到法向的贡献之和，法向取 (sinθ,cosθ) 时取到等号）。
// 要求旋转后最不利方向上的采样间距不劣于零角度基线 g(0)=p/N0，即
//
//	p/N · (|cosθ|+|sinθ|) ≤ p/N0
//
// 两边消去 p 并整理，即得上面的反解公式。
//
// 可复算性：调用方只需代入同一组 N0 与 θ（弧度）即可得到同一整数。
// θ=0 时 (|cos0|+|sin0|)²=1，N(0)=N0，管线采样路径与基线完全一致。
func EffectiveSupersample(base int, theta float64) int {
	stretch := math.Abs(math.Cos(theta)) + math.Abs(math.Sin(theta))
	return int(math.Ceil(float64(base) * stretch * stretch))
}
