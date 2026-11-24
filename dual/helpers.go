package dual

import (
	"math"
	"math/bits"
)

// mul128 返回有符号 a*b 的 128 位结果。
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
		return neg128(int64(uhi), int64(ulo))
	}
	return int64(uhi), int64(ulo)
}

// add128 返回两个有符号 128 位数之和。
func add128(hi1, lo1, hi2, lo2 int64) (hi, lo int64) {
	ulo, carry := bits.Add64(uint64(lo1), uint64(lo2), 0)
	uhi, _ := bits.Add64(uint64(hi1), uint64(hi2), carry)
	return int64(uhi), int64(ulo)
}

// cmp128 比较两个有符号 128 位数：a>b 返回 1，相等返回 0，否则 -1。
func cmp128(hi1, lo1, hi2, lo2 int64) int {
	if hi1 != hi2 {
		if hi1 > hi2 {
			return 1
		}
		return -1
	}
	if uint64(lo1) > uint64(lo2) {
		return 1
	}
	if uint64(lo1) < uint64(lo2) {
		return -1
	}
	return 0
}

// div64 是无符号 128/64 除法，返回商和余数。
func div64(hi, lo, d uint64) (q, r uint64) {
	return bits.Div64(hi, lo, d)
}

// neg128 返回有符号 128 位数的相反数。
func neg128(hi, lo int64) (int64, int64) {
	lo = ^lo + 1
	hi = ^hi
	if lo == 0 {
		hi++
	}
	return hi, lo
}

// cos/sin 仅在入口量化时使用一次。
func cos(rad float64) float64 { return math.Cos(rad) }
func sin(rad float64) float64 { return math.Sin(rad) }
