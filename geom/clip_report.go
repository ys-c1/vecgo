package geom

import (
	"fmt"
)

// ClipDiagnosis 是带拓扑诊断的一次裁剪记录。
//
// Raw/After 都是欧拉计数：Raw 是 SH 原始顶点流，After 是清理后的环。
// 退化顶点分三段统计，互不重叠：
//
//	Removed      清理时被合并/删除的点（相邻重复、共线中间点、零面积冗余）；
//	Residual     清理后仍存在、无法靠合并相邻顶点消除的退化；
//	Verdict      汇总判定（是否满足不变量、差几个、差在哪一类）。
//
// 因为 Removed 会把原始流里“多算的顶点”删掉，所以只比较
// After.V−Before.V 会互相抵消；必须同时看 Raw.V 与 After.V。
type ClipDiagnosis struct {
	Poly, Window []Point
	Raw, Clean   []Point
	Before       EulerCounts // 输入多边形
	RawCounts    EulerCounts // 未清理的 SH 输出
	After        EulerCounts // 清理后的环
	Removed      []DegenerateVertex
	Residual     []DegenerateVertex
	Err          error
}

// RawInflated 是原始流相对清理结果多出来的顶点数（即“被清理掉几个”）。
func (d ClipDiagnosis) RawInflated() int { return d.RawCounts.V - d.After.V }

// InvariantSummary 给出报告要求的三项量：Before、After、差值，逐框可列。
func (d ClipDiagnosis) InvariantSummary() string {
	return fmt.Sprintf("前: %s | 后: %s | ΔV=%+d ΔE=%+d Δχ=%+d | 退化=%d (%s)",
		d.Before, d.After,
		d.After.V-d.Before.V, d.After.E-d.Before.E, d.After.Chi-d.Before.Chi,
		len(d.Removed)+len(d.Residual), kindSummary(append(append([]DegenerateVertex{}, d.Removed...), d.Residual...)))
}

// ClipReport 执行裁剪并给出完整拓扑诊断：原始流、清理后环、
// 裁剪前后的欧拉计数，以及被清理/残留的退化顶点。
func ClipReport(poly, window []Point) ClipDiagnosis {
	d := ClipDiagnosis{Poly: poly, Window: window}
	raw, err := ClipRaw(poly, window)
	if err != nil {
		d.Err = err
		return d
	}
	clean := CleanPolygon(raw)
	d.Raw = raw
	d.Clean = clean.Ring
	d.Before = EulerOf(poly)
	d.RawCounts = EulerOf(raw)
	d.After = EulerOf(clean.Ring)
	d.Removed = clean.Removed
	d.Residual = clean.Residual
	return d
}

// InvariantError 描述欧拉不变量被破坏时的差值明细：差几个、差在哪一类。
type InvariantError struct {
	Where   string
	Before  EulerCounts
	After   EulerCounts
	DeltaV  int // 顶点数变化
	DeltaE  int // 边数变化
	ChiDiff int // χ 的变化
	Degener []DegenerateVertex
}

func (e *InvariantError) Error() string {
	return fmt.Sprintf("%s 不满足 V-E+F=2：%s；ΔV=%+d ΔE=%+d；类别 %s；坐标 %s",
		e.Where, Explain(e.After, e.Degener), e.DeltaV, e.DeltaE,
		kindSummary(e.Degener), Coords(e.Degener, 5))
}

// KindSummary 汇总退化类别与数量，如“相邻重复顶点×2、共线中间顶点×1”。
func KindSummary(degs []DegenerateVertex) string { return kindSummary(degs) }

// SortedKinds 返回按字典序排列的退化类别，供报告稳定输出。
func SortedKinds(degs []DegenerateVertex) []DegenerateKind { return sortedKinds(degs) }

func kindSummary(degs []DegenerateVertex) string {
	counts := KindCounts(degs)
	if len(counts) == 0 {
		return "无"
	}
	out := ""
	for _, k := range sortedKinds(degs) {
		if out != "" {
			out += "、"
		}
		out += fmt.Sprintf("%s×%d", k, counts[k])
	}
	return out
}

// CheckInvariant 校验清环后的结果满足 V-E+F=2，且没有残留捏点/自交。
// 不满足时返回 *InvariantError，差值精确到“差几个、差在哪一类”。
// 单点/线段/空环属于“无面积”，不视为违反不变量（χ 仍为 2），
// 由调用方用 HasArea 判断。
func (d ClipDiagnosis) CheckInvariant(where string) error {
	if d.Err != nil {
		return d.Err
	}
	if d.After.EulerSatisfied() && !HasTopologicalDefect(d.Residual) {
		return nil
	}
	return &InvariantError{
		Where:   where,
		Before:  d.Before,
		After:   d.After,
		DeltaV:  d.After.V - d.Before.V,
		DeltaE:  d.After.E - d.Before.E,
		ChiDiff: d.After.Chi - d.Before.Chi,
		Degener: append(append([]DegenerateVertex{}, d.Removed...), d.Residual...),
	}
}

// HasArea 表示清理后的环围出非零面积（才是一个有效面）。
func (d ClipDiagnosis) HasArea() bool {
	return len(d.Clean) >= 3 && !zeroArea(d.Clean)
}

// InvariantHolds 是布尔便捷形式：环是否满足 V-E+F=2 且无残缺陷。
func InvariantHolds(poly []Point) bool { return EulerOf(poly).EulerSatisfied() }
