package raster

import "math"

// 本文件实现抗锯齿子采样点的低差异序列构造。
//
// 分布规则（推导两行）：
//  1. 二维 Hammersley 点集取 p_i = (i/N, φ_2(i))，其中二进制逆根
//     φ_2(i)=Σ b_k 2^{-k-1}（i 的第 k 个二进制位为 b_k）；N=2^m 时
//     它是 (0,m,2)-网，星差异 D_N=O(log N/N)，优于 m×m 均匀网格的 O(N^{-1/2})。
//  2. 代入两档样本数：N=16=2^4 -> p_i=(i/16, φ_2(i))，i=0..15；
//     N=64=2^6 -> p_i=(i/64, φ_2(i))，i=0..63；重复出图时再叠加同一个
//     种子确定的 Cranley–Patterson 环面平移 (ux,uy)，即 p'_i=frac(p_i+(ux,uy))。
//
// 随机化与可复现：每次“重复出图”的平移量由 SplitMix64 生成，主种子固定为
// VarianceSeed（20261125），逐角度、逐档位再派生独立子流；整数运算与 Go
// 工具链无关，因此任何人执行 `go run ./cmd/variance` 都得到同一张方差表。

// VarianceSeed 是方差对照实验的固定主随机种子（写成十进制以便复现）。
const VarianceSeed int64 = 20261125

// VarianceTrials 是每个“角度×样本数”格子重复估计覆盖率的次数。
const VarianceTrials = 4000

// VarianceCounts 是对照用的两档样本总数：16=4×4 网格 / Hammersley 2^4，
// 64=8×8 网格 / Hammersley 2^6；也正好对应 config.Supersample 的 4 与 8 档。
var VarianceCounts = []int{16, 64}

// VarianceAngles 是对照用的三种边缘倾角（相对 x 轴）：
// 0°（水平/轴对齐）、arctan(1/2)≈26.565°（2:1 有理斜率）、45°（对角）。
var VarianceAngles = []float64{0, 26.56505117707799, 45}

// RadicalInverse2 返回 i 的二进制逆根 φ_2(i)：
// 把 i 的有效二进制位按反序写到小数点之后。
// 例：i=1 (…0001) -> 0.1_2 = 0.5；i=2 (…0010) -> 0.01_2 = 0.25；
// i=3 (…0011) -> 0.11_2 = 0.75。
func RadicalInverse2(i uint64) float64 {
	x := 0.0
	f := 0.5
	for i > 0 {
		if i&1 != 0 {
			x += f
		}
		i >>= 1
		f *= 0.5
	}
	return x
}

// HammersleySample 返回 N 点 Hammersley(2) 点集的第 i 个点
// (i/N, φ_2(i))，坐标落在 [0,1)²。N 取 2 的幂时分层/网性质最好。
func HammersleySample(n, i int) (sx, sy float64) {
	return float64(i) / float64(n), RadicalInverse2(uint64(i))
}

// GridSample 返回 m×m 居中均匀网格中按 si、sj 索引的样本
// ((si+0.5)/m, (sj+0.5)/m)，与 Image.Fill 的默认网格完全一致。
func GridSample(m, si, sj int) (sx, sy float64) {
	return (float64(si) + 0.5) / float64(m), (float64(sj) + 0.5) / float64(m)
}

// wrap01 把坐标折回 [0,1)（Cranley–Patterson 环面平移）。
func wrap01(x float64) float64 {
	x = math.Mod(x, 1)
	if x < 0 {
		x++
	}
	if x >= 1 {
		return 0
	}
	return x
}

// ShiftCP 对单位像素内样本施加 Cranley–Patterson 平移并环面折回。
func ShiftCP(sx, sy, ux, uy float64) (float64, float64) {
	return wrap01(sx + ux), wrap01(sy + uy)
}

// splitMix64 是确定性的 64 位伪随机数发生器（SplitMix64）。
// 不使用 math/rand 的全局流与版本相关的种子算法，保证跨版本可复现。
type splitMix64 struct{ state uint64 }

func newSplitMix64(seed int64) *splitMix64 {
	return &splitMix64{state: uint64(seed)}
}

// next 返回下一个均匀的 uint64。
func (r *splitMix64) next() uint64 {
	r.state += 0x9E3779B97F4A7C15
	z := r.state
	z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
	z = (z ^ (z >> 27)) * 0x94D049BB133111EB
	return z ^ (z >> 31)
}

// float64 返回 [0,1) 上的均匀双精度数（取高 53 位）。
func (r *splitMix64) float64() float64 {
	return float64(r.next()>>11) / float64(1<<53)
}

// deriveSeed 从主种子与两个下标确定性派生一个子种子（SplitMix64 搅拌），
// 使每个“角度×档位”格子拥有独立且与执行顺序无关的随机流。
func deriveSeed(master int64, a, b uint64) int64 {
	z := uint64(master) + 0x9E3779B97F4A7C15
	z ^= a*0xBF58476D1CE4E5B9 + 0x165667B19E3779F9
	z ^= b*0x94D049BB133111EB + 0xD1B54A32D192ED03
	z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
	z = (z ^ (z >> 27)) * 0x94D049BB133111EB
	z ^= z >> 31
	return int64(z)
}
