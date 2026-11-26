package geom

import (
	"errors"
	"math"
)

// Triangle 是逆时针给出的三角形。
type Triangle [3]Point

// Area 返回三角形面积。
func (t Triangle) Area() float64 {
	return math.Abs(cross(t[0], t[1], t[2])) / 2
}

// Perimeter 返回三边长度之和。
func (t Triangle) Perimeter() float64 {
	return dist(t[0], t[1]) + dist(t[1], t[2]) + dist(t[2], t[0])
}

// Side2Sum 返回三边长度的平方和。
func (t Triangle) Side2Sum() float64 {
	return sq(dist(t[0], t[1])) + sq(dist(t[1], t[2])) + sq(dist(t[2], t[0]))
}

// Quality 是常用的三角形面积比 4√3·A/(a²+b²+c²)。
// 等边三角形为 1，越接近 0 表示越细长。
func (t Triangle) Quality() float64 {
	s := t.Side2Sum()
	if s == 0 {
		return 0
	}
	return 4 * math.Sqrt(3) * t.Area() / s
}

// MinAngle 返回最小内角（弧度）。
func (t Triangle) MinAngle() float64 {
	a := angleAt(t[0], t[1], t[2])
	b := angleAt(t[1], t[2], t[0])
	c := angleAt(t[2], t[0], t[1])
	return math.Min(a, math.Min(b, c))
}

// Centroid 返回重心。
func (t Triangle) Centroid() Point {
	return Point{(t[0].X + t[1].X + t[2].X) / 3, (t[0].Y + t[1].Y + t[2].Y) / 3}
}

func sq(x float64) float64 { return x * x }

func dist(a, b Point) float64 { return math.Hypot(a.X-b.X, a.Y-b.Y) }

func cross(o, a, b Point) float64 {
	return (a.X-o.X)*(b.Y-o.Y) - (a.Y-o.Y)*(b.X-o.X)
}

func angleAt(v, a, b Point) float64 {
	ux, uy := a.X-v.X, a.Y-v.Y
	wx, wy := b.X-v.X, b.Y-v.Y
	nu, nw := math.Hypot(ux, uy), math.Hypot(wx, wy)
	if nu == 0 || nw == 0 {
		return 0
	}
	c := (ux*wx + uy*wy) / (nu * nw)
	return math.Acos(math.Max(-1, math.Min(1, c)))
}

// SignedArea 返回带符号面积，逆时针为正。
func SignedArea(poly []Point) float64 {
	n := len(poly)
	s := 0.0
	for i := 0; i < n; i++ {
		j := (i + 1) % n
		s += poly[i].X*poly[j].Y - poly[j].X*poly[i].Y
	}
	return s / 2
}

// InteriorAngles 返回每个顶点的内角（弧度），凹角大于 π。
//
// 取入边方向 d1 = v[i] - v[i-1] 与出边方向 d2 = v[i+1] - v[i]，
// 转向角 τ = atan2(d1×d2, d1·d2)。逆时针多边形内角为 π − τ，
// 顺时针为 π + τ；凹顶点的 τ 与方向相反，结果自然大于 π。
func InteriorAngles(poly []Point) ([]float64, error) {
	n := len(poly)
	if n < 3 {
		return nil, errors.New("多边形至少三个顶点")
	}
	s := 1.0
	if SignedArea(poly) < 0 {
		s = -1
	}
	out := make([]float64, n)
	for i := 0; i < n; i++ {
		prev := poly[(i-1+n)%n]
		cur := poly[i]
		next := poly[(i+1)%n]
		d1x, d1y := cur.X-prev.X, cur.Y-prev.Y
		d2x, d2y := next.X-cur.X, next.Y-cur.Y
		turn := math.Atan2(d1x*d2y-d1y*d2x, d1x*d2x+d1y*d2y)
		out[i] = math.Pi - s*turn
	}
	return out, nil
}

// MinInteriorAngle 返回最小内角（弧度）及其顶点下标。
func MinInteriorAngle(poly []Point) (float64, int, error) {
	angles, err := InteriorAngles(poly)
	if err != nil {
		return 0, -1, err
	}
	best, idx := math.Inf(1), -1
	for i, a := range angles {
		if a < best {
			best, idx = a, i
		}
	}
	return best, idx, nil
}

// QualityCeiling 返回最小角为 theta 的三角形所能达到的最大面积比。
// 由等腰三角形 (theta, (π-theta)/2, (π-theta)/2) 取得：
//
//	q(θ) = 2√3·cos²(θ/2)·sinθ / (sin²θ + 2cos²(θ/2))
//
// 该函数在 [0, π/3] 上严格单调递增，q(π/3)=1。
func QualityCeiling(theta float64) float64 {
	if theta <= 0 {
		return 0
	}
	if theta >= math.Pi/3 {
		return 1
	}
	s := math.Sin(theta)
	c2 := math.Cos(theta / 2)
	c2 *= c2
	return 2 * math.Sqrt(3) * c2 * s / (s*s + 2*c2)
}

// AngleUpperBound 是“任意仅使用多边形顶点的三角化”都无法越过的最小角上界：
//
//	θ(𝒯) ≤ U(α) = min{ α, (π−α)/2 },
//
// 其中 α 是多边形的最小内角，θ(𝒯) 是三角化 𝒯 中所有三角形的最小角。
//
// 两个分支分别来自两条独立论证：
//
//   - 系数 1 的 α 项：取内角恰为 α 的顶点 v*。从 v* 出发的 k 条边把
//     张角 α 切成 k≥1 个三角形角，这些角之和恰为 α（角度可加性），
//     故至少一个 ≤ α/k ≤ α。系数 1 即“整角等于各部分之和”，没有缩放。
//
//   - 系数 1/2 的 (π−α)/2 项：三角化的对偶图是一棵树（n−2 个结点），
//     因而至少有两个叶子；叶子三角形带两条多边形边，是一个“耳”
//     (v_prev, v, v_next)。耳尖 v 必为凸顶点，耳三角形在 v 处的角就是
//     多边形内角 α_v ≥ α。由三角形内角和为 π（欧几里得《几何原本》
//     第 I 卷命题 32），另外两个角之和为 π−α_v，于是其中较小者
//     ≤ (π−α_v)/2 ≤ (π−α)/2。这里的系数 1/2 来自“两个角分配
//     π−α_v，均值为其一半”。耳的存在也可直接引用 Meisters 的
//     两耳定理（G. H. Meisters, “Polygons have ears”,
//     Amer. Math. Monthly 82(6), 1975, 648–651）。
//
// 两条直线在 α=π/3（60°）相交，故 U(α) ≤ π/3，自动包含普适界
// θ≤60°（π/3 的系数 1/3 来自内角和 π 由三个角分摊）。
//
// 该界关于单一参数 α 是紧的：α≤π/3 一侧由三角形本身取到 θ=α；
// α≥π/3 一侧由正 m 边形取到 α=π−2π/m、(π−α)/2=π/m，
// 从一个顶点扇出的三角化在该顶点处每个角恰为 π/m。
func AngleUpperBound(alpha float64) float64 {
	return math.Min(alpha, (math.Pi-alpha)/2)
}

// MaxMinAngleBound 返回给定多边形的最小角上界 U(α)（弧度）。
func MaxMinAngleBound(poly []Point) (float64, error) {
	alpha, _, err := MinInteriorAngle(poly)
	if err != nil {
		return 0, err
	}
	return AngleUpperBound(alpha), nil
}

// FanBound 是上界的细分形式：若三角化在顶点 v 处有 k 个三角形，
// 则其中必有一个在 v 处的角不超过 α_v/k。
func FanBound(poly []Point, v, k int) float64 {
	angles, err := InteriorAngles(poly)
	if err != nil || v < 0 || v >= len(angles) || k < 1 {
		return 0
	}
	return angles[v] / float64(k)
}

// CleanPolygon 去掉相邻重复顶点（含首尾）。
func CleanPolygon(poly []Point) []Point {
	const eps = 1e-12
	out := make([]Point, 0, len(poly))
	for _, p := range poly {
		if len(out) > 0 && dist(out[len(out)-1], p) < eps {
			continue
		}
		out = append(out, p)
	}
	for len(out) > 1 && dist(out[0], out[len(out)-1]) < eps {
		out = out[:len(out)-1]
	}
	return out
}

// IsSimple 判断多边形是否为简单多边形（除相邻边外无自交）。
func IsSimple(poly []Point) bool {
	n := len(poly)
	if n < 3 {
		return false
	}
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			if j == i+1 || (i == 0 && j == n-1) {
				continue
			}
			if segmentsIntersect(poly[i], poly[(i+1)%n], poly[j], poly[(j+1)%n]) {
				return false
			}
		}
	}
	return true
}

func segmentsIntersect(a, b, c, d Point) bool {
	const eps = 1e-9
	o := func(p, q, r Point) int {
		v := cross(p, q, r)
		switch {
		case v > eps:
			return 1
		case v < -eps:
			return -1
		}
		return 0
	}
	o1, o2 := o(a, b, c), o(a, b, d)
	o3, o4 := o(c, d, a), o(c, d, b)
	if o1*o2 < 0 && o3*o4 < 0 {
		return true
	}
	if o1 == 0 && pointOnSegment(c, a, b) {
		return true
	}
	if o2 == 0 && pointOnSegment(d, a, b) {
		return true
	}
	if o3 == 0 && pointOnSegment(a, c, d) {
		return true
	}
	if o4 == 0 && pointOnSegment(b, c, d) {
		return true
	}
	return false
}

func pointOnSegment(p, a, b Point) bool {
	const eps = 1e-9
	if math.Abs(cross(a, b, p)) > eps {
		return false
	}
	if p.X < math.Min(a.X, b.X)-eps || p.X > math.Max(a.X, b.X)+eps {
		return false
	}
	if p.Y < math.Min(a.Y, b.Y)-eps || p.Y > math.Max(a.Y, b.Y)+eps {
		return false
	}
	return true
}

func pointStrictlyInsideTriangle(p, a, b, c Point) bool {
	const eps = 1e-9
	s := cross(a, b, c)
	if math.Abs(s) < eps {
		return false
	}
	return s*cross(a, b, p) > eps && s*cross(b, c, p) > eps && s*cross(c, a, p) > eps
}

func pointInTriangleInclusive(p, a, b, c Point) bool {
	const eps = 1e-9
	s := cross(a, b, c)
	if math.Abs(s) < eps {
		return false
	}
	return s*cross(a, b, p) >= -eps && s*cross(b, c, p) >= -eps && s*cross(c, a, p) >= -eps
}

// normalizeCCW 返回逆时针排列的副本。
func normalizeCCW(poly []Point) []Point {
	out := append([]Point(nil), poly...)
	if SignedArea(out) < 0 {
		for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
			out[i], out[j] = out[j], out[i]
		}
	}
	return out
}

// EarClip 是基线三角化：按顶点顺序反复剪去第一个合法的耳朵。
// 它以圆形的面积为代价换取简单，容易在顶点附近产出细长三角形。
func EarClip(poly []Point) ([]Triangle, error) {
	if len(poly) < 3 {
		return nil, errors.New("多边形至少三个顶点")
	}
	p := normalizeCCW(CleanPolygon(poly))
	if len(p) < 3 || !IsSimple(p) {
		return nil, errors.New("不是简单多边形")
	}
	n := len(p)
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	tris := make([]Triangle, 0, n-2)
	for len(idx) > 3 {
		m := len(idx)
		cut := -1
		for t := 0; t < m && cut < 0; t++ {
			i0, i1, i2 := idx[(t-1+m)%m], idx[t], idx[(t+1)%m]
			a, b, c := p[i0], p[i1], p[i2]
			if cross(a, b, c) <= 0 {
				continue
			}
			ok := true
			for _, j := range idx {
				if j == i0 || j == i1 || j == i2 {
					continue
				}
				if pointInTriangleInclusive(p[j], a, b, c) {
					ok = false
					break
				}
			}
			if ok {
				cut = t
				tris = append(tris, Triangle{a, b, c})
			}
		}
		if cut < 0 {
			return nil, errors.New("找不到耳朵（多边形退化或共线顶点过多）")
		}
		idx = append(idx[:cut], idx[cut+1:]...)
	}
	a, b, c := p[idx[0]], p[idx[1]], p[idx[2]]
	if cross(a, b, c) <= 0 {
		return nil, errors.New("最后一个三角形退化")
	}
	tris = append(tris, Triangle{a, b, c})
	return tris, nil
}

// OptimalTriangulation 是改进策略：在所有三角化中最大化最小面积比
// （即让最差三角形尽量好），用区间动态规划求解，复杂度 O(n⁴)。
//
// 由上界 θ(𝒯) ≤ U(α)=min(α,(π−α)/2) 与面积比包络
// q_min ≤ QualityCeiling(U(α)) 可知任何“仅用原顶点”的三角化都有天花板，
// 该 DP 在给定顶点集下让最坏面积比达到全局最大（瓶颈最优）。
func OptimalTriangulation(poly []Point) ([]Triangle, error) {
	if len(poly) < 3 {
		return nil, errors.New("多边形至少三个顶点")
	}
	p := normalizeCCW(CleanPolygon(poly))
	n := len(p)
	if n < 3 || !IsSimple(p) {
		return nil, errors.New("不是简单多边形")
	}
	if n == 3 {
		if cross(p[0], p[1], p[2]) <= 0 {
			return nil, errors.New("三角形退化")
		}
		return []Triangle{{p[0], p[1], p[2]}}, nil
	}

	valid := make([][]bool, n)
	for i := range valid {
		valid[i] = make([]bool, n)
	}
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			valid[i][j] = diagonalOK(p, i, j)
			valid[j][i] = valid[i][j]
		}
	}
	triOK := func(i, k, j int) bool {
		if !valid[i][k] || !valid[k][j] || !valid[i][j] {
			return false
		}
		a, b, c := p[i], p[k], p[j]
		if cross(a, b, c) <= 0 {
			return false
		}
		cen := Point{(a.X + b.X + c.X) / 3, (a.Y + b.Y + c.Y) / 3}
		if !Inside(p, cen.X, cen.Y) {
			return false
		}
		for m := 0; m < n; m++ {
			if m == i || m == k || m == j {
				continue
			}
			if pointStrictlyInsideTriangle(p[m], a, b, c) {
				return false
			}
		}
		return true
	}

	dp := make([][]float64, n)
	ch := make([][]int, n)
	for i := range dp {
		dp[i] = make([]float64, n)
		ch[i] = make([]int, n)
		for j := range dp[i] {
			dp[i][j] = -1
			ch[i][j] = -1
		}
	}
	for i := 0; i+1 < n; i++ {
		dp[i][i+1] = math.Inf(1)
	}
	for length := 2; length < n; length++ {
		for i := 0; i+length < n; i++ {
			j := i + length
			if !valid[i][j] {
				continue
			}
			best := -1.0
			bk := -1
			for k := i + 1; k < j; k++ {
				if dp[i][k] < 0 || dp[k][j] < 0 {
					continue
				}
				if !triOK(i, k, j) {
					continue
				}
				q := Triangle{p[i], p[k], p[j]}.Quality()
				v := math.Min(q, math.Min(dp[i][k], dp[k][j]))
				if v > best {
					best, bk = v, k
				}
			}
			if bk >= 0 {
				dp[i][j] = best
				ch[i][j] = bk
			}
		}
	}
	if dp[0][n-1] < 0 {
		return nil, errors.New("找不到合法三角化")
	}
	var tris []Triangle
	var rec func(i, j int)
	rec = func(i, j int) {
		if j <= i+1 {
			return
		}
		k := ch[i][j]
		if k < 0 {
			return
		}
		tris = append(tris, Triangle{p[i], p[k], p[j]})
		rec(i, k)
		rec(k, j)
	}
	rec(0, n-1)
	return tris, nil
}

func diagonalOK(p []Point, i, j int) bool {
	n := len(p)
	if j == i+1 || (i == 0 && j == n-1) {
		return true
	}
	a, b := p[i], p[j]
	for k := 0; k < n; k++ {
		k2 := (k + 1) % n
		if k == i || k == j || k2 == i || k2 == j {
			continue
		}
		if segmentsIntersect(a, b, p[k], p[k2]) {
			return false
		}
	}
	for k := 0; k < n; k++ {
		if k == i || k == j {
			continue
		}
		if pointOnSegment(p[k], a, b) {
			return false
		}
	}
	mid := Point{(a.X + b.X) / 2, (a.Y + b.Y) / 2}
	return Inside(p, mid.X, mid.Y)
}

// MeshMinAngle 返回网格中最小的三角形内角（弧度）。
func MeshMinAngle(tris []Triangle) float64 {
	best := math.Inf(1)
	for _, t := range tris {
		if a := t.MinAngle(); a < best {
			best = a
		}
	}
	return best
}

// MeshMinQuality 返回网格中最小的三角形面积比。
func MeshMinQuality(tris []Triangle) float64 {
	best := math.Inf(1)
	for _, t := range tris {
		if q := t.Quality(); q < best {
			best = q
		}
	}
	return best
}

// MeshArea 返回网格总面积。
func MeshArea(tris []Triangle) float64 {
	s := 0.0
	for _, t := range tris {
		s += t.Area()
	}
	return s
}
