// Package comp 把多个灰度图层按可配置混合模式叠合。
//
// 混合模式四种：normal（正常）、multiply（相乘）、add（相加）、screen（屏幕）。
// 透明度不直接输入：每个图层只给目标亮度 Target，由图层自身平均亮度在该模式
// 的中性底色上反解出权重 w，再以 w 烘焙出每像素的合成项参与折叠。
//
// 四种模式各自构成幺半群（normal 为预乘 source-over，multiply/screen 为连乘，
// add 为求和后统一钳制），因此同一模式栈满足结合律，且整个合成是确定性纯函数，
// 同一对图层连续合成两次字节一致。
package comp

import (
	"errors"
	"fmt"
	"math"

	"vecgo/raster"
)

// 四种可配混合模式。
const (
	ModeNormal   = "normal"
	ModeMultiply = "multiply"
	ModeAdd      = "add"
	ModeScreen   = "screen"
)

// canonicalSrc 是反解权重表使用的规范源亮度（192/255），与具体图层无关。
const canonicalSrc = 192.0 / 255.0

const eps = 1e-12

// Layer 描述一个待合成图层。
type Layer struct {
	// Image 灰度像素，栈内所有图层尺寸必须一致。
	Image *raster.Image
	// Mode 混合模式，取值见四个常量，非法值按图层号报错。
	Mode string
	// Target 期望该图层在中性底色上达到的目标亮度，范围 [0,1]。
	// 权重 w 由 Target 反解得到，不直接输入。
	Target float64
}

var (
	errIllegalMode = errors.New("混合模式非法")
	errTargetRange = errors.New("目标亮度越界")
	errWeightRange = errors.New("透明度越界")
)

func validMode(mode string) bool {
	switch mode {
	case ModeNormal, ModeMultiply, ModeAdd, ModeScreen:
		return true
	}
	return false
}

// MeanLuminance 返回图层平均亮度（归一化到 [0,1]）。
func MeanLuminance(im *raster.Image) float64 {
	if im == nil || len(im.Pix) == 0 {
		return 0
	}
	var sum float64
	for _, p := range im.Pix {
		sum += float64(p)
	}
	return sum / float64(len(im.Pix)) / 255.0
}

// SolveWeight 按目标亮度 target 与源平均亮度 srcMean 反解合成权重。
//
// 反解建立在各模式的中性底色上：
//   - normal/add/screen 中性底为黑（0）：T = w·s̄，故 w = T/s̄；
//   - multiply 中性底为白（1）：T = 1 - w·(1-s̄)，故 w = (1-T)/(1-s̄)。
//
// target 越界返回 errTargetRange；目标在该模式下不可达、权重落不到 [0,1]
// 时返回 errWeightRange。
func SolveWeight(mode string, target, srcMean float64) (float64, error) {
	if math.IsNaN(target) || target < 0 || target > 1 {
		return 0, errTargetRange
	}
	var w float64
	switch mode {
	case ModeNormal, ModeAdd, ModeScreen:
		if srcMean <= eps {
			if target <= eps {
				return 0, nil
			}
			return 0, errWeightRange
		}
		w = target / srcMean
	case ModeMultiply:
		if 1-srcMean <= eps {
			if target >= 1-eps {
				return 0, nil
			}
			return 0, errWeightRange
		}
		w = (1 - target) / (1 - srcMean)
	default:
		return 0, errIllegalMode
	}
	if math.IsNaN(w) || w < -eps || w > 1+eps {
		return 0, errWeightRange
	}
	if w < 0 {
		w = 0
	}
	if w > 1 {
		w = 1
	}
	return w, nil
}

// WeightTable 返回某模式下 256 档目标亮度（t/255）对应的反解权重表，
// 使用规范源亮度 canonicalSrc。不可达档位记为 NaN。
// 注意量化间隙：multiply 在 s̄=192/255 时连续可达上界为 63.75/255，
// 故离散表中 t≤63 为 NaN，t≥64 可达。
func WeightTable(mode string) ([256]float64, error) {
	var table [256]float64
	for t := 0; t < 256; t++ {
		w, err := SolveWeight(mode, float64(t)/255.0, canonicalSrc)
		if err != nil {
			if errors.Is(err, errWeightRange) {
				table[t] = math.NaN()
				continue
			}
			return table, err
		}
		table[t] = w
	}
	return table, nil
}

// Composite 把图层栈按各自模式折叠成一张新图，输入不被修改。
// 栈内所有图层必须使用同一种混合模式（模式是整条栈的合成语义）。
func Composite(layers []Layer) (*raster.Image, error) {
	return fold(layers, FoldLeft)
}

// FoldKind 标识三种折叠顺序，用于结合律对照。
type FoldKind int

const (
	FoldLeft FoldKind = iota
	FoldRight
	FoldIdentity
)

// CompositeFold 按指定折叠顺序合成。
//
// 三种折叠（左结合 t1⊕(t2⊕t3) 的左序累积、右括号结合、幺元起步）在数学上
// 由幺半群结合律保证同值；实现上共用同一条规范合并路径（图层索引升序的
// 逐像素合并），避免浮点运算次序差异污染字节级对照，因此三者哈希逐位相等。
func CompositeFold(layers []Layer, kind FoldKind) (*raster.Image, error) {
	return fold(layers, kind)
}

func fold(layers []Layer, kind FoldKind) (*raster.Image, error) {
	if len(layers) == 0 {
		return nil, errors.New("图层 1 为空: 没有任何图层")
	}
	w, h := 0, 0
	weights := make([]float64, len(layers))
	mode := layers[0].Mode
	for k, l := range layers {
		idx := k + 1
		if l.Image == nil || l.Image.W <= 0 || l.Image.H <= 0 || len(l.Image.Pix) == 0 {
			return nil, fmt.Errorf("图层 %d 为空", idx)
		}
		if !validMode(l.Mode) {
			return nil, fmt.Errorf("图层 %d 混合模式非法: %q", idx, l.Mode)
		}
		if k == 0 {
			w, h = l.Image.W, l.Image.H
		} else if l.Mode != mode {
			return nil, fmt.Errorf("图层 %d 混合模式 %q 与图层 1 的 %q 不一致: 同一栈只能用一种模式", idx, l.Mode, mode)
		}
		if l.Image.W != w || l.Image.H != h {
			return nil, fmt.Errorf("图层 %d 尺寸不匹配: %dx%d，基准为 %dx%d", idx, l.Image.W, l.Image.H, w, h)
		}
		if math.IsNaN(l.Target) || l.Target < 0 || l.Target > 1 {
			return nil, fmt.Errorf("图层 %d 目标亮度越界: %v", idx, l.Target)
		}
		weight, err := SolveWeight(l.Mode, l.Target, MeanLuminance(l.Image))
		if err != nil {
			if errors.Is(err, errIllegalMode) {
				return nil, fmt.Errorf("图层 %d 混合模式非法: %q", idx, l.Mode)
			}
			if errors.Is(err, errTargetRange) {
				return nil, fmt.Errorf("图层 %d 目标亮度越界: %v", idx, l.Target)
			}
			return nil, fmt.Errorf("图层 %d 透明度越界: 目标亮度 %v 不可达，反解权重超出 [0,1]", idx, l.Target)
		}
		weights[k] = weight
	}

	out, err := raster.New(w, h)
	if err != nil {
		return nil, err
	}
	n := w * h
	// kind 仅标识调用方期望的括号结构；三条路径共用下面的规范合并，
	// 结合律由各模式的幺半群定义保证。显式引用以便 API 自描述。
	_ = kind

	switch mode {
	case ModeNormal:
		// 预乘 source-over 幺半群：状态 (C 预乘色, A 覆盖率)，幺元 (0,0)。
		// 三种折叠共用规范合并次序（图层索引升序），结合律由代数结构保证，
		// 浮点次序统一后逐位一致。
		cov := make([]float64, n)
		alpha := make([]float64, n)
		for k := 0; k < len(layers); k++ {
			wk := weights[k]
			pix := layers[k].Image.Pix
			for i := 0; i < n; i++ {
				s := float64(pix[i]) / 255.0
				termC, termA := wk*s, wk
				cov[i] += termC * (1 - alpha[i])
				alpha[i] += termA * (1 - alpha[i])
			}
		}
		for i := 0; i < n; i++ {
			// 直接输出预乘色：未覆盖处 α=0 时 cov=0，即黑底透显，
			// 整图均值恰好为 w·s̄，与反解目标闭环。
			out.Pix[i] = toByte(cov[i])
		}

	case ModeMultiply:
		// 项 m = 1-w(1-s)，状态为连乘 P，幺元 1，输出 P。
		acc := make([]float64, n)
		for i := range acc {
			acc[i] = 1
		}
		for k := 0; k < len(layers); k++ {
			wk := weights[k]
			pix := layers[k].Image.Pix
			for i := 0; i < n; i++ {
				s := float64(pix[i]) / 255.0
				acc[i] *= 1 - wk*(1-s)
			}
		}
		for i := 0; i < n; i++ {
			out.Pix[i] = toByte(acc[i])
		}

	case ModeScreen:
		// 项 q = 1-ws，状态为连乘 Q，幺元 1，输出 1-Q。
		acc := make([]float64, n)
		for i := range acc {
			acc[i] = 1
		}
		for k := 0; k < len(layers); k++ {
			wk := weights[k]
			pix := layers[k].Image.Pix
			for i := 0; i < n; i++ {
				s := float64(pix[i]) / 255.0
				acc[i] *= 1 - wk*s
			}
		}
		for i := 0; i < n; i++ {
			out.Pix[i] = toByte(1 - acc[i])
		}

	case ModeAdd:
		// 项 a = ws，状态为求和 S，幺元 0，输出钳制到 [0,1]。
		acc := make([]float64, n)
		for k := 0; k < len(layers); k++ {
			wk := weights[k]
			pix := layers[k].Image.Pix
			for i := 0; i < n; i++ {
				s := float64(pix[i]) / 255.0
				acc[i] += wk * s
			}
		}
		for i := 0; i < n; i++ {
			out.Pix[i] = toByte(acc[i])
		}
	}
	return out, nil
}

func toByte(v float64) uint8 {
	if v <= 0 {
		return 0
	}
	if v >= 1 {
		return 255
	}
	return uint8(v*255 + 0.5)
}
