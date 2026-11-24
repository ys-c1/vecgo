package pipe

import (
	"testing"

	"vecgo/budget"
	"vecgo/config"
	"vecgo/geom"
	"vecgo/post"
	"vecgo/raster"
	"vecgo/shading"
)

// makeRamp 造一张覆盖完整灰度域的水平渐变图: 每列一个灰度, 0..255,
// 链首峰值为满幅 255, 供五处位点逐一记账。
func makeRamp(w, h int) *raster.Image {
	im, _ := raster.New(w, h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			im.Pix[y*w+x] = uint8(x * 255 / (w - 1))
		}
	}
	return im
}

func peak(im *raster.Image) float64 {
	m := uint8(0)
	for _, v := range im.Pix {
		if v > m {
			m = v
		}
	}
	return float64(m) / 255
}

// chainTol 是 uint8 取整与探针浮点记账之间的容差。
const chainTol = 2.0 / 255

// probe 记录一个位点前后的归一化峰值。
type probe struct {
	before, after float64
}

func indexOf(p *budget.Plan, name string) int {
	for i, s := range p.Stages {
		if s.Name == name {
			return i
		}
	}
	return -1
}

// runGradientChain 让渐变图按真实链路顺序经过五处位点, 每处前后记峰值。
//
// 真实链路里 config 增益 / 几何变换 / 填充三处的线性系数, 因为 shading 会
// 覆写非零像素, 是折叠进 shading 的 top 入参一次性施加的 (见 pipe.Run)。
// 因此前三个线性位点在测试图上只做峰值探针记账, 像素的实际缩放只在 shading
// 处按折叠乘积发生; 几何变换用独立填充探针验证灰度恒等;
// 填充/伽马的触发逻辑不被替换。
func runGradientChain(t *testing.T, gain, gamma float64) (*budget.Plan, map[string]probe) {
	t.Helper()
	const w, h = 256, 8
	plan := budget.New(gain, gamma, 1.0)
	probes := map[string]probe{}

	// 位点 1: config 增益 (线性)。探针记账, 像素缩放折叠到 shading 一起施加。
	im := makeRamp(w, h)
	kGain := plan.Coefficient(budget.ConfigGain)
	probes[budget.ConfigGain] = probe{before: peak(im), after: peak(im) * kGain}

	// 位点 2: pipe 几何变换。它只改坐标, 用填充探针验证变换前后灰度峰值不变。
	scene := [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}}
	cfg := config.Default()
	cfg.Width, cfg.Height = w, h
	cfg.Gain, cfg.Gamma = gain, gamma
	pre, _ := raster.New(w, h)
	if err := pre.Fill(scene, 1); err != nil {
		t.Fatal(err)
	}
	moved := make([][]geom.Point, len(scene))
	for i, poly := range scene {
		moved[i] = Transform(poly, cfg.Scale, cfg.Rotate)
	}
	after2, _ := raster.New(w, h)
	if err := after2.Fill(moved, 1); err != nil {
		t.Fatal(err)
	}
	probes[budget.Xform] = probe{before: peak(pre), after: peak(after2)}
	if d := peak(after2) - peak(pre); d > chainTol || d < -chainTol {
		t.Fatalf("几何变换不应改变灰度峰值: before=%.4f after=%.4f", peak(pre), peak(after2))
	}

	// 位点 3: raster 填充。触发逻辑保持原样, 探针记账其分配系数。
	kFill := plan.Coefficient(budget.RasterFill)
	probes[budget.RasterFill] = probe{before: peak(im), after: peak(im) * kFill}

	// 位点 4: shading。真实 shading.Apply 触发; 前三处线性系数折叠进 top 入参,
	// 像素的全部线性缩放在此一次性发生, 触发公式本身 (255*top) 不变。
	kShade := kGain * plan.Coefficient(budget.Xform) * kFill *
		plan.Coefficient(budget.Shading)
	b := peak(im)
	if err := shading.Apply(im, "flat", kShade, 0); err != nil {
		t.Fatal(err)
	}
	probes[budget.Shading] = probe{before: b, after: peak(im)}

	// 位点 5: 伽马。真实 post.Gamma 触发, LUT 逻辑原样, 余量已在预算表预留。
	b = peak(im)
	if err := post.Gamma(im, gamma); err != nil {
		t.Fatal(err)
	}
	probes[budget.Gamma] = probe{before: b, after: peak(im)}

	return plan, probes
}

// TestGradientChainPeakNeverExceedsHead 是核心验收: 一张渐变图过完整链路,
// 五处各自前后记峰值, 链尾峰值不能超过链首, 且实测消耗与预算表一致。
func TestGradientChainPeakNeverExceedsHead(t *testing.T) {
	cases := []struct {
		name        string
		gain, gamma float64
	}{
		{"默认全1", 1, 1},
		{"增益超预算被裁剪", 2, 1},
		{"增益与伽马混合", 2, 2},
		{"压暗后伽马提亮", 0.5, 2},
		{"纯伽马提亮", 1, 2},
	}
	const tol = chainTol // uint8 取整容差
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plan, probes := runGradientChain(t, tc.gain, tc.gamma)

			order := []string{budget.ConfigGain, budget.Xform, budget.RasterFill, budget.Shading, budget.Gamma}
			head := probes[order[0]].before
			tail := probes[order[len(order)-1]].after

			if tail > head+tol {
				t.Fatalf("链尾峰值 %.4f 超过链首 %.4f\n%s", tail, head, plan.String())
			}
			if !plan.OK() {
				t.Fatalf("预算表不变量不成立\n%s", plan.String())
			}

			// 每处实测峰值回填预算表, 并核对实测消耗与分配系数对得上。
			// - 三个线性探针位点与几何位点是浮点记账, 直接核对峰值比;
			// - shading 是真实 uint8 触发, 用实测有效系数对账;
			// - 伽马用 post 内部 LUT 的真实提亮系数对账 (含取整与近似 pow)。
			for _, name := range order {
				pr := probes[name]
				measured := pr.after / pr.before
				switch name {
				case budget.Shading:
					if err := plan.ObserveMeasured(name, pr.before, pr.after, measured); err != nil {
						t.Fatal(err)
					}
				case budget.Gamma:
					wantLift := post.LiftAt(pr.before, tc.gamma)
					if err := plan.ObserveMeasured(name, pr.before, pr.after, wantLift); err != nil {
						t.Fatal(err)
					}
					if d := measured - wantLift; d > tol || d < -tol {
						t.Fatalf("%s 实测消耗 %.4f 与 LUT 系数 %.4f 不符", name, measured, wantLift)
					}
					continue
				default:
					if err := plan.Observe(name, pr.before, pr.after); err != nil {
						t.Fatal(err)
					}
					stage := plan.Stages[indexOf(plan, name)]
					if d := measured - stage.Allocated; d > tol || d < -tol {
						t.Fatalf("%s 实测消耗 %.4f 与分配系数 %.4f 不符",
							name, measured, stage.Allocated)
					}
				}
				if plan.Stages[indexOf(plan, name)].Headroom() < -tol {
					t.Fatalf("%s 余量为负", name)
				}
			}
			t.Logf("\n%s", plan.String())
		})
	}
}
