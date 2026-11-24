// Package budget 在归一化灰度域 [0,1] 内统一管理渲染链路上五处系数的余量。
//
// 五处系数位点按链路顺序为: config 增益、pipe 几何变换、raster 填充、
// shading 着色、post 伽马。过去五处各自乘各自的系数, 没有人管总和, 混着调
// 亮度时链尾峰值会冲破灰度天花板。这里把总余量天花板固定为 Ceiling=1.0,
// 每一处分到多少系数、实际消耗多少、还剩多少余量, 都登记在一张可查的表里。
package budget

import (
	"fmt"
	"math"
	"strings"
)

// Ceiling 是灰度域的总余量天花板 (归一化峰值 1.0, 对应 uint8 的 255)。
const Ceiling = 1.0

// 五个系数位点的名称, 按链路顺序排列。
const (
	ConfigGain = "config-gain"    // config 的增益
	Xform      = "pipe-transform" // pipe 的几何变换
	RasterFill = "raster-fill"    // raster 的填充
	Shading    = "shading"        // shading 着色强度
	Gamma      = "post-gamma"     // 后处理那层的伽马
)

// Stage 是余量分配表中的一行: 登记一个系数位点的分配系数、实测消耗与剩余余量。
type Stage struct {
	Name       string  // 位点名称
	Request    float64 // 调用方请求的原始系数 (伽马行登记的是 γ 旋钮值)
	Allocated  float64 // 余量规划后实际下发的灰度域系数
	PeakBefore float64 // 进入该位点前的归一化峰值
	PeakAfter  float64 // 离开该位点后的归一化峰值
	Consumed   float64 // 实测消耗 = PeakAfter / PeakBefore
}

// Headroom 返回该位点之后距灰度天花板还剩多少归一化余量。
func (s Stage) Headroom() float64 { return Ceiling - s.PeakAfter }

// Plan 是五处系数的统一余量分配表。
type Plan struct {
	Stages []Stage
	byName map[string]int
}

// New 以链首满幅峰值 Ceiling 为基准, 对五处系数做一次统一余量分配。
//
// gain: config 增益请求; gamma: 后处理伽马旋钮值; shadeTop: shading 峰值强度请求。
// 几何变换只动坐标、不改灰度, 系数恒为 1; 填充触发后的固有覆盖度系数为 1。
// 伽马是非线性位点, 其有效系数按进入伽马时的峰值 x 换算为 x^(1/γ-1),
// 所需提亮余量由其上游的线性位点压缩让出; 伽马触发本身不在此被改动。
func New(gain, gamma, shadeTop float64) *Plan {
	const (
		xformReq = 1.0 // 几何变换灰度恒等
		fillReq  = 1.0 // 填充的固有覆盖度系数
	)

	// 三个可调线性位点 (config 增益 / 填充 / shading) 共享伽马之前的线性余量。
	// 中间值是 uint8, 任何单一位点瞬时超限都会被硬截断、无法靠后续位点拉回,
	// 因此压缩比同时约束总乘积与每个单一位点系数, 二者都不许突破天花板。
	linearReq := gain * fillReq * shadeTop
	trim := Ceiling / linearReq
	if r := 1 / gain; r < trim {
		trim = r
	}
	if r := 1 / shadeTop; r < trim {
		trim = r
	}
	if r := 1 / fillReq; r < trim {
		trim = r
	}
	if trim > 1 {
		trim = 1
	}
	linear := map[string]float64{
		ConfigGain: gain * trim,
		Xform:      xformReq, // 几何位点不参与灰度压缩
		RasterFill: fillReq * trim,
		Shading:    shadeTop * trim,
	}
	request := map[string]float64{
		ConfigGain: gain,
		Xform:      xformReq,
		RasterFill: fillReq,
		Shading:    shadeTop,
		Gamma:      gamma,
	}

	names := []string{ConfigGain, Xform, RasterFill, Shading, Gamma}
	stages := make([]Stage, 0, len(names))
	byName := make(map[string]int, len(names))
	peak := Ceiling // 链首峰值: 满幅渐变图的归一化峰值 1.0
	for _, name := range names {
		before := peak
		var allocated, after float64
		if name == Gamma {
			// 伽马有效提亮倍率, 在实际进入伽马的峰值处取值。
			allocated = gammaLift(peak, gamma)
			after = math.Pow(peak, 1/gamma)
		} else {
			allocated = linear[name]
			after = peak * allocated
		}
		stages = append(stages, Stage{
			Name:       name,
			Request:    request[name],
			Allocated:  allocated,
			PeakBefore: before,
			PeakAfter:  after,
			Consumed:   ratio(before, after),
		})
		byName[name] = len(stages) - 1
		peak = after
	}
	return &Plan{Stages: stages, byName: byName}
}

// Coefficient 返回某位点规划后下发的灰度域系数, 供链路实际乘用。
// 伽马位点的系数仅用于记账 (其触发逻辑不允许改动), 不应被外部乘用。
func (p *Plan) Coefficient(name string) float64 {
	i, ok := p.byName[name]
	if !ok {
		panic(fmt.Sprintf("budget: 未知系数位点 %q", name))
	}
	return p.Stages[i].Allocated
}

// Observe 用链路上实测的前后峰值回填某位点 (峰值为归一化值), 重算实测消耗。
func (p *Plan) Observe(name string, before, after float64) error {
	i, ok := p.byName[name]
	if !ok {
		return fmt.Errorf("budget: 未知系数位点 %q", name)
	}
	p.Stages[i].PeakBefore = before
	p.Stages[i].PeakAfter = after
	p.Stages[i].Consumed = ratio(before, after)
	return nil
}

// ObserveMeasured 用真实链路 (含 uint8 取整、伽马 LUT 近似) 的实测数据对账:
// 回填前后峰值, 并把该位点的分配系数改写为实测有效系数。规划阶段给的是
// 浮点理想值, 实测值才是该位点真正消耗掉的余量。
func (p *Plan) ObserveMeasured(name string, before, after, allocated float64) error {
	if err := p.Observe(name, before, after); err != nil {
		return err
	}
	i := p.byName[name]
	p.Stages[i].Allocated = allocated
	return nil
}

// HeadPeak 返回链首 (第一个位点之前) 的归一化峰值。
func (p *Plan) HeadPeak() float64 { return p.Stages[0].PeakBefore }

// TailPeak 返回链尾 (最后一个位点之后) 的归一化峰值。
func (p *Plan) TailPeak() float64 { return p.Stages[len(p.Stages)-1].PeakAfter }

// OK 报告余量不变量是否成立: 每个位点后的峰值都不突破天花板,
// 且链尾峰值不超过链首。
func (p *Plan) OK() bool {
	const eps = 1e-12
	for _, s := range p.Stages {
		if s.PeakAfter > Ceiling+eps {
			return false
		}
	}
	return p.TailPeak() <= p.HeadPeak()+eps
}

// String 把分配表渲染成可查阅的纯文本表格。
func (p *Plan) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "灰度域余量天花板 = %.4f\n", Ceiling)
	fmt.Fprintln(&b, "位点                 请求      分配      前峰      后峰      消耗      余量")
	for _, s := range p.Stages {
		name := s.Name
		if s.Name == Gamma {
			name = fmt.Sprintf("%s(γ=%.2f)", s.Name, s.Request)
		}
		fmt.Fprintf(&b, "%-18s  %8.4f  %8.4f  %8.4f  %8.4f  %8.4f  %8.4f\n",
			name, s.Request, s.Allocated, s.PeakBefore, s.PeakAfter, s.Consumed, s.Headroom())
	}
	fmt.Fprintf(&b, "链首峰值=%.4f 链尾峰值=%.4f 不变量(链尾≤链首)=%v\n",
		p.HeadPeak(), p.TailPeak(), p.OK())
	return b.String()
}

// gammaLift 返回伽马在入射峰值 x 处的有效灰度系数 x^(1/γ-1)。
// x<=0 时无信号可提亮, 系数记 1。
func gammaLift(x, gamma float64) float64 {
	if x <= 0 {
		return 1.0
	}
	return math.Pow(x, 1/gamma-1)
}

func ratio(before, after float64) float64 {
	if before <= 0 {
		return 1.0
	}
	return after / before
}
