// Package seam 给出裁剪接缝处“边界像素归谁”的精确计数台账。
//
// 所有计数都在整数像素集合上完成：不做抽样、不引入亚像素覆盖率噪声，
// 因而“重复像素数”和“两块计数和与整块计数之差”可以逐项严格对账。
package seam

import "math"

// Rule 是接缝索引 k 处边界像素的端点归属规则。
//
// 内部一律用半开整数区间 [Lo,Hi) 表示一块像素，端点规则只改变区间边界。
type Rule string

const (
	// ClosedClosed 双闭：左 [0,k]（即 [0,k+1)），右 [k,W)。
	// 索引 k 的像素两块都拿到，必然重复计数。
	ClosedClosed Rule = "closed_closed"
	// OpenOpen 双开（相对接缝）：左 [0,k)，右 (k,W)（即 [k+1,W)）。
	// 索引 k 的像素两块都不拿，必然漏一列。
	OpenOpen Rule = "open_open"
	// HalfOpenRight 标准半开半闭：左 [0,k)，右 [k,W)，边界像素归右。
	HalfOpenRight Rule = "half_open_right"
	// HalfOpenLeft 反向互补半开半闭：左 [0,k]，右 (k,W)，边界像素归左。
	HalfOpenLeft Rule = "half_open_left"
)

// AllRules 是逐类核对时遍历的四种端点取舍。
func AllRules() []Rule {
	return []Rule{ClosedClosed, OpenOpen, HalfOpenRight, HalfOpenLeft}
}

// Class 是裁剪位置的三类划分。
type Class string

const (
	ClassInteger Class = "integer"  // 整数缝 s = k
	ClassHalf    Class = "half"     // 半像素缝 s = k + 1/2（穿过像素中心，取整平局位）
	ClassFrac    Class = "fraction" // 一般小数缝 s = k + f，f∈(0,1)\{1/2}
)

// Band 是一条横贯全宽的实心覆盖带：行 [Y0,Y1) 内每列都覆盖。
// 用它做实验可以让“接缝列上有多少被覆盖像素”等于固定的带高，便于手算核对。
type Band struct {
	W, H   int
	Y0, Y1 int
}

// BandHeight 返回覆盖带高度（接缝一列上的被覆盖像素数）。
func (b Band) BandHeight() int { return b.Y1 - b.Y0 }

// Covered 判定整数像素坐标 (x,y) 是否被整块覆盖。
func (b Band) Covered(x, y int) bool {
	return x >= 0 && x < b.W && y >= b.Y0 && y < b.Y1
}

// WholeCount 是整块的覆盖像素数。
func (b Band) WholeCount() int { return b.W * (b.Y1 - b.Y0) }

// Span 是整数像素索引上的半开区间 [Lo,Hi)。
type Span struct{ Lo, Hi int }

func (iv Span) Has(x int) bool { return x >= iv.Lo && x < iv.Hi }

// ruleBounds 给出规则 r 在缝索引 k 处的左右半开区间。
func ruleBounds(k int, r Rule) (L, R Span) {
	switch r {
	case ClosedClosed:
		return Span{0, k + 1}, Span{k, 1 << 30}
	case OpenOpen:
		return Span{0, k}, Span{k + 1, 1 << 30}
	case HalfOpenRight:
		return Span{0, k}, Span{k, 1 << 30}
	case HalfOpenLeft:
		return Span{0, k + 1}, Span{k + 1, 1 << 30}
	default:
		panic("seam: 未知端点规则 " + string(r))
	}
}

// RuleSpans 给出规则 r 在画布宽度 W、缝索引 k 处的左右半开区间，右端收回到 W。
func RuleSpans(W, k int, r Rule) (L, R Span) {
	l, rr := ruleBounds(k, r)
	l.Hi = minInt(l.Hi, W)
	rr.Hi = minInt(rr.Hi, W)
	return l, rr
}

// Ledger 是一次“整块 vs 两块”对账的完整结果。
type Ledger struct {
	Rule  Rule  `json:"rule"`
	Class Class `json:"class,omitempty"`
	// S 是连续缝坐标（若该次核对来自连续裁剪位置）。
	S float64 `json:"s,omitempty"`
	// K 是对账使用的整数缝索引。
	K int `json:"k"`

	LeftLo  int `json:"left_lo"`
	LeftHi  int `json:"left_hi"`
	RightLo int `json:"right_lo"`
	RightHi int `json:"right_hi"`

	// Whole/Left/Right 是覆盖像素数。
	Whole int `json:"whole"`
	Left  int `json:"left"`
	Right int `json:"right"`

	// DupPixels/GapPixels 是几何意义上的重复/漏空像素（不看是否被覆盖）。
	DupPixels int `json:"dup_pixels"`
	GapPixels int `json:"gap_pixels"`
	// DupCovered/GapCovered 是其中真正被覆盖的像素数。
	// 计数差恒等于 DupCovered-GapCovered。
	DupCovered int `json:"dup_covered"`
	GapCovered int `json:"gap_covered"`

	// Delta = Left+Right-Whole。
	Delta int `json:"delta"`
	// IdentityResidual = Delta-(DupCovered-GapCovered)，必须恒为 0。
	IdentityResidual int `json:"identity_residual"`
	// HalfOpenDupNonzero 是“半开规则下仍有重复”的失败信号。
	HalfOpenDupNonzero bool `json:"half_open_dup_nonzero"`
}

// Reconcile 在给定左右像素区间上做完整集合对账。
//
// 对每个像素只检查四件事：在不在左块、在不在右块、重没重复、漏没漏空。
func Reconcile(b Band, class Class, s float64, k int, rule Rule, l, r Span) Ledger {
	le := Ledger{
		Rule: rule, Class: class, S: s, K: k,
		LeftLo: clampLo(l.Lo), LeftHi: l.Hi,
		RightLo: clampLo(r.Lo), RightHi: r.Hi,
		Whole: b.WholeCount(),
	}
	// 把右端的“无穷大”哨兵收回到画布宽度，仅用于展示。
	if le.LeftHi > b.W {
		le.LeftHi = b.W
	}
	if le.RightHi > b.W {
		le.RightHi = b.W
	}

	for y := 0; y < b.H; y++ {
		for x := 0; x < b.W; x++ {
			inL, inR := l.Has(x), r.Has(x)
			cov := b.Covered(x, y)
			if inL && cov {
				le.Left++
			}
			if inR && cov {
				le.Right++
			}
			switch {
			case inL && inR:
				le.DupPixels++
				if cov {
					le.DupCovered++
				}
			case !inL && !inR:
				le.GapPixels++
				if cov {
					le.GapCovered++
				}
			}
		}
	}
	le.Delta = le.Left + le.Right - le.Whole
	le.IdentityResidual = le.Delta - (le.DupCovered - le.GapCovered)
	if rule == HalfOpenRight || rule == HalfOpenLeft {
		le.HalfOpenDupNonzero = le.DupCovered != 0 || le.GapCovered != 0
	}
	return le
}

func clampLo(v int) int {
	if v < 0 {
		return 0
	}
	return v
}

// ReconcileRule 按端点规则 r 在整数缝 k 处对账。
func ReconcileRule(b Band, class Class, s float64, k int, r Rule) Ledger {
	l, rr := ruleBounds(k, r)
	// 把哨兵右端限制在画布内，避免无意义的超大区间参与 has（has 只比较整数，安全）。
	rr.Hi = b.W
	l.Hi = minInt(l.Hi, b.W)
	if l.Lo < 0 {
		l.Lo = 0
	}
	if rr.Lo < 0 {
		rr.Lo = 0
	}
	return Reconcile(b, class, s, k, r, l, rr)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// SnapPolicy 是把连续裁剪缝 s 落成整数像素区间的取整口径。
type SnapPolicy string

const (
	// SnapFloorShared 只取整一次：k=floor(s)，两块共用 k 并采用 [0,k)+[k,W)。
	// 集合严格划分，Δ 恒为 0；取整只改变跨缝列归谁，不改变总数。
	SnapFloorShared SnapPolicy = "floor_shared_halfopen"
	// SnapOutwardHalfOpen 两端各自向外取整：左 [0,ceil(s))，右 [floor(s),W)。
	// 非整数缝会让 floor(s) 列被两块同时包住——取整造成的一列重叠。
	SnapOutwardHalfOpen SnapPolicy = "outward_ceil_floor"
	// SnapInwardTrunc 两端各自向内截断：左 [0,floor(s))，右 [ceil(s),W)。
	// 非整数缝会漏掉 floor(s) 列——取整造成的一列空隙。
	SnapInwardTrunc SnapPolicy = "inward_floor_ceil"
	// SnapNearestSeparate 两块宽度各自四舍五入（半数向上），再从左右外缘锚定。
	// 只有半像素平局缝可能因两端同时进位而多出一列。
	SnapNearestSeparate SnapPolicy = "nearest_separate_halfup"
)

// RoundingRow 是一种取整口径在一类裁剪位置上的核对结果。
type RoundingRow struct {
	Policy SnapPolicy `json:"policy"`
	Class  Class      `json:"class"`
	S      float64    `json:"s"`

	LeftInterval  [2]int `json:"left_interval"`
	RightInterval [2]int `json:"right_interval"`

	Left  int `json:"left"`
	Right int `json:"right"`
	Sum   int `json:"sum"`
	Whole int `json:"whole"`
	Delta int `json:"delta"`

	// RoundingTermColumns 是纯取整口径预言的列差（右宽+左宽-W，按列计）。
	RoundingTermColumns int `json:"rounding_term_columns"`
	// RoundingTerm = 带高 × 列差。
	RoundingTerm int `json:"rounding_term"`
	// Unexplained = Δ - RoundingTerm，必须为 0：差值只能由取整解释。
	Unexplained int `json:"unexplained"`

	DupCovered int `json:"dup_covered"`
	GapCovered int `json:"gap_covered"`
}

// RoundHalfUp 实现四舍五入、半数向 +∞ 进位。
func RoundHalfUp(x float64) int {
	return int(math.Floor(x + 0.5))
}

// ReconcileSnap 按取整口径把连续缝 s 落成两个整数区间并对账。
func ReconcileSnap(b Band, class Class, s float64, p SnapPolicy) RoundingRow {
	var l, r Span
	predCols := 0
	switch p {
	case SnapFloorShared:
		k := int(math.Floor(s))
		l, r = Span{0, k}, Span{k, b.W}
	case SnapOutwardHalfOpen:
		kf, kc := int(math.Floor(s)), int(math.Ceil(s))
		l, r = Span{0, kc}, Span{kf, b.W}
		predCols = kc - kf
	case SnapInwardTrunc:
		kf, kc := int(math.Floor(s)), int(math.Ceil(s))
		l, r = Span{0, kf}, Span{kc, b.W}
		predCols = kf - kc
	case SnapNearestSeparate:
		wL := RoundHalfUp(s)
		wR := RoundHalfUp(float64(b.W) - s)
		l, r = Span{0, wL}, Span{b.W - wR, b.W}
		predCols = wL + wR - b.W
	default:
		panic("seam: 未知取整口径 " + string(p))
	}
	le := Reconcile(b, class, s, int(math.Floor(s)), HalfOpenRight, l, r)
	row := RoundingRow{
		Policy: p, Class: class, S: s,
		LeftInterval:  [2]int{l.Lo, minInt(l.Hi, b.W)},
		RightInterval: [2]int{maxInt(r.Lo, 0), r.Hi},
		Left:          le.Left, Right: le.Right, Sum: le.Left + le.Right,
		Whole: le.Whole, Delta: le.Delta,
		RoundingTermColumns: predCols,
		RoundingTerm:        b.BandHeight() * predCols,
		Unexplained:         le.Delta - b.BandHeight()*predCols,
		DupCovered:          le.DupCovered, GapCovered: le.GapCovered,
	}
	return row
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Classify 按小数部分把缝坐标归入三类裁剪位置。
func Classify(s float64) Class {
	f := s - math.Floor(s)
	switch {
	case f == 0:
		return ClassInteger
	case math.Abs(f-0.5) < 1e-12:
		return ClassHalf
	default:
		return ClassFrac
	}
}
