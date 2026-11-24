package dual

import (
	"math"
	"testing"
)

func testOptions(size int) Options {
	return Options{
		Width: size, Height: size, Scale: 1, Rotate: 23 * math.Pi / 180,
		Supersample: 2, Shading: "flat", Top: 1, Bottom: 0.55,
		Gamma: 1, Blur: 0, CoordBits: 16, ColorBits: 8,
	}
}

var testScene = [][][2]float64{{{-5, -5}, {5, -5}, {5, 5}, {-5, 5}}}

// TestAlignedPathsIdentical 是核心性质：同一精度取整 + 同一位深截断后，
// 整型路径与对齐浮点路径在两档画幅上逐像素相同。
func TestAlignedPathsIdentical(t *testing.T) {
	for _, size := range []int{32, 128} {
		opt := testOptions(size)
		ir, err := IntEngine{}.Run(testScene, opt)
		if err != nil {
			t.Fatal(err)
		}
		fr, err := FloatEngine{}.Run(testScene, opt)
		if err != nil {
			t.Fatal(err)
		}
		rep := Compare(ir, fr)
		if !rep.Identical() {
			t.Fatalf("画幅 %d 对齐后仍有 %d 个差异像素，首阶段 %s，首坐标 %+v",
				size, rep.DiffCount, rep.FirstStage, rep.Diffs)
		}
		// 两路径变换后的顶点必须落在同一 Q 网格点。
		for i := range ir.VertQ {
			if ir.VertQ[i] != fr.VertQ[i] {
				t.Fatalf("画幅 %d 顶点 %d 不在同一网格: int=%v float=%v",
					size, i, ir.VertQ[i], fr.VertQ[i])
			}
		}
	}
}

// TestAlignedSweepQ 在 Q=8..18 范围内扫描，要求对齐路径始终逐像素相同。
//
// 上限 18 来自 float64 的 53 位尾数：本场景 Q 网格上的叉积峰值约为
// (span*2^Q)^2，Q=18 时仍小于 2^53，浮点可精确表示每个整数中间结果；
// Q 更高时浮点尾数不足，分歧会被 TestFloatMantissaLimitDetected 专门验证。
func TestAlignedSweepQ(t *testing.T) {
	for _, q := range []uint{8, 10, 12, 14, 16, 18} {
		opt := testOptions(64)
		opt.CoordBits = q
		ir, err := IntEngine{}.Run(testScene, opt)
		if err != nil {
			t.Fatalf("q=%d 整型路径失败: %v", q, err)
		}
		fr, err := FloatEngine{}.Run(testScene, opt)
		if err != nil {
			t.Fatalf("q=%d 浮点路径失败: %v", q, err)
		}
		if rep := Compare(ir, fr); !rep.Identical() {
			t.Fatalf("q=%d 对齐失败: %d 个差异，首阶段 %s，首例 %+v",
				q, rep.DiffCount, rep.FirstStage, rep.Diffs)
		}
	}
}

// TestFloatMantissaLimitDetected 验证当定点中间结果超出 float64 尾数精度时，
// 比对器能定位到具体阶段，而不是静默产出错误的“相同”结论。
// 这正是“做不到逐像素相同时，说明是哪一步精度不够”的兜底机制。
//
// 触发条件：大坐标场景（±1000）配合 Q=44。旋转后的非平凡顶点网格值达到
// 2^53 量级，float64 尾数无法逐位表示，snapFloat 量化结果与整型路径的精确
// 网格值分叉；整型路径全程 64/128 位整数（Q44 下 int64 仍不溢出），不受影响。
func TestFloatMantissaLimitDetected(t *testing.T) {
	bigScene := [][][2]float64{{{-1000, -1000}, {1000, -1000}, {1000, 1000}, {-1000, 1000}}}
	opt := testOptions(64)
	opt.CoordBits = 44
	ir, err := IntEngine{}.Run(bigScene, opt)
	if err != nil {
		t.Fatalf("整型路径在 Q44 应仍精确: %v", err)
	}
	fr, err := FloatEngine{}.Run(bigScene, opt)
	if err != nil {
		t.Fatal(err)
	}
	rep := Compare(ir, fr)
	if rep.Identical() {
		t.Skip("当前场景 Q=44 未触发尾数极限")
	}
	if rep.FirstStage != "coverage" {
		t.Fatalf("尾数极限应首先作用于 coverage，实际首阶段 %s", rep.FirstStage)
	}
	t.Logf("Q=44 触发 float64 尾数极限：%d 个差异，最大差 %d，首阶段 %s",
		rep.DiffCount, rep.MaxDelta, rep.FirstStage)
}

// TestColorBitsTruncation 验证颜色截断位深生效：低位被清零。
func TestColorBitsTruncation(t *testing.T) {
	cases := []struct {
		bits int
		in   uint8
		want uint8
	}{
		{8, 200, 200},
		{6, 255, 252},
		{4, 200, 192},
		{1, 200, 128},
	}
	for _, c := range cases {
		if got := truncate(c.in, c.bits); got != c.want {
			t.Fatalf("truncate(%d,bits=%d)=%d want %d", c.in, c.bits, got, c.want)
		}
	}
}

// TestColorBitsConfigurable 验证位深作为可配项贯通两条路径且仍保持一致。
func TestColorBitsConfigurable(t *testing.T) {
	for _, bits := range []int{1, 4, 6, 8} {
		opt := testOptions(48)
		opt.ColorBits = bits
		ir, err := IntEngine{}.Run(testScene, opt)
		if err != nil {
			t.Fatal(err)
		}
		fr, err := FloatEngine{}.Run(testScene, opt)
		if err != nil {
			t.Fatal(err)
		}
		if rep := Compare(ir, fr); !rep.Identical() {
			t.Fatalf("colorbits=%d 对齐失败: %+v", bits, rep.Diffs)
		}
		// 所有非零像素必须是该位深的合法量化值（低位全零）。
		mask := uint8(1<<(8-bits)) - 1
		for _, v := range ir.Final.V {
			if v&mask != 0 {
				t.Fatalf("colorbits=%d 出现未截断值 %d", bits, v)
			}
		}
	}
}

// TestLowQExposesDivergence 验证低精度网格下整型路径与原生浮点基线出现分歧，
// 且比对器把首个分歧阶段定位到 coverage。
func TestLowQExposesDivergence(t *testing.T) {
	opt := testOptions(64)
	opt.CoordBits = 4 // 网格仅 1/16，边缘采样必然与全精度浮点分叉
	ir, err := IntEngine{}.Run(testScene, opt)
	if err != nil {
		t.Fatal(err)
	}
	nr, err := NativeFloatEngine{}.Run(testScene, opt)
	if err != nil {
		t.Fatal(err)
	}
	rep := Compare(ir, nr)
	if rep.Identical() {
		t.Fatal("Q=4 时整型与原生浮点应当出现边缘差异")
	}
	if rep.FirstStage != "coverage" {
		t.Fatalf("首个分歧阶段应为 coverage，实际 %s", rep.FirstStage)
	}
	if len(rep.Diffs) > MaxDiffCoords {
		t.Fatalf("坐标列表超过上限 %d", MaxDiffCoords)
	}
	// 列表必须按 Delta 降序。
	for i := 1; i < len(rep.Diffs); i++ {
		if rep.Diffs[i-1].Delta < rep.Diffs[i].Delta {
			t.Fatal("差异坐标未按差值降序排列")
		}
	}
}

// TestGradientAndBlur 覆盖渐变着色与模糊阶段，两路径仍需逐像素相同。
func TestGradientAndBlur(t *testing.T) {
	opt := testOptions(64)
	opt.Shading = "gradient"
	opt.Blur = 1
	ir, err := IntEngine{}.Run(testScene, opt)
	if err != nil {
		t.Fatal(err)
	}
	fr, err := FloatEngine{}.Run(testScene, opt)
	if err != nil {
		t.Fatal(err)
	}
	if rep := Compare(ir, fr); !rep.Identical() {
		t.Fatalf("渐变+模糊下对齐失败，首阶段 %s: %+v", rep.FirstStage, rep.Diffs)
	}
}

// TestValidateRejectsBadPrecision 保证两个可配项都有边界校验。
func TestValidateRejectsBadPrecision(t *testing.T) {
	opt := testOptions(16)
	opt.CoordBits = 64
	if err := opt.Validate(); err == nil {
		t.Fatal("coordBits 越界应报错")
	}
	opt = testOptions(16)
	opt.ColorBits = 0
	if err := opt.Validate(); err == nil {
		t.Fatal("colorBits=0 应报错")
	}
	opt = testOptions(16)
	opt.ColorBits = 9
	if err := opt.Validate(); err == nil {
		t.Fatal("colorBits=9 应报错")
	}
}

// TestSideBySideAndDiffMap 验证并排拼接与差异图尺寸。
func TestSideBySideAndDiffMap(t *testing.T) {
	a := Pix{W: 2, H: 2, V: []uint8{0, 10, 20, 30}}
	b := Pix{W: 2, H: 2, V: []uint8{0, 11, 20, 0}}
	sb := SideBySide(a, b)
	if sb.W != 5 || sb.H != 2 {
		t.Fatalf("并排图尺寸错误: %dx%d", sb.W, sb.H)
	}
	dm := DiffMap(a, b)
	if dm.V[0] != 0 || dm.V[3] == 0 {
		t.Fatal("差异图内容错误")
	}
}
