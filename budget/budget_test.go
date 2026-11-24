package budget

import (
	"math"
	"strings"
	"testing"
)

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-12 }

func TestDefaultPlanStaysAtCeiling(t *testing.T) {
	p := New(1, 1, 1)
	for _, s := range p.Stages {
		if !approx(s.Allocated, 1) {
			t.Fatalf("默认系数应全部为 1, %s=%g", s.Name, s.Allocated)
		}
	}
	if !p.OK() || !approx(p.HeadPeak(), 1) || !approx(p.TailPeak(), 1) {
		t.Fatalf("默认链路应贴天花板且不超限: %s", p.String())
	}
}

func TestLinearProductTrimmed(t *testing.T) {
	p := New(2, 1, 1.6) // 线性请求乘积 3.2
	prod := p.Coefficient(ConfigGain) * p.Coefficient(RasterFill) * p.Coefficient(Shading)
	if prod > Ceiling+1e-12 {
		t.Fatalf("线性乘积超限: %g", prod)
	}
	for _, name := range []string{ConfigGain, RasterFill, Shading} {
		if p.Coefficient(name) > 1+1e-12 {
			t.Fatalf("单一位点系数瞬时超限: %s=%g", name, p.Coefficient(name))
		}
	}
	if !p.OK() {
		t.Fatalf("超预算链路仍应满足不变量: %s", p.String())
	}
	// 裁剪是等比的: 三个位点拿到同一压缩比。
	tr := p.Coefficient(ConfigGain) / 2
	if !approx(p.Coefficient(Shading)/1.6, tr) || !approx(p.Coefficient(RasterFill), tr) {
		t.Fatal("线性位点应按同一压缩比等比收回")
	}
}

func TestSingleOverdriveAlsoCapped(t *testing.T) {
	p := New(2, 1, 1)
	if p.Coefficient(ConfigGain) > 1+1e-12 {
		t.Fatal("单点增益请求也不许让中间峰值突破天花板")
	}
	if !p.OK() {
		t.Fatalf("不变量应成立: %s", p.String())
	}
}

func TestGammaHeadroomReservedUpstream(t *testing.T) {
	// 入射峰值 1.0 时伽马有效系数恰为 1, 无提亮余量可吃;
	// 入射峰值 0.25、γ=2 时有效系数应为 2。
	g := New(0.25, 2, 1)
	if !approx(g.Coefficient(Gamma), 2) {
		t.Fatalf("伽马有效系数应为 2, 实际 %g", g.Coefficient(Gamma))
	}
	// 伽马只对入峰值做幂运算, 后峰永不超过入峰, 链尾不超链首。
	if !g.OK() || g.TailPeak() > g.HeadPeak()+1e-12 {
		t.Fatalf("伽马链路不变量被破坏: %s", g.String())
	}
	// 暗调提亮的实际消耗确实大于 1, 余量必须显式登记而非隐式吞掉。
	gam := g.Stages[4]
	if gam.Consumed < 1.99 || gam.Headroom() < 0 {
		t.Fatalf("伽马消耗与余量登记异常: %+v", gam)
	}
}

func TestGammaBelowOneAttenuates(t *testing.T) {
	// 入射峰值为 1 时任何 γ 的有效系数都是 1; γ<1 的衰减只发生在中间调,
	// 因此先把入峰压到 0.25 再看系数。
	g := New(0.25, 0.5, 1)
	if g.Coefficient(Gamma) >= 1 {
		t.Fatal("γ<1 在非满幅峰值处应为衰减系数")
	}
	if !g.OK() {
		t.Fatalf("γ<1 不变量应成立: %s", g.String())
	}
}

func TestTransformAndFillRows(t *testing.T) {
	p := New(3, 2, 1)
	if !approx(p.Coefficient(Xform), 1) {
		t.Fatal("几何变换灰度恒等, 系数恒为 1")
	}
	if !approx(p.Coefficient(RasterFill), 1.0/3) {
		t.Fatalf("填充应按统一压缩比拿到 1/3: %g", p.Coefficient(RasterFill))
	}
}

func TestTableIsQueryable(t *testing.T) {
	p := New(1, 2, 1)
	table := p.String()
	for _, name := range []string{ConfigGain, Xform, RasterFill, Shading, Gamma} {
		if !strings.Contains(table, name) {
			t.Fatalf("表格缺少位点 %s:\n%s", name, table)
		}
	}
	for _, want := range []string{"余量", "消耗", "链尾峰值", "天花板"} {
		if !strings.Contains(table, want) {
			t.Fatalf("表格缺少 %q 列:\n%s", want, table)
		}
	}
	if err := p.Observe(ConfigGain, 1, 0.5); err != nil {
		t.Fatal(err)
	}
	if !approx(p.Stages[0].Consumed, 0.5) {
		t.Fatal("Observe 应回填实测消耗")
	}
	if err := p.Observe("不存在", 1, 1); err == nil {
		t.Fatal("未知位点应报错")
	}
}

func TestUnknownCoefficientPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("未知位点应 panic")
		}
	}()
	New(1, 1, 1).Coefficient("bogus")
}
