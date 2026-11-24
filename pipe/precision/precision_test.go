package precision

import (
	"math"
	"math/rand"
	"testing"

	"vecgo/geom"
)

const testE = 0.01

func TestConditions(t *testing.T) {
	// 谱条件数：缩放各向同性、旋转正交、平移单位阵，三者 κ₂ 恒为 1。
	// 无穷范数增益：旋转步随角度变化，范围 [1,√2]，其余为常数。
	for _, deg := range []float64{0, 15, 30, 45, 60, 90, 135, 270} {
		th := deg * math.Pi / 180
		st := Conditions(2, th, 32, 32)
		for i, s := range st {
			if math.Abs(s.Cond2-1) > 1e-12 {
				t.Fatalf("deg=%g stage=%d κ₂=%v, 期望 1", deg, i, s.Cond2)
			}
		}
		want := math.Abs(math.Cos(th)) + math.Abs(math.Sin(th))
		if math.Abs(st[StageRotate].InfGain-want) > 1e-12 {
			t.Fatalf("旋转 ‖R‖∞=%v, 期望 %v", st[StageRotate].InfGain, want)
		}
		if want < 1-1e-12 || want > math.Sqrt2+1e-12 {
			t.Fatalf("‖R‖∞=%v 越出 [1,√2]", want)
		}
		// 放大倍数链：a1=‖R‖∞, a2=a3=1
		if math.Abs(st[StageScale].AmplifyTo-want) > 1e-12 ||
			st[StageRotate].AmplifyTo != 1 || st[StageTranslate].AmplifyTo != 1 {
			t.Fatalf("放大倍数链错误: %+v", st)
		}
	}
}

func TestAdaptiveBoundNeverExceeded(t *testing.T) {
	// 对全部角度与多组参数，证明性检查：取整后保证上界 Σ a_i·0.5·10^-d_i ≤ E。
	rng := rand.New(rand.NewSource(1))
	for trial := 0; trial < 2000; trial++ {
		k := 0.1 + rng.Float64()*10
		th := rng.Float64() * 2 * math.Pi
		E := math.Pow(10, -1-rng.Float64()*5) // 1e-1 .. 1e-6
		p := PlanAdaptive(k, th, 32, 32, E)
		if b := p.Bound(); b > E*(1+1e-9) {
			t.Fatalf("k=%v th=%v E=%v 保证上界 %v 超界", k, th, E, b)
		}
	}
}

func TestEmpiricalErrors(t *testing.T) {
	// 实测：对一批随机点跑量化流水线，分量误差同时不超过保证上界与 E。
	rng := rand.New(rand.NewSource(7))
	k, th, tx, ty := 2.0, math.Pi/4, 32.0, 32.0
	pa := PlanAdaptive(k, th, tx, ty, testE)
	d := pa.DigitsOf()
	var worst float64
	for i := 0; i < 200000; i++ {
		p := geom.Point{X: rng.Float64() * 100, Y: rng.Float64() * 100}
		got := Apply(p, k, th, tx, ty, d)
		ref := Exact(p, k, th, tx, ty)
		w := math.Max(math.Abs(got.X-ref.X), math.Abs(got.Y-ref.Y))
		if w > worst {
			worst = w
		}
		if w > pa.Bound()*(1+1e-12) {
			t.Fatalf("实测误差 %v 超过保证上界 %v", w, pa.Bound())
		}
	}
	t.Logf("自适应方案 实测最坏误差=%v 保证上界=%v E=%v", worst, pa.Bound(), testE)
	if worst >= testE {
		t.Fatalf("实测误差 %v 不应达到 E", worst)
	}
}

func TestAdaptiveNoMoreDigitsThanEqual(t *testing.T) {
	// 同一 E（两方案都保证 ≤E）下，自适应总位数 ≤ 等精度；
	// 选取能跨越整数位边界的 E，验证在 ‖R‖∞>1 时严格更少。
	rng := rand.New(rand.NewSource(3))
	for trial := 0; trial < 500; trial++ {
		th := rng.Float64() * 2 * math.Pi
		E := math.Pow(10, -1-rng.Float64()*5)
		pa := PlanAdaptive(2, th, 32, 32, E)
		pe := PlanEqual(2, th, 32, 32, E)
		if pa.TotalDigits() > pe.TotalDigits() {
			t.Fatalf("th=%v E=%v 自适应总位数 %d > 等精度 %d", th, E, pa.TotalDigits(), pe.TotalDigits())
		}
		if pe.Bound() > E*(1+1e-9) || pa.Bound() > E*(1+1e-9) {
			t.Fatal("两方案都不应超界")
		}
	}
	// 定点验证严格省位：E=1e-4、45°（‖R‖∞=√2≈1.414）
	th := math.Pi / 4
	const E4 = 1e-4
	pa := PlanAdaptive(2, th, 32, 32, E4)
	pe := PlanEqual(2, th, 32, 32, E4)
	if pa.TotalDigits() >= pe.TotalDigits() {
		t.Fatalf("E=1e-4/45° 自适应 %d 位未严格少于等精度 %d 位", pa.TotalDigits(), pe.TotalDigits())
	}
	t.Logf("E=1e-4/45°: 自适应位数=%v 共%d (bound=%v); 等精度位数=%d 共%d (bound=%v)",
		pa.DigitsOf(), pa.TotalDigits(), pa.Bound(), pe.Stages[0].Digits, pe.TotalDigits(), pe.Bound())
}

func TestGreedyIsGloballyOptimal(t *testing.T) {
	// 暴力枚举所有 d_i∈[0,10] 的整数组合，验证贪心给出的总位数是
	// 满足 Σa_i·0.5·10^-d_i ≤ E 的全局最小值。
	rng := rand.New(rand.NewSource(11))
	for trial := 0; trial < 300; trial++ {
		k := 0.1 + rng.Float64()*10
		th := rng.Float64() * 2 * math.Pi
		E := math.Pow(10, -1-rng.Float64()*4)
		st := Conditions(k, th, 32, 32)
		pa := PlanAdaptive(k, th, 32, 32, E)
		opt := 1 << 30
		var d [StageCount]int
		for d[0] = 0; d[0] <= 10; d[0]++ {
			for d[1] = 0; d[1] <= 10; d[1]++ {
				for d[2] = 0; d[2] <= 10; d[2]++ {
					b := 0.0
					for i, s := range st {
						b += s.AmplifyTo * 0.5 * math.Pow10(-d[i])
					}
					if b <= E*(1+1e-12) && d[0]+d[1]+d[2] < opt {
						opt = d[0] + d[1] + d[2]
					}
				}
			}
		}
		if pa.TotalDigits() != opt {
			t.Fatalf("k=%v th=%v E=%v 贪心 %d 位，暴力最优 %d 位", k, th, E, pa.TotalDigits(), opt)
		}
	}
}

func TestWorstCaseGrid(t *testing.T) {
	// 网格扫描逼近各量化步的最坏舍入点，给出可复现的实测总误差。
	// 两个误差上限各跑一组：E=0.01（两方案同位）与 E=1e-4（自适应严格省位）。
	k, th, tx, ty := 2.0, math.Pi/4, 32.0, 32.0 // 45°：‖R‖∞=√2，放大最强
	scan := func(digits [StageCount]int) float64 {
		worst := 0.0
		for i := 0; i <= 400; i++ {
			for j := 0; j <= 400; j++ {
				p := geom.Point{
					X: float64(i)*0.025 + 0.0123,
					Y: float64(j)*0.025 + 0.0231,
				}
				got := Apply(p, k, th, tx, ty, digits)
				ref := Exact(p, k, th, tx, ty)
				w := math.Max(math.Abs(got.X-ref.X), math.Abs(got.Y-ref.Y))
				if w > worst {
					worst = w
				}
			}
		}
		return worst
	}
	for _, E := range []float64{testE, 1e-4} {
		pa := PlanAdaptive(k, th, tx, ty, E)
		pe := PlanEqual(k, th, tx, ty, E)
		wa, we := scan(pa.DigitsOf()), scan(pe.DigitsOf())
		t.Logf("E=%g: 自适应 实测=%g 保证=%g 位数=%v(共%d); 等精度 实测=%g 保证=%g 位数=%d(共%d)",
			E, wa, pa.Bound(), pa.DigitsOf(), pa.TotalDigits(),
			we, pe.Bound(), pe.Stages[0].Digits, pe.TotalDigits())
		if wa > pa.Bound() || we > pe.Bound() || pa.Bound() > E || pe.Bound() > E {
			t.Fatal("实测或保证上界越界")
		}
	}
}
