// Package probe 提供缺省实验专用的确定性场景与工具。
//
// 场景选为一个整体旋转过的正方形，四条边都不与像素网格对齐。这样在默认
// supersample=2 下，边缘像素的覆盖率会出现 25%/50%/75% 等中间值，使
// supersample、gamma、blur、rotate、shading 等字段都能在像素上被观察到；
// 否则轴对齐正方形只会产生 0/255 两种值，gamma/blur 的对照会变成空测。
package probe

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"strings"

	"vecgo/config"
	"vecgo/geom"
	"vecgo/pipe"
	"vecgo/post"
	"vecgo/raster"
)

// Scene 返回实验场景：中心 (5,5)、半边 4.6、旋转 0.35 弧度的正方形。
// 顶点按逆时针给出，坐标保留三位小数，任何一次渲染都不含随机量。
func Scene() [][]geom.Point {
	return [][]geom.Point{{
		{X: 2.256, Y: -0.898},
		{X: 10.897, Y: 2.256},
		{X: 7.744, Y: 10.897},
		{X: -0.897, Y: 7.744},
	}}
}

// Render 用给定配置渲染实验场景。调用链与 cmd 相同：pipe.Run。
func Render(cfg config.Config) (*raster.Image, error) {
	return pipe.Run(Scene(), cfg)
}

// AltValues 是逐字段“非默认值”敏感性对照值。用途：证明该字段在本场景里
// 确实能改变输出；若换值后输出完全不变，则“缺省 vs 显式”的零差不能作为
// 默认值生效的证据，只能改用取值快照核对。取值均在 config.Validate 合法区间内。
var AltValues = map[string]any{
	"width":       48,
	"height":      48,
	"scale":       2.0,
	"rotate":      0.5,
	"supersample": 1,
	"shading":     "gradient",
	"gamma":       2.0,
	"blur":        1,
	"output":      "alt.pgm",
}

// AltFor 返回该字段在 context 下可用的非默认值；若对照值恰好与上下文相同
// （例如上下文已是 gradient），改取另一个合法值，避免空测。
func AltFor(name string, context config.Config) (any, error) {
	v, ok := AltValues[name]
	if !ok {
		return nil, fmt.Errorf("字段 %s 缺少敏感性对照值", name)
	}
	switch name {
	case "width":
		if context.Width == 48 {
			return 52, nil
		}
	case "height":
		if context.Height == 48 {
			return 52, nil
		}
	case "scale":
		if context.Scale == 2 {
			return 3.0, nil
		}
	case "rotate":
		if context.Rotate == 0.5 {
			return 0.2, nil
		}
	case "supersample":
		if context.Supersample == 1 {
			return 4, nil
		}
	case "shading":
		if context.Shading == "gradient" {
			return "flat", nil
		}
	case "gamma":
		if context.Gamma == 2 {
			return 1.5, nil
		}
	case "blur":
		if context.Blur == 1 {
			return 2, nil
		}
	}
	return v, nil
}

// WithAlt 返回把 name 换成 alt 后的配置副本。
func WithAlt(context config.Config, name string, alt any) (config.Config, error) {
	switch name {
	case "width":
		context.Width = alt.(int)
	case "height":
		context.Height = alt.(int)
	case "scale":
		context.Scale = alt.(float64)
	case "rotate":
		context.Rotate = alt.(float64)
	case "supersample":
		context.Supersample = alt.(int)
	case "shading":
		context.Shading = alt.(string)
	case "gamma":
		context.Gamma = alt.(float64)
	case "blur":
		context.Blur = alt.(int)
	case "output":
		context.Output = alt.(string)
	default:
		return context, fmt.Errorf("未知字段 %s", name)
	}
	return context, nil
}

// Diff 描述两张图逐像素比较的结果。
type Diff struct {
	DimsMatch bool
	N         int // 不同的像素个数；尺寸不同时无意义
	FirstX    int // 第一个差异像素坐标；N==0 时为 -1
	FirstY    int
}

// Equal 表示尺寸相同且逐像素完全一致。
func (d Diff) Equal() bool { return d.DimsMatch && d.N == 0 }

// Compare 逐像素比较两张图。
func Compare(a, b *raster.Image) Diff {
	if a.W != b.W || a.H != b.H {
		return Diff{DimsMatch: false, N: -1, FirstX: -1, FirstY: -1}
	}
	d := Diff{DimsMatch: true, FirstX: -1, FirstY: -1}
	for i := range a.Pix {
		if a.Pix[i] != b.Pix[i] {
			if d.N == 0 {
				d.FirstX = i % a.W
				d.FirstY = i / a.W
			}
			d.N++
		}
	}
	return d
}

// Sens 是一次敏感性对照的结果。
type Sens struct {
	Pixels      int  // 差异像素数
	DimsChanged bool // 画幅尺寸是否改变
	Alt         any  // 实际使用的非默认值
}

// Observable 表示换值后输出确实发生了变化（含尺寸变化）。
func (s Sens) Observable() bool { return s.DimsChanged || s.Pixels > 0 }

// Sensitivity 在 context 下把 name 换成非默认值渲染，与 context 原样渲染比较。
func Sensitivity(name string, context config.Config) (Sens, error) {
	alt, err := AltFor(name, context)
	if err != nil {
		return Sens{}, err
	}
	altCfg, err := WithAlt(context, name, alt)
	if err != nil {
		return Sens{}, err
	}
	base, err := Render(context)
	if err != nil {
		return Sens{}, err
	}
	changed, err := Render(altCfg)
	if err != nil {
		return Sens{}, err
	}
	d := Compare(base, changed)
	return Sens{Pixels: d.N, DimsChanged: !d.DimsMatch, Alt: alt}, nil
}

// ScaleProbe 验证 scale 对输出的实际作用。返回每个取值相对 scale=1 的差异像素数，
// 以及顶点的变换是否随 scale 改变（几何层面的直接证据）。
func ScaleProbe() (map[float64]int, bool, error) {
	c := config.Default()
	one := c
	one.Scale = 1
	base, err := Render(one)
	if err != nil {
		return nil, false, err
	}
	out := map[float64]int{}
	for _, v := range []float64{0.25, 0.5, 2, 7.5, 100} {
		cc := c
		cc.Scale = v
		im, err := Render(cc)
		if err != nil {
			return nil, false, err
		}
		out[v] = Compare(base, im).N
	}
	pts := []geom.Point{{X: 2, Y: 3}, {X: 5, Y: 7}}
	t1 := pipe.Transform(pts, 1, 0)
	t2 := pipe.Transform(pts, 2, 0)
	transformed := false
	for i := range pts {
		if t1[i] != t2[i] {
			transformed = true
		}
	}
	return out, transformed, nil
}

// GammaProbe 检查默认 gamma=1 是否等价于“不处理”。它直接调用性能管线
// 同一个 post.Gamma，对 0..255 全灰阶做映射，返回被改动的灰阶个数及前几个例子。
func GammaProbe() (mismatches int, examples []string, err error) {
	im, err := raster.New(256, 1)
	if err != nil {
		return 0, nil, err
	}
	for i := 0; i < 256; i++ {
		im.Pix[i] = uint8(i)
	}
	orig := append([]uint8(nil), im.Pix...)
	if err := post.Gamma(im, 1); err != nil {
		return 0, nil, err
	}
	for i := 0; i < 256; i++ {
		if im.Pix[i] != orig[i] {
			mismatches++
			if len(examples) < 6 {
				examples = append(examples, fmt.Sprintf("%d→%d", orig[i], im.Pix[i]))
			}
		}
	}
	return mismatches, examples, nil
}

// LoadPGM 读取 SavePGM 写出的 P5 文件，用于与改动前产物比较。
// 头部格式固定为 "P5\n<w> <h>\n255\n"，其后是裸像素，按该格式严格解析。
func LoadPGM(path string) (*raster.Image, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var imW, imH int
	rest := b
	for i := 0; i < 3; i++ {
		idx := bytes.IndexByte(rest, '\n')
		if idx < 0 {
			return nil, fmt.Errorf("%s: PGM 头不完整", path)
		}
		line := strings.TrimSpace(string(rest[:idx]))
		switch i {
		case 0:
			if line != "P5" {
				return nil, fmt.Errorf("%s: 不是 P5 PGM: %q", path, line)
			}
		case 1:
			fields := strings.Fields(line)
			if len(fields) != 2 {
				return nil, fmt.Errorf("%s: 尺寸行非法: %q", path, line)
			}
			w, err1 := strconv.Atoi(fields[0])
			h, err2 := strconv.Atoi(fields[1])
			if err1 != nil || err2 != nil {
				return nil, fmt.Errorf("%s: 尺寸行非法: %q", path, line)
			}
			imW, imH = w, h
		case 2:
			if line != "255" {
				return nil, fmt.Errorf("%s: 只支持 maxval=255，实际 %q", path, line)
			}
		}
		rest = rest[idx+1:]
	}
	im, err := raster.New(imW, imH)
	if err != nil {
		return nil, err
	}
	if len(rest) != len(im.Pix) {
		return nil, fmt.Errorf("%s: 像素字节数 %d，期望 %d", path, len(rest), len(im.Pix))
	}
	copy(im.Pix, rest)
	return im, nil
}
