package cliparea

import (
	"math"
	"math/rand"
	"testing"

	"vecgo/geom"
)

// onGridHex 是主实验多边形：完全在域内，且多个顶点压在网格裁剪线上。
var onGridHex = []geom.Point{
	{X: 0, Y: 0}, {X: 7, Y: 0}, {X: 10, Y: 4},
	{X: 6, Y: 10}, {X: 3, Y: 10}, {X: 0, Y: 6},
}

// TestExactClippingPreservesArea 证明“裁剪只保面积”：
// 在精确有理算术下，三档分块各块面积之和与原面积严格相等（差值恰为 0），
// 与分多少块、产生多少新交点无关。
func TestExactClippingPreservesArea(t *testing.T) {
	a0 := ExactArea(onGridHex)
	for _, k := range Levels {
		sum := ExactPieceSum(onGridHex, k, Domain)
		if sum.Cmp(a0) != 0 {
			t.Fatalf("k=%d 精确分块面积和 %s ≠ 原面积 %s", k, sum.RatString(), a0.RatString())
		}
	}
}

// TestClipLosesVertices 证明“裁剪不保顶点”：原 6 个顶点在分块后
// 被拆成更多顶点（交点替换了原顶点，顶点总数随档数变化且不等于 6）。
func TestClipLosesVertices(t *testing.T) {
	reports, err := Run(onGridHex)
	if err != nil {
		t.Fatal(err)
	}
	if reports[0].Vertices != len(onGridHex) {
		t.Fatalf("1×1 档不应产生交点，顶点数应保持 %d，实际 %d", len(onGridHex), reports[0].Vertices)
	}
	for _, r := range reports[1:] {
		if r.Vertices <= len(onGridHex) {
			t.Fatalf("k=%d 裁剪应引入交点使顶点总数超过原顶点数，实际 V=%d", r.K, r.Vertices)
		}
	}
}

// TestAreaDiffWithinBound 逐档判定：浮点面积差必须落在实测上界内，
// 同时也落在与顶点数成正比的闭式上界 C·V 内。
func TestAreaDiffWithinBound(t *testing.T) {
	reports, err := Run(onGridHex)
	if err != nil {
		t.Fatal(err)
	}
	coef := ComputeCoefficient(onGridHex)
	if coef.C <= 0 {
		t.Fatalf("上界系数必须为正: %v", coef)
	}
	for _, r := range reports {
		if !r.Within {
			t.Errorf("k=%d 面积差 %.3e 超出实测上界 %.3e", r.K, r.AreaDiff, r.Bound)
		}
		cv := coef.C * float64(r.Vertices)
		if r.AreaDiff > cv {
			t.Errorf("k=%d 面积差 %.3e 超出闭式上界 C·V=%.3e (C=%.3e, V=%d)",
				r.K, r.AreaDiff, cv, coef.C, r.Vertices)
		}
	}
}

// TestBoundScalesWithVertices 检验上界的线性性质：顶点越多的档，上界越大，
// 且每顶点的边际上界围绕同一系数（闭式 C 是其统一上限）。
func TestBoundScalesWithVertices(t *testing.T) {
	reports, _ := Run(onGridHex)
	coef := ComputeCoefficient(onGridHex)
	for _, r := range reports {
		if r.Bound > coef.C*float64(r.Vertices) {
			t.Errorf("k=%d 实测上界 %.3e 不应超过闭式 C·V=%.3e", r.K, r.Bound, coef.C*float64(r.Vertices))
		}
	}
	if reports[2].Bound <= reports[1].Bound || reports[1].Bound <= reports[0].Bound {
		t.Fatalf("上界应随顶点数单调增大: %v", reports)
	}
}

// TestAmbiguousOnLineCounted 顶点落在裁剪线上时，浮点叉积近零导致内/外
// 二义；本实验把这种情形单独计数。k≥2 时网格线 x=5 等必压过多边形顶点。
func TestAmbiguousOnLineCounted(t *testing.T) {
	reports, _ := Run(onGridHex)
	for _, r := range reports[1:] {
		if r.Ambiguous == 0 {
			t.Errorf("k=%d 应有顶点落在裁剪线上并被计入二义，实际为 0", r.K)
		}
	}
}

// TestTieConventionDoesNotMoveExactArea 二义的处置约定（统一按在内）
// 只影响浮点路径的拓扑；精确有理路径以叉积符号 c=0 为准，三档面积和
// 仍然严格等于原面积（与 TestExactClippingPreservesArea 互证）。
func TestTieConventionDoesNotMoveExactArea(t *testing.T) {
	a0 := ExactArea(onGridHex)
	for _, k := range Levels {
		if ExactPieceSum(onGridHex, k, Domain).Cmp(a0) != 0 {
			t.Fatalf("k=%d 二义处置影响了精确面积，不应发生", k)
		}
	}
}

// TestStressSimplePolygons 对抗性检查：构造 500 个量化到 1/16 的随机简单
// 多边形（含大量落线/近线交点），逐一验证精确守恒、实测紧界与闭式 C·V 界。
func TestStressSimplePolygons(t *testing.T) {
	rng := rand.New(rand.NewSource(20261126))
	for trial := 0; trial < 500; trial++ {
		n := 3 + rng.Intn(10)
		base := 2.5 + rng.Float64()*1.5
		poly := make([]geom.Point, n)
		for i := 0; i < n; i++ {
			ang := 2 * math.Pi * float64(i) / float64(n)
			r := quantize(base + rng.Float64()*1.2)
			poly[i] = geom.Point{
				X: quantize(5 + r*math.Cos(ang+0.3)),
				Y: quantize(5 + r*math.Sin(ang+0.3)),
			}
		}
		if !isSimple(poly) {
			t.Fatalf("试验 %d 生成器产生了自交多边形，前提被破坏", trial)
		}
		reports, err := Run(poly)
		if err != nil {
			t.Fatal(err)
		}
		coef := ComputeCoefficient(poly)
		for _, r := range reports {
			if r.ExactDiff != 0 {
				t.Errorf("试验 %d k=%d 精确路径不守恒: %.3e", trial, r.K, r.ExactDiff)
			}
			if !r.Within {
				t.Errorf("试验 %d k=%d 面积差 %.3e 超出实测上界 %.3e", trial, r.K, r.AreaDiff, r.Bound)
			}
			if r.AreaDiff > coef.C*float64(r.Vertices) {
				t.Errorf("试验 %d k=%d 面积差 %.3e 超出 C·V=%.3e", trial, r.K, r.AreaDiff, coef.C*float64(r.Vertices))
			}
		}
	}
}

func quantize(v float64) float64 { return float64(int64(v*16+0.5)) / 16 }

func edgeSide(a, b, p geom.Point) float64 {
	return (b.X-a.X)*(p.Y-a.Y) - (b.Y-a.Y)*(p.X-a.X)
}

// isSimple 判定多边形是否自交（SH 裁剪与鞋带面积只对简单多边形有定义）。
func isSimple(poly []geom.Point) bool {
	n := len(poly)
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			if (j+1)%n == i || (i+1)%n == j {
				continue // 相邻边共享端点，不算自交
			}
			a, b := poly[i], poly[(i+1)%n]
			c, d := poly[j], poly[(j+1)%n]
			if edgeSide(a, b, c)*edgeSide(a, b, d) < 0 &&
				edgeSide(c, d, a)*edgeSide(c, d, b) < 0 {
				return false
			}
		}
	}
	return true
}
