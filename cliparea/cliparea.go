package cliparea

import (
	"fmt"
	"math"
	"math/big"

	"vecgo/geom"
)

// U 是 binary64（float64）的单位舍入 u = 2^-53。
const U = 1.0 / (1 << 53)

// tieTol 是“点落在裁剪线上”的近零判定系数：
// 半平面符号 c=e×(p-a) 的一次浮点求值误差约 8u|e||p-a|，
// 故 |c| ≤ 8u|e||p-a| 时内/外判定不可信，记为一次二义。
const tieTol = 8 * U

// Domain 是三档分块裁剪所在的方形域边长。
const Domain = 10.0

// Levels 是三档裁剪的每维分块数：1×1、2×2、4×4。
var Levels = []int{1, 2, 4}

// GridWindows 返回把 [0,D]² 分成 k×k 个轴对齐方形窗口的窗口列表，
// 每个窗口按逆时针给出四个顶点。
func GridWindows(k int, D float64) [][]geom.Point {
	h := D / float64(k)
	ws := make([][]geom.Point, 0, k*k)
	for i := 0; i < k; i++ {
		for j := 0; j < k; j++ {
			x0, y0 := float64(i)*h, float64(j)*h
			x1, y1 := float64(i+1)*h, float64(j+1)*h
			ws = append(ws, []geom.Point{
				{X: x0, Y: y0}, {X: x1, Y: y0}, {X: x1, Y: y1}, {X: x0, Y: y1},
			})
		}
	}
	return ws
}

// tracked 是带坐标误差半径的顶点：|v_fl - v_exact| ≤ radius（欧氏范数）。
type tracked struct {
	p      geom.Point
	radius float64
}

// localIntersect 复刻 geom.intersect 的浮点运算，并给出本次交点的
// 一次舍入误差半径 δ。推导见 doc/clip-area-proof.md 第 3 节。
// 记 d=q-p、e=b-a、r=a-p，则 den=d×e，num=r×e，t=num/den，I=p+t·d：
//
//	δ = |d|·u·(8|r|/|d| + 4|num|/|den| + 3)
//
// 仅当 ci≠pi（确有跨线）时调用，精确算术中 den 必不为零；
// 因此不照搬 geom.intersect 的 1e-12 几何阈值——该阈值会把近线交点
// 误替换成外部顶点而引入拓扑性大误差。极小 den 对应的大舍入半径
// 由上界中的 1/sin 项如实吸收。
func localIntersect(p, q, a, b geom.Point) (geom.Point, float64) {
	dx, dy := q.X-p.X, q.Y-p.Y
	ex, ey := b.X-a.X, b.Y-a.Y
	den := dx*ey - dy*ex               // d×e
	num := (a.X-p.X)*ey - (a.Y-p.Y)*ex // r×e, r=a-p
	t := num / den
	I := geom.Point{X: p.X + dx*t, Y: p.Y + dy*t}

	dLen := math.Hypot(dx, dy)
	rLen := math.Hypot(a.X-p.X, a.Y-p.Y)
	delta := U * (8*rLen/dLen + 4*math.Abs(num)/math.Abs(den) + 3)
	return I, dLen * delta
}

// clipTracked 与 geom.Clip 同一条 Sutherland–Hodgman 流程，额外维护
// 每个顶点的误差半径，并统计线上二义次数。
func clipTracked(poly []geom.Point, window []geom.Point) (pts []tracked, ambiguous int) {
	out := make([]tracked, len(poly))
	for i, p := range poly {
		out[i] = tracked{p: p, radius: 0} // 输入为整数点，精确可表示，初始半径为 0
	}
	for i := range window {
		a := window[i]
		b := window[(i+1)%len(window)]
		ex, ey := b.X-a.X, b.Y-a.Y
		eLen := math.Hypot(ex, ey)
		classify := func(p geom.Point) (inside bool, ambiguous bool) {
			c := ex*(p.Y-a.Y) - ey*(p.X-a.X)
			amb := math.Abs(c) <= tieTol*eLen*math.Hypot(p.X-a.X, p.Y-a.Y)
			return c >= 0, amb
		}
		nxt := make([]tracked, 0, len(out)+2)
		for j := range out {
			cur := out[j]
			prv := out[(j-1+len(out))%len(out)]
			ci, ambC := classify(cur.p)
			pi, _ := classify(prv.p)
			// 每个顶点在每条裁剪线上至多被统计一次（只在它作 cur 时计数），
			// 统计口径为“顶点-裁剪线二义分类次数”。
			if ambC {
				ambiguous++
			}
			if ci != pi {
				I, local := localIntersect(prv.p, cur.p, a, b)
				// 新交点继承上游顶点的误差半径，再加本轮交点自身的舍入半径。
				nxt = append(nxt, tracked{p: I, radius: prv.radius + local})
			}
			if ci {
				nxt = append(nxt, cur)
			}
		}
		out = nxt
		if len(out) == 0 {
			break
		}
	}
	return out, ambiguous
}

// LevelReport 是一档裁剪的实测结果。
type LevelReport struct {
	K         int     // 每维分块数
	Pieces    int     // 非空裁剪块数
	Vertices  int     // 所有块的顶点总数 V
	Ambiguous int     // 线上二义分类次数
	FloatSum  float64 // 浮点各块鞋带面积之和
	ExactDiff float64 // 精确分块面积和 − 精确原面积（应为 0）
	AreaDiff  float64 // |浮点分块面积和 − 精确原面积|
	Bound     float64 // 实测上界 Σ_g (M·Σε + 2u n_g M_g²)
	Within    bool    // 面积差是否在界内（逐档判定）
}

// Run 在给定输入多边形上跑完三档裁剪。
func Run(poly []geom.Point) ([]LevelReport, error) {
	exactArea, _ := ExactArea(poly).Float64()
	// M：输入顶点坐标的最大欧氏范数，用于把顶点坐标误差换成面积误差。
	M := 0.0
	for _, p := range poly {
		if r := math.Hypot(p.X, p.Y); r > M {
			M = r
		}
	}
	reports := make([]LevelReport, 0, len(Levels))
	for _, k := range Levels {
		rep := LevelReport{K: k}
		exactSum := big.NewRat(0, 1)
		for _, w := range GridWindows(k, Domain) {
			tv, amb := clipTracked(poly, w)
			rep.Ambiguous += amb
			if len(tv) < 3 {
				continue
			}
			rep.Pieces++
			pts := make([]geom.Point, len(tv))
			epsSum := 0.0
			Mg := 0.0
			for i, t := range tv {
				pts[i] = t.p
				epsSum += t.radius
				if r := math.Hypot(t.p.X, t.p.Y); r > Mg {
					Mg = r
				}
			}
			rep.Vertices += len(tv)
			rep.FloatSum += geom.Shoelace(pts)
			// 面积上界：顶点坐标误差项 M·Σε，加每块鞋带公式自身的
			// 求值/求和舍入项 2u·n_g·M_g²（nu≪1 时的标准一阶界）。
			rep.Bound += M*epsSum + 2*U*float64(len(tv))*Mg*Mg
			exactSum.Add(exactSum, rShoelace(rClip(toRat(poly), RatWindow(w))))
		}
		rep.ExactDiff, _ = new(big.Rat).Sub(exactSum, ExactArea(poly)).Float64()
		rep.AreaDiff = math.Abs(rep.FloatSum - exactArea)
		rep.Within = rep.AreaDiff <= rep.Bound
		reports = append(reports, rep)
	}
	return reports, nil
}

// UniformCoefficient 给出与顶点数成正比的闭式上界系数 C（量纲为面积/顶点），
// 使每档都满足 AreaDiff ≤ C·V。C 由输入几何的最坏情形算出：
//
//	δ_max = L_max·u·(8√2 M/L_min + 4/s0 + 3)
//	C     = M·r·δ_max + 2u·M²
//
// 其中 r=4 是矩形窗口的裁剪轮数，L_max/L_min 是多边形最长/最短边，
// s0 是“实际发生跨线”的边与裁剪线夹角正弦 |d×e|/(|d||e|) 的最小值；
// 平行（不跨线）的边不产生交点，故不参与 s0，不会出现除零。
type UniformCoefficient struct {
	U        float64
	M        float64
	Lmax     float64
	Lmin     float64
	SinMin   float64
	Crossing int
	Rounds   int
	Delta    float64
	C        float64
}

// crossingSin 求线段 p→q 与裁剪线 a→b 交点处的夹角正弦 |d×e|/(|d||e|)。
func crossingSin(p, q, a, b geom.Point) float64 {
	dx, dy := q.X-p.X, q.Y-p.Y
	ex, ey := b.X-a.X, b.Y-a.Y
	dLen, eLen := math.Hypot(dx, dy), math.Hypot(ex, ey)
	return math.Abs(dx*ey-dy*ex) / (dLen * eLen)
}

// ComputeCoefficient 依据输入多边形与三档网格窗口统计最坏情形几何量并算出 C。
// s0 只统计真正跨线（边的两端分居裁剪线两侧）的交点，覆盖近线但不退化的情形。
func ComputeCoefficient(poly []geom.Point) UniformCoefficient {
	M := 0.0
	Lmax, Lmin := 0.0, math.Inf(1)
	for i := range poly {
		p, q := poly[i], poly[(i+1)%len(poly)]
		if r := math.Hypot(p.X, p.Y); r > M {
			M = r
		}
		l := math.Hypot(q.X-p.X, q.Y-p.Y)
		if l > Lmax {
			Lmax = l
		}
		if l < Lmin {
			Lmin = l
		}
	}
	sinMin := 1.0
	crossings := 0
	for _, k := range Levels {
		for _, w := range GridWindows(k, Domain) {
			for i := range w {
				a, b := w[i], w[(i+1)%len(w)]
				ex, ey := b.X-a.X, b.Y-a.Y
				for j := range poly {
					p, q := poly[j], poly[(j+1)%len(poly)]
					sp := ex*(p.Y-a.Y) - ey*(p.X-a.X)
					sq := ex*(q.Y-a.Y) - ey*(q.X-a.X)
					if (sp >= 0) != (sq >= 0) { // 真正跨线，必有交点
						sinMin = math.Min(sinMin, crossingSin(p, q, a, b))
						crossings++
					}
				}
			}
		}
	}
	if crossings == 0 {
		sinMin = 1 // 没有任何交点时，sin 项不被使用
	}
	const rounds = 4
	delta := Lmax * U * (8*math.Sqrt2*M/Lmin + 4/sinMin + 3)
	c := M*rounds*delta + 2*U*M*M
	return UniformCoefficient{
		U: U, M: M, Lmax: Lmax, Lmin: Lmin, SinMin: sinMin,
		Crossing: crossings, Rounds: rounds, Delta: delta, C: c,
	}
}

// FormatTable 把三档结果按“面积差与上界并列”排版。
func FormatTable(reports []LevelReport, coef UniformCoefficient) string {
	s := "档(k)  块数  顶点V  二义次数  面积差|ΔA|        实测上界B         C·V(闭式上界)     逐档判定\n"
	for _, r := range reports {
		verdict := "在界内"
		if !r.Within {
			verdict = "越界!"
		}
		cv := coef.C * float64(r.Vertices)
		s += fmt.Sprintf("%-5d  %-4d  %-5d  %-8d  %-15.3e  %-15.3e  %-15.3e  %s\n",
			r.K, r.Pieces, r.Vertices, r.Ambiguous, r.AreaDiff, r.Bound, cv, verdict)
	}
	s += fmt.Sprintf("\n精确路径核对: 三档 (精确分块面积和 − 精确原面积) 分别为 %.3e, %.3e, %.3e（应全为 0）\n",
		reports[0].ExactDiff, reports[1].ExactDiff, reports[2].ExactDiff)
	s += fmt.Sprintf("系数: u=2^-53=%.3e, M=%.4f, Lmax=%.4f, Lmin=%.4f, 跨线交点=%d, s0=%.6f, δmax=%.3e, C=%.3e\n",
		coef.U, coef.M, coef.Lmax, coef.Lmin, coef.Crossing, coef.SinMin, coef.Delta, coef.C)
	return s
}
