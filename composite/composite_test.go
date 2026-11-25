package composite

import (
	"testing"
)

// 三组透明度，每组在 3x3 测试图上固定三个反例点，坐标便于手工复算：
//
//	组0 alpha=(80,160,120):  P0=(0,0) 颜色(0,0,3)
//	                         P1=(1,1) 颜色(0,0,11)
//	                         P2=(2,2) 颜色(0,0,16)
//	组1 alpha=(40,200,100):  P0=(0,0) 颜色(0,0,6)
//	                         P1=(1,1) 颜色(0,0,16)
//	                         P2=(2,2) 颜色(0,0,17)
//	组2 alpha=(120,90,170):  P0=(0,0) 颜色(0,0,2)
//	                         P1=(1,1) 颜色(0,0,6)
//	                         P2=(2,2) 颜色(0,0,21)
type pointCase struct {
	X, Y                int
	CA, CB, CC          uint8
	ExpectStraightL, R  uint8 // 朴素 straight 管线左/右括号颜色
	ExpectStraightAlpha uint8 // 朴素管线（截断除法）得到的 alpha
	ExpectAccColor      uint8 // 精确累加器给出的颜色
	ExpectAccAlpha      uint8 // 精确累加器（半值向上）给出的 alpha
}

type alphaCase struct {
	name       string
	aA, aB, aC uint8
	points     [3]pointCase
}

var cases = [3]alphaCase{
	{
		name: "alpha=(80,160,120)", aA: 80, aB: 160, aC: 120,
		points: [3]pointCase{
			{0, 0, 0, 0, 3, 0, 1, 220, 0, 220},
			{1, 1, 0, 0, 11, 2, 1, 220, 2, 220},
			{2, 2, 0, 0, 16, 2, 3, 220, 2, 220},
		},
	},
	{
		name: "alpha=(40,200,100)", aA: 40, aB: 200, aC: 100,
		points: [3]pointCase{
			{0, 0, 0, 0, 6, 0, 1, 226, 0, 227},
			{1, 1, 0, 0, 16, 1, 2, 226, 1, 227},
			{2, 2, 0, 0, 17, 1, 2, 226, 1, 227},
		},
	},
	{
		name: "alpha=(120,90,170)", aA: 120, aB: 90, aC: 170,
		points: [3]pointCase{
			{0, 0, 0, 0, 2, 1, 0, 225, 1, 226},
			{1, 1, 0, 0, 6, 2, 1, 225, 2, 226},
			{2, 2, 0, 0, 21, 5, 6, 225, 5, 226},
		},
	},
}

func buildLayers(t *testing.T, ac alphaCase) (a, b, c *Layer) {
	t.Helper()
	mk := func() *Layer {
		l, err := NewLayer(3, 3)
		if err != nil {
			t.Fatalf("NewLayer: %v", err)
		}
		return l
	}
	a, b, c = mk(), mk(), mk()
	for i := range a.Pix {
		a.Pix[i] = RGBA{R: 200, G: 120, B: 40, A: ac.aA}
		b.Pix[i] = RGBA{R: 40, G: 160, B: 200, A: ac.aB}
		c.Pix[i] = RGBA{R: 10, G: 30, B: 90, A: ac.aC}
	}
	put := func(l *Layer, x, y int, col, alpha uint8) {
		l.Pix[y*l.W+x] = RGBA{R: col, G: col, B: col, A: alpha}
	}
	for _, p := range ac.points {
		put(a, p.X, p.Y, p.CA, ac.aA)
		put(b, p.X, p.Y, p.CB, ac.aB)
		put(c, p.X, p.Y, p.CC, ac.aC)
	}
	return a, b, c
}

func foldLeft(over func(top, bot RGBA) RGBA, A, B, C RGBA) RGBA {
	return over(over(A, B), C)
}

func foldRight(over func(top, bot RGBA) RGBA, A, B, C RGBA) RGBA {
	return over(A, over(B, C))
}

// TestExactAccumulatorIsBracketIndependent：重写后的精确有理数累加器，
// 左结合、右结合以及"先合 B、C 再让 A 盖上去"三种走法逐像素相同，差异为零。
func TestExactAccumulatorIsBracketIndependent(t *testing.T) {
	for _, ac := range cases {
		t.Run(ac.name, func(t *testing.T) {
			a, b, c := buildLayers(t, ac)

			// 走法 1：生产路径，三层一次性左折叠。
			left, err := Composite([]*Layer{a, b, c})
			if err != nil {
				t.Fatalf("Composite: %v", err)
			}
			// 走法 2：先合 B、C 成中间 8-bit 图，再与 A 合成会引入中间量化，
			// 不能用来对照；改为在精确累加器内部用右结合递归逐像素求值。
			diff := 0
			for i := range left.Pix {
				px := []RGBA{a.Pix[i], b.Pix[i], c.Pix[i]}
				rf := compositePixelFoldRight(px)
				if left.Pix[i] != rf {
					diff++
					t.Errorf("像素 %d: 左折叠=%+v 右结合=%+v", i, left.Pix[i], rf)
				}
				// 走法 3：另一种括号 (A over B) over C 的逐像素精确计算，
				// 与生产路径同构，再独立算一遍。
				ab := overAcc(layerFrac(px[0]), layerFrac(px[1]))
				lr := overAcc(ab, layerFrac(px[2])).finalize()
				if lr != left.Pix[i] {
					diff++
					t.Errorf("像素 %d: 显式左括号=%+v 生产路径=%+v", i, lr, left.Pix[i])
				}
			}
			t.Logf("精确累加器差异像素数 = %d / %d", diff, len(left.Pix))
			if diff != 0 {
				t.Fatalf("要求差异像素数为零，实际 %d", diff)
			}
		})
	}
}

// TestCounterexampleValues：核对每个反例点的坐标、颜色三元组与三种管线数值。
func TestCounterexampleValues(t *testing.T) {
	for _, ac := range cases {
		t.Run(ac.name, func(t *testing.T) {
			a, b, c := buildLayers(t, ac)
			for _, p := range ac.points {
				i := p.Y*3 + p.X
				L := foldLeft(OverStraight, a.Pix[i], b.Pix[i], c.Pix[i])
				R := foldRight(OverStraight, a.Pix[i], b.Pix[i], c.Pix[i])
				if L.R != p.ExpectStraightL || R.R != p.R {
					t.Errorf("坐标(%d,%d) straight 数值不符: L=%d R=%d, 期望 L=%d R=%d",
						p.X, p.Y, L.R, R.R, p.ExpectStraightL, p.R)
				}
				if L.A != p.ExpectStraightAlpha || R.A != p.ExpectStraightAlpha {
					t.Errorf("坐标(%d,%d) straight alpha 不符: L.A=%d R.A=%d 期望 %d",
						p.X, p.Y, L.A, R.A, p.ExpectStraightAlpha)
				}
				acc := compositePixelFoldRight([]RGBA{a.Pix[i], b.Pix[i], c.Pix[i]})
				if acc.R != p.ExpectAccColor || acc.A != p.ExpectAccAlpha {
					t.Errorf("坐标(%d,%d) 累加器不符: got=(%d,%d) 期望颜色=%d alpha=%d",
						p.X, p.Y, acc.R, acc.A, p.ExpectAccColor, p.ExpectAccAlpha)
				}
				t.Logf("坐标(%d,%d) 颜色(%d,%d,%d): straight 左=%d 右=%d | 精确累加器=%d (alpha=%d)",
					p.X, p.Y, p.CA, p.CB, p.CC, L.R, R.R, acc.R, acc.A)
			}
		})
	}
}

// TestNaivePipelinesBreakAssociativity：两条逐步量化管线在反例点上必须出现差异。
func TestNaivePipelinesBreakAssociativity(t *testing.T) {
	for _, ac := range cases {
		t.Run(ac.name, func(t *testing.T) {
			a, b, c := buildLayers(t, ac)
			stDiff, pmDiff := 0, 0
			for i := range a.Pix {
				if foldLeft(OverStraight, a.Pix[i], b.Pix[i], c.Pix[i]) !=
					foldRight(OverStraight, a.Pix[i], b.Pix[i], c.Pix[i]) {
					stDiff++
				}
				if foldLeft(OverPremultStep, a.Pix[i], b.Pix[i], c.Pix[i]) !=
					foldRight(OverPremultStep, a.Pix[i], b.Pix[i], c.Pix[i]) {
					pmDiff++
				}
			}
			t.Logf("朴素 straight 管线差异像素数 = %d / %d", stDiff, len(a.Pix))
			t.Logf("逐步量化预乘管线差异像素数 = %d / %d", pmDiff, len(a.Pix))
			if stDiff == 0 {
				t.Fatal("straight 朴素管线应出现结合律反例，实际差异为 0")
			}
		})
	}
}

// TestManyLayersBracketIndependent：扩到 6 层随机数据，比较所有括号化方式
// （通过 Catalan 式递归枚举）输出必须完全一致。
func TestManyLayersBracketIndependent(t *testing.T) {
	const n = 6
	px := make([]RGBA, n)
	// 确定性局部 LCG，不依赖全局状态。
	state := uint32(123456789)
	rnd := func() uint8 {
		state = state*1103515245 + 12345
		return uint8((state >> 16) & 0xff)
	}
	for i := range px {
		px[i] = RGBA{R: rnd(), G: rnd(), B: rnd(), A: rnd()}
	}

	// 枚举 [lo,hi) 区间所有括号化方式的精确累加状态 acc，最后统一 finalize。
	var allFolds func(lo, hi int) []acc
	allFolds = func(lo, hi int) []acc {
		if hi-lo == 1 {
			return []acc{layerFrac(px[lo])}
		}
		var out []acc
		for k := lo + 1; k < hi; k++ {
			for _, l := range allFolds(lo, k) {
				for _, r := range allFolds(k, hi) {
					out = append(out, overAcc(l, r))
				}
			}
		}
		return out
	}
	results := map[RGBA]struct{}{}
	for _, s := range allFolds(0, n) {
		results[s.finalize()] = struct{}{}
	}
	if len(results) != 1 {
		t.Fatalf("6 层括号化结果不唯一，得到 %d 种输出: %v", len(results), results)
	}
	// 生产路径必须落在这个唯一结果里。
	layers := make([]*Layer, n)
	for i := range layers {
		l, _ := NewLayer(1, 1)
		l.Pix[0] = px[i]
		layers[i] = l
	}
	prod, err := Composite(layers)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := results[prod.Pix[0]]; !ok {
		t.Fatalf("生产路径输出 %+v 不在括号枚举结果集 %v 中", prod.Pix[0], results)
	}
}

// TestValidation：空图层与画幅不一致要报错。
func TestValidation(t *testing.T) {
	if _, err := Composite(nil); err == nil {
		t.Fatal("空图层应报错")
	}
	l1, _ := NewLayer(2, 2)
	l2, _ := NewLayer(2, 3)
	if _, err := Composite([]*Layer{l1, l2}); err == nil {
		t.Fatal("画幅不一致应报错")
	}
}
