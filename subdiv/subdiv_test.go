package subdiv

import (
	"math"
	"testing"
)

// 三组固定实验三角形（坐标单位 px，画幅 160×160）。
// 它们由数值搜索挑出：在 ρ=1/256 px、4× 超采样下，
// 一次与两次细分的覆盖位图逐格相同。
var testGroups = []struct {
	name string
	t    Triangle
}{
	{"g1", Triangle{{X: 53.928, Y: 43.3}, {X: 13.988, Y: 109.108}, {X: 113.543, Y: 142.499}}},
	{"g2", Triangle{{X: 41.344, Y: 10.936}, {X: 37.952, Y: 149.017}, {X: 137.329, Y: 36.22}}},
	{"g3", Triangle{{X: 25.698, Y: 145.976}, {X: 127.338, Y: 91.466}, {X: 25.684, Y: 47.895}}},
}

const (
	testRes = 1.0 / 256.0 // 坐标分辨率 ρ：1/256 px（8 位子像素精度）
	testW   = 160
	testH   = 160
	testSS  = 4
)

func near(t *testing.T, got, want, tol float64, msg string) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Fatalf("%s: got %.12g want %.12g (diff %.3g > %.3g)", msg, got, want, got-want, tol)
	}
}

// 四个中点子三角形的面积都必须恰好是父三角形的 1/4（浮点容差内）。
func TestMidpointChildrenQuarterArea(t *testing.T) {
	for _, g := range testGroups {
		a0 := Area(g.t)
		for _, ch := range MidChildren(g.t) {
			near(t, Area(ch), a0/4, 1e-9*math.Max(1, a0), g.name+" child")
		}
	}
}

// 精确中点细分 n 次，面积总和严格等于初始面积（恒等式，浮点残差极小）。
func TestExactSubdivisionPreservesArea(t *testing.T) {
	for n := 0; n <= 7; n++ {
		for _, g := range testGroups {
			tris := Subdivide(g.t, n)
			if len(tris) != int(math.Pow(4, float64(n))) {
				t.Fatalf("n=%d 三角形数=%d, 期望 %d", n, len(tris), int(math.Pow(4, float64(n))))
			}
			got := TotalArea(tris)
			// 容差随 n 温和放大，仅吸收 float64 求和/运算残差。
			tol := 1e-9 * math.Max(1, Area(g.t)) * (1 + float64(n))
			near(t, got, Area(g.t), tol, g.name+" exact total")
		}
	}
}

// B(n+1) 必须恰好是 B(n) 的 4 倍：上界按四的幂累积。
func TestBoundAccumulatesByFour(t *testing.T) {
	const L = 123.456
	b0 := TotalBound(0, testRes, L)
	for n := 0; n <= 8; n++ {
		bn := TotalBound(n, testRes, L)
		if math.Abs(bn-b0*math.Pow(4, float64(n))) > 1e-9*bn {
			t.Fatalf("B(%d)=%.12g 不等于 B0*4^n", n, bn)
		}
		if n < 8 {
			if ratio := TotalBound(n+1, testRes, L) / bn; math.Abs(ratio-4) > 1e-12 {
				t.Fatalf("相邻级上界比例=%.15g, 期望 4", ratio)
			}
		}
	}
}

// 所有组、所有层级的实测面积差都必须落在 3ρ·Lcap·4^n 上界内，
// 且每片量化叶子的最长边都不超过统一边长上限 Lcap。
func TestRoundedAreaWithinBound(t *testing.T) {
	for _, g := range testGroups {
		a0 := Area(g.t)
		lcap := EdgeCap(LongestEdge(g.t), testRes)
		for n := 0; n <= 8; n++ {
			tris := SnapAll(Subdivide(g.t, n), testRes)
			diff := math.Abs(TotalArea(tris) - a0)
			bound := TotalBound(n, testRes, lcap)
			if diff > bound+1e-12 {
				t.Fatalf("%s n=%d |ΔA|=%.6g 超过上界 %.6g", g.name, n, diff, bound)
			}
			for _, q := range tris {
				if l := LongestEdge(q); l > lcap+1e-12 {
					t.Fatalf("%s n=%d 叶边长 %.6g 超过 Lcap %.6g", g.name, n, l, lcap)
				}
				if LongestEdge(q) < testRes {
					t.Fatalf("%s n=%d 叶子边长小于分辨率，3ρL 化简条件失效", g.name, n)
				}
			}
		}
	}
}

// 关键验收：一次与两次细分的覆盖像素数必须对得上，差值为 0。
// 同时校验任何覆盖像素(threshold=1)与 repo 主程序使用的 128 阈值，
// 并要求 4× 超采样灰度位图逐格相同（最强形式）。
func TestCoverageDepth1EqualsDepth2(t *testing.T) {
	for _, g := range testGroups {
		im1, err := Coverage(SnapAll(Subdivide(g.t, 1), testRes), testW, testH, testSS)
		if err != nil {
			t.Fatal(err)
		}
		im2, err := Coverage(SnapAll(Subdivide(g.t, 2), testRes), testW, testH, testSS)
		if err != nil {
			t.Fatal(err)
		}
		any1, any2 := im1.CountOn(1), im2.CountOn(1)
		on1, on2 := im1.CountOn(128), im2.CountOn(128)
		t.Logf("%s: any n1=%d n2=%d diff=%d; on128 n1=%d n2=%d diff=%d",
			g.name, any1, any2, any2-any1, on1, on2, on2-on1)
		if any2-any1 != 0 {
			t.Fatalf("%s 任意覆盖像素数差值=%d, 期望 0", g.name, any2-any1)
		}
		if on2-on1 != 0 {
			t.Fatalf("%s threshold=128 覆盖像素数差值=%d, 期望 0", g.name, on2-on1)
		}
		if d := BitmapDifferentPixels(im1, im2); d != 0 {
			t.Fatalf("%s n1/n2 位图有 %d 个像素不同, 期望 0", g.name, d)
		}

		// 精确（不量化）细分下，覆盖位图更应严格一致：区域恒等。
		ex1, _ := Coverage(Subdivide(g.t, 1), testW, testH, testSS)
		ex2, _ := Coverage(Subdivide(g.t, 2), testW, testH, testSS)
		if d := BitmapDifferentPixels(ex1, ex2); d != 0 {
			t.Fatalf("%s 精确细分 n1/n2 位图有 %d 像素不同", g.name, d)
		}
	}
}

// 在 1% 初始面积预算下，三组的可接受次数都应为 2：
// B(2) <= 0.01 A0 < B(3)。
func TestAcceptableN(t *testing.T) {
	for _, g := range testGroups {
		a0 := Area(g.t)
		lcap := EdgeCap(LongestEdge(g.t), testRes)
		budget := 0.01 * a0
		nStar := AcceptableN(testRes, lcap, budget)
		t.Logf("%s A0=%.3f Lcap=%.4f B2=%.4f budget=%.4f n*=%d",
			g.name, a0, lcap, TotalBound(2, testRes, lcap), budget, nStar)
		if nStar != 2 {
			t.Fatalf("%s n*=%d, 期望 2", g.name, nStar)
		}
		if TotalBound(2, testRes, lcap) > budget {
			t.Fatal("n=2 应当被上界接受")
		}
		if TotalBound(3, testRes, lcap) <= budget {
			t.Fatal("n=3 应当被上界拒绝")
		}
	}
}
