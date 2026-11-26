package geom

import (
	"math"
	"testing"
)

func deg(x float64) float64 { return x * 180 / math.Pi }

func TestInteriorAnglesRegular(t *testing.T) {
	poly := EllipsePolygon(1, 1, 8) // 正八边形
	angles, err := InteriorAngles(poly)
	if err != nil {
		t.Fatal(err)
	}
	for i, a := range angles {
		if math.Abs(deg(a)-135) > 1e-9 {
			t.Fatalf("顶点 %d 内角 %.6f 应为 135", i, deg(a))
		}
	}
}

func TestInteriorAnglesConcave(t *testing.T) {
	// L 形，顶点 3 是凹角 270°
	poly := []Point{{0, 0}, {4, 0}, {4, 2}, {2, 2}, {2, 4}, {0, 4}}
	minA, _, err := MinInteriorAngle(poly)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(deg(minA)-90) > 1e-9 {
		t.Fatalf("最小角应为 90°，得到 %.3f", deg(minA))
	}
	angles, _ := InteriorAngles(poly)
	if math.Abs(deg(angles[3])-270) > 1e-9 {
		t.Fatalf("凹角应为 270°，得到 %.3f", deg(angles[3]))
	}
}

func TestQualityCeiling(t *testing.T) {
	if q := QualityCeiling(math.Pi / 3); math.Abs(q-1) > 1e-12 {
		t.Fatalf("等边三角形质量应为 1，得到 %v", q)
	}
	if QualityCeiling(0) != 0 {
		t.Fatal("θ=0 应返回 0")
	}
	var prev float64
	for th := 0.01; th <= math.Pi/3; th += 0.01 {
		q := QualityCeiling(th)
		if q < prev-1e-12 || q > 1+1e-12 {
			t.Fatalf("q(θ) 应在 [0,1] 单调递增，θ=%.3f q=%.6f", th, q)
		}
		prev = q
	}
}

// 强上界 θ(𝒯) ≤ U(α)=min(α,(π−α)/2) 必须对改进后的三角化成立。
func TestOptimalRespectsBound(t *testing.T) {
	cases := map[string][]Point{
		"ellipse":  EllipsePolygon(2.0, 0.30, 14),
		"star":     StarPolygon(1.0, 0.45),
		"corridor": CorridorPolygon(1.7, 0.45, 14, 36),
		"L":        {{0, 0}, {4, 0}, {4, 2}, {2, 2}, {2, 4}, {0, 4}},
		"square":   {{0, 0}, {4, 0}, {4, 4}, {0, 4}},
	}
	for name, poly := range cases {
		alpha, _, err := MinInteriorAngle(poly)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		bound := AngleUpperBound(alpha)
		tris, err := OptimalTriangulation(poly)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		got := MeshMinAngle(tris)
		if got > bound+1e-9 {
			t.Fatalf("%s: 最小角 %.4f° 超过上界 %.4f°", name, deg(got), deg(bound))
		}
	}
}

// 已知多边形的上界数值，锁定系数 1 与 1/2 的两条分支。
func TestAngleUpperBoundValues(t *testing.T) {
	cases := []struct {
		name   string
		poly   []Point
		expect float64 // 度
	}{
		{"正三角形", EllipsePolygon(1, 1, 3), 60},
		{"正方形", []Point{{0, 0}, {1, 0}, {1, 1}, {0, 1}}, 45},
		{"正五边形", EllipsePolygon(1, 1, 5), 36},
		{"正六边形", EllipsePolygon(1, 1, 6), 30},
	}
	for _, tc := range cases {
		alpha, _, err := MinInteriorAngle(tc.poly)
		if err != nil {
			t.Fatal(err)
		}
		if got := deg(AngleUpperBound(alpha)); math.Abs(got-tc.expect) > 1e-6 {
			t.Fatalf("%s: U=%.6f°, 期望 %.1f°", tc.name, got, tc.expect)
		}
	}
}

// 含 5° 尖顶角的三角形：上界就是 5°，对应面积比天花板约 0.15，
// 定量说明小内角多边形不可能完全摆脱细长三角形。
func TestSharpCornerForcesSliver(t *testing.T) {
	const alpha = 5 * math.Pi / 180
	L := 10.0
	poly := []Point{{0, 0}, {L, 0}, {L * math.Cos(alpha), L * math.Sin(alpha)}}
	gotAlpha, _, err := MinInteriorAngle(poly)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(gotAlpha-alpha) > 1e-9 {
		t.Fatalf("构造尖顶角失败: %.4f°", deg(gotAlpha))
	}
	if b := AngleUpperBound(gotAlpha); math.Abs(deg(b)-5) > 1e-9 {
		t.Fatalf("尖顶角分支上界应为 5°, 得到 %.4f°", deg(b))
	}
	if q := QualityCeiling(AngleUpperBound(gotAlpha)); q > 0.16 || q < 0.14 {
		t.Fatalf("5° 对应的面积比包络应约 0.15, 得到 %.4f", q)
	}
	tris, err := OptimalTriangulation(poly)
	if err != nil {
		t.Fatal(err)
	}
	if MeshMinAngle(tris) > AngleUpperBound(gotAlpha)+1e-9 {
		t.Fatal("唯一三角化也必须满足上界")
	}
}

// 长高比悬殊的矩形：所有内角都是 90°，角度上界 U=45° 很松，
// 但仅用四个角点的任意三角化（n=4 只有两种、且全等）都被迫含
// 最小角 arctan(w/L)、面积比 √3·Lw/(L²+w²) 的细长直角三角形。
// 这说明即使没有小内角，长宽比仍强制细长。
func TestLongRectangleForcesSliver(t *testing.T) {
	const L, w = 100.0, 1.0
	rect := []Point{{0, 0}, {L, 0}, {L, w}, {0, w}}
	alpha, _, err := MinInteriorAngle(rect)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(deg(alpha)-90) > 1e-9 {
		t.Fatalf("矩形最小内角应为 90°, 得到 %.4f°", deg(alpha))
	}
	if math.Abs(deg(AngleUpperBound(alpha))-45) > 1e-9 {
		t.Fatalf("矩形的角度上界应为 45°, 得到 %.4f°", deg(AngleUpperBound(alpha)))
	}
	wantAng := math.Atan(w / L)
	wantQ := math.Sqrt(3) * L * w / (L*L + w*w)
	for name, fn := range map[string]func([]Point) ([]Triangle, error){
		"ear": EarClip,
		"dp":  OptimalTriangulation,
	} {
		ts, err := fn(rect)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := MeshMinAngle(ts); math.Abs(got-wantAng) > 1e-9 {
			t.Fatalf("%s: 最小角 %.6f° 应为 arctan(w/L)=%.6f°", name, deg(got), deg(wantAng))
		}
		if got := MeshMinQuality(ts); math.Abs(got-wantQ) > 1e-12 {
			t.Fatalf("%s: 最差面积比 %.9f 应为 %.9f", name, got, wantQ)
		}
	}
}

// 穷举小规模凸多边形的全部 Catalan 种三角化：
//  1. 每一种都满足强上界 U(α)；
//  2. 区间 DP 给出的最差面积比确实等于所有三角化里的最大值。
func TestBoundAndDPAgainstExhaustive(t *testing.T) {
	for seed := int64(1); seed <= 40; seed++ {
		rnd := newLCG(seed)
		n := 4 + int(rnd.Uint64()%6) // 4..9
		// 单位圆上按排序角度取点 => 严格凸；随机角度间隔制造悬殊内角。
		angles := make([]float64, n)
		for i := range angles {
			angles[i] = 2 * math.Pi * (float64(i) + 0.15*rnd.Float64()) / float64(n)
		}
		// 加入随机扰动后重新排序，保证间隔不均。
		for i := range angles {
			angles[i] += (rnd.Float64() - 0.5) * 0.5
		}
		sortFloat64s(angles)
		poly := make([]Point, n)
		for i, a := range angles {
			poly[i] = Point{math.Cos(a), math.Sin(a)}
		}
		if SignedArea(poly) < 0 {
			reversePoints(poly)
		}
		alpha, _, err := MinInteriorAngle(poly)
		if err != nil {
			t.Fatal(err)
		}
		ub := AngleUpperBound(alpha)

		all := exhaustiveTriangulations(poly)
		if len(all) != catalan(n-2) {
			t.Fatalf("seed=%d n=%d 三角化数 %d != Catalan %d", seed, n, len(all), catalan(n-2))
		}
		bestQ := -1.0
		for _, ts := range all {
			if a := MeshMinAngle(ts); a > ub+1e-7 {
				t.Fatalf("seed=%d n=%d 存在三角化 minAngle=%.4f° 越过上界 %.4f°",
					seed, n, deg(a), deg(ub))
			}
			if q := MeshMinQuality(ts); q > bestQ {
				bestQ = q
			}
		}
		dp, err := OptimalTriangulation(poly)
		if err != nil {
			t.Fatalf("seed=%d: %v", seed, err)
		}
		if dq := MeshMinQuality(dp); math.Abs(dq-bestQ) > 1e-9 {
			t.Fatalf("seed=%d n=%d DP 最差面积比 %.9f != 穷举最优 %.9f", seed, n, dq, bestQ)
		}
	}
}

// exhaustiveTriangulations 用 (0,n-1) 根边的标准 Catalan 递归枚举凸多边形全部三角化。
func exhaustiveTriangulations(poly []Point) [][]Triangle {
	n := len(poly)
	var rec func(i, j int) [][]Triangle
	rec = func(i, j int) [][]Triangle {
		if j == i+1 {
			return [][]Triangle{{}}
		}
		var out [][]Triangle
		for k := i + 1; k < j; k++ {
			for _, L := range rec(i, k) {
				for _, R := range rec(k, j) {
					ts := make([]Triangle, 0, len(L)+len(R)+1)
					ts = append(ts, L...)
					ts = append(ts, R...)
					ts = append(ts, Triangle{poly[i], poly[k], poly[j]})
					out = append(out, ts)
				}
			}
		}
		return out
	}
	return rec(0, n-1)
}

func catalan(m int) int {
	c := 1
	for i := 0; i < m; i++ {
		c = c * 2 * (2*i + 1) / (i + 2)
	}
	return c
}

func reversePoints(p []Point) {
	for i, j := 0, len(p)-1; i < j; i, j = i+1, j-1 {
		p[i], p[j] = p[j], p[i]
	}
}

func sortFloat64s(a []float64) {
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && a[j-1] > a[j]; j-- {
			a[j-1], a[j] = a[j], a[j-1]
		}
	}
}

// 确定性的最小随机源，避免测试依赖 math/rand 的全局状态与种子API差异。
type lcg struct{ x uint64 }

func newLCG(seed int64) *lcg { return &lcg{x: uint64(seed)*6364136223846793005 + 1442695040888963407} }
func (g *lcg) Uint64() uint64 {
	g.x = g.x*6364136223846793005 + 1442695040888963407
	return g.x
}
func (g *lcg) Float64() float64 { return float64(g.Uint64()>>11) / float64(1<<53) }

func TestTriangulationsCoverPolygon(t *testing.T) {
	cases := [][]Point{
		EllipsePolygon(2.0, 0.30, 14),
		StarPolygon(1.0, 0.45),
		CorridorPolygon(1.7, 0.45, 14, 36),
	}
	for _, poly := range cases {
		want := math.Abs(SignedArea(poly))
		ear, err := EarClip(poly)
		if err != nil {
			t.Fatal(err)
		}
		opt, err := OptimalTriangulation(poly)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range [][]Triangle{ear, opt} {
			if d := math.Abs(MeshArea(m) - want); d > 1e-9*math.Max(1, want) {
				t.Fatalf("面积不守恒: %.3e", d)
			}
			if len(m) != len(poly)-2 {
				t.Fatalf("三角形数应为 n-2=%d，得到 %d", len(poly)-2, len(m))
			}
		}
	}
}

func TestOptimalBeatsEarClip(t *testing.T) {
	cases := map[string][]Point{
		"ellipse":  EllipsePolygon(2.0, 0.30, 14),
		"star":     StarPolygon(1.0, 0.45),
		"corridor": CorridorPolygon(1.7, 0.45, 14, 36),
	}
	for name, poly := range cases {
		ear, err := EarClip(poly)
		if err != nil {
			t.Fatal(err)
		}
		opt, err := OptimalTriangulation(poly)
		if err != nil {
			t.Fatal(err)
		}
		if MeshMinQuality(opt) < MeshMinQuality(ear)-1e-12 {
			t.Fatalf("%s: 最优三角化的最差面积比不应低于耳切: %.6f < %.6f",
				name, MeshMinQuality(opt), MeshMinQuality(ear))
		}
	}
}

func TestCoverageErrorImproves(t *testing.T) {
	poly := CorridorPolygon(1.7, 0.45, 14, 36)
	ear, _ := EarClip(poly)
	opt, _ := OptimalTriangulation(poly)
	ce, err := CoverageError(ear, SubpixelLevels)
	if err != nil {
		t.Fatal(err)
	}
	co, err := CoverageError(opt, SubpixelLevels)
	if err != nil {
		t.Fatal(err)
	}
	if co >= ce {
		t.Fatalf("覆盖率误差应随面积比改善而下降: %.6f → %.6f", ce, co)
	}
}

func TestQualErrScalesWithElongation(t *testing.T) {
	// 同一个顶点集，细长三角形量化误差应大于等边三角形
	slender := Triangle{{0, 0}, {10, 0}, {10, 0.05}}
	equi := Triangle{{0, 0}, {10, 0}, {5, 5 * math.Sqrt(3)}}
	if slender.QuantErr(SubpixelLevels) <= equi.QuantErr(SubpixelLevels) {
		t.Fatalf("细长三角形的覆盖率量化误差应更大: %.6f vs %.6f",
			slender.QuantErr(SubpixelLevels), equi.QuantErr(SubpixelLevels))
	}
}

func TestEarClipRejectsDegenerate(t *testing.T) {
	if _, err := EarClip([]Point{{0, 0}, {1, 1}, {2, 2}}); err == nil {
		t.Fatal("共线三点应报错")
	}
	if _, err := EarClip([]Point{{0, 0}, {1, 1}}); err == nil {
		t.Fatal("少于三个顶点应报错")
	}
}
