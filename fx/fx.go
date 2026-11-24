// Package fx 提供两条渲染路径共用的定点数原语。
//
// 定点数用 int64 承载，真实值 = 存储值 / 2^Q，Q 由调用方通过 shift 参数给出。
// 整型路径全程使用本包的整数运算；对齐浮点路径用相同的 Q 量化中间结果，
// 从而保证两条路径在同一精度网格上计算。
package fx

import "math/bits"

// T 是定点标量。具体精度由各函数的 shift 参数决定，类型本身不携带 Q。
type T int64

// FromFloat 把浮点数量化为 Q=shift 的定点值，采用四舍五入（负数对称）。
func FromFloat(v float64, shift uint) T {
	scale := float64(int64(1) << shift)
	if v >= 0 {
		return T(v*scale + 0.5)
	}
	return T(v*scale - 0.5)
}

// Float 把定点值还原为浮点数。
func (x T) Float(shift uint) float64 {
	return float64(x) / float64(int64(1)<<shift)
}

// MulShr 返回 (a*b) >> shift 的有符号结果。
//
// 乘积可能超出 int64，因此先做完整的 128 位有符号乘法，再做算术右移。
// 右移为向零截断（非负数即向下取整），两条路径必须使用同一舍入方向。
func MulShr(a, b T, shift uint) T {
	hi, lo := mul128(int64(a), int64(b))
	return T(sar128(hi, lo, shift))
}

// Div128 返回有符号 128 位被除数 (hi,lo) 除以 d 的商，向零截断。
// 供光栅化阶段 (大整数常量)>>Q / 步长 这类分子超过 int64 的除法使用。
func Div128(hi, lo int64, d int64) int64 {
	neg := hi < 0
	if neg {
		hi, lo = neg128(hi, lo)
	}
	// 取绝对值后，bits.Div64 得到 |被除数|/d 的非负向下取整商。
	// 原数为负时直接对商取负即得到向零截断结果（整除与不整除均成立）。
	q, _ := bits.Div64(uint64(hi), uint64(lo), uint64(d))
	if neg {
		q = -q
	}
	return int64(q)
}

// RoundDiv 返回有符号 a/b 的四舍五入商（.5 远离零），用于不同精度间的换算。
func RoundDiv(a, b int64) int64 {
	if (a >= 0) == (b >= 0) {
		return (a + b/2) / b
	}
	return (a - b/2) / b
}

// mul128 返回有符号 a*b 的 128 位结果 (hi,lo)。
func mul128(a, b int64) (hi, lo int64) {
	neg := (a < 0) != (b < 0)
	ua, ub := uint64(a), uint64(b)
	if a < 0 {
		ua = -ua
	}
	if b < 0 {
		ub = -ub
	}
	uhi, ulo := bits.Mul64(ua, ub)
	if neg {
		nh, nl := neg128(int64(uhi), int64(ulo))
		uhi, ulo = uint64(nh), uint64(nl)
	}
	return int64(uhi), int64(ulo)
}

// sar128 对有符号 128 位数做算术右移 shift 位。
func sar128(hi, lo int64, shift uint) int64 {
	if shift == 0 {
		return lo
	}
	if shift >= 64 {
		return hi >> (shift - 64)
	}
	uhi := uint64(hi)
	ulo := uint64(lo)
	return int64((uhi << (64 - shift)) | (ulo >> shift))
}

// neg128 返回有符号 128 位数的相反数。
func neg128(hi, lo int64) (int64, int64) {
	nlo := ^lo + 1
	nhi := ^hi
	if nlo == 0 {
		nhi++
	}
	return nhi, nlo
}
