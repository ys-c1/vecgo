package post

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"vecgo/raster"
)

// Kernel 描述误差扩散核。Offsets 中每项为 {dx, dy, weight}，表示当前像素的
// 量化误差按 weight 比例扩散到邻居 (x+dx, y+dy)。只接受“前向”偏移
// （dy>0，或 dy==0 且 dx>0）；蛇形推进时在反向行自动把 dx 镜像。
type Kernel struct {
	Name      string
	Offsets   [][3]float64
	Normalize bool // true 时按权重和归一化，保证扩散出去的误差总量守恒
}

// FloydSteinberg 返回经典 Floyd–Steinberg 核（原始整数权重 7/3/5/1，和为 16）。
func FloydSteinberg() Kernel {
	return Kernel{
		Name: "floyd-steinberg",
		Offsets: [][3]float64{
			{1, 0, 7},
			{-1, 1, 3},
			{0, 1, 5},
			{1, 1, 1},
		},
		Normalize: true,
	}
}

// Atkinson 返回 Atkinson 核（6 个权重为 1 的前向偏移，和为 6）。
func Atkinson() Kernel {
	return Kernel{
		Name: "atkinson",
		Offsets: [][3]float64{
			{1, 0, 1},
			{2, 0, 1},
			{-1, 1, 1},
			{0, 1, 1},
			{1, 1, 1},
			{0, 2, 1},
		},
		Normalize: true,
	}
}

func KernelByName(name string) (Kernel, error) {
	switch name {
	case "floyd-steinberg":
		return FloydSteinberg(), nil
	case "atkinson":
		return Atkinson(), nil
	}
	return Kernel{}, fmt.Errorf("kernel 非法: 未知核 %q", name)
}

func validateKernel(k Kernel) error {
	if len(k.Offsets) == 0 {
		return errors.New("kernel 非法: 偏移表为空")
	}
	sum := 0.0
	for _, o := range k.Offsets {
		dx, dy := o[0], o[1]
		if dy < 0 || (dy == 0 && dx <= 0) {
			return fmt.Errorf("kernel 非法: 偏移 (%v,%v) 不是前向偏移", dx, dy)
		}
		sum += o[2]
	}
	if k.Normalize && sum == 0 {
		return errors.New("kernel 非法: 权重和为 0，无法归一化")
	}
	return nil
}

// WeightTable 把核的权重表打印成文本（原始权重与归一化权重并列），便于逐项核对。
func (k Kernel) WeightTable() string {
	sum := 0.0
	for _, o := range k.Offsets {
		sum += o[2]
	}
	var b strings.Builder
	fmt.Fprintf(&b, "kernel=%s normalize=%v weightSum=%g\n", k.Name, k.Normalize, sum)
	fmt.Fprintf(&b, "%6s %6s %10s %12s\n", "dx", "dy", "raw", "normalized")
	for _, o := range k.Offsets {
		n := o[2]
		if k.Normalize && sum != 0 {
			n = o[2] / sum
		}
		fmt.Fprintf(&b, "%6g %6g %10g %12.6f\n", o[0], o[1], o[2], n)
	}
	return b.String()
}

// Dither 对图像做误差扩散抖动，按 depth 位深量化后仍存回 0..255。
// 扫描按蛇形（serpentine）推进：偶数行从左到右，奇数行从右到左并镜像 dx。
// strength 为扩散强度（0 等价于只做量化）；Normalize 开启时权重先除以权重和。
func Dither(im *raster.Image, k Kernel, strength float64, depth int) (*raster.Image, error) {
	if err := validateKernel(k); err != nil {
		return nil, err
	}
	if strength < 0 {
		return nil, fmt.Errorf("strength 不能为负: %v", strength)
	}
	if depth < 1 || depth > 8 {
		return nil, fmt.Errorf("bitDepth 超出范围 [1,8]: %d", depth)
	}
	sum := 0.0
	for _, o := range k.Offsets {
		sum += o[2]
	}
	norm := 1.0
	if k.Normalize && sum != 0 {
		norm = 1 / sum
	}
	maxLevel := (1 << uint(depth)) - 1
	levels := float64(maxLevel)
	buf := make([]float64, im.W*im.H)
	for i, v := range im.Pix {
		buf[i] = float64(v)
	}
	out, _ := raster.New(im.W, im.H)
	for y := 0; y < im.H; y++ {
		reverse := y%2 == 1
		for xi := 0; xi < im.W; xi++ {
			x := xi
			if reverse {
				x = im.W - 1 - xi
			}
			i := y*im.W + x
			vc := buf[i]
			if vc < 0 {
				vc = 0
			}
			if vc > 255 {
				vc = 255
			}
			q := math.Round(vc/255*levels) * 255 / levels
			out.Pix[i] = uint8(q)
			err := (vc - q) * strength
			for _, o := range k.Offsets {
				dx := int(o[0])
				if reverse {
					dx = -dx
				}
				nx, ny := x+dx, y+int(o[1])
				if nx >= 0 && nx < im.W && ny >= 0 && ny < im.H {
					buf[ny*im.W+nx] += err * o[2] * norm
				}
			}
		}
	}
	return out, nil
}

// Quantize 只做逐像素量化、不做误差扩散，代表“现有实现”的低位深输出。
func Quantize(im *raster.Image, depth int) (*raster.Image, error) {
	if depth < 1 || depth > 8 {
		return nil, fmt.Errorf("bitDepth 超出范围 [1,8]: %d", depth)
	}
	maxLevel := (1 << uint(depth)) - 1
	levels := float64(maxLevel)
	out, _ := raster.New(im.W, im.H)
	for i, v := range im.Pix {
		out.Pix[i] = uint8(math.Round(float64(v)/255*levels) * 255 / levels)
	}
	return out, nil
}

// CountBands 统计环带数：把 4-连通的等值区域用洪水填充划分，
// 面积 >= minArea 像素的区域计为一条环带。
// minArea 的约定取法：一个完整量化色带的面积
// （如 128 宽 4 位深下为 128/16=8 列 x 行高）。仅量化的渐变会出现
// 达到该面积的贯穿色带；误差扩散把等值区域打碎成小于一个色带的小块。
func CountBands(im *raster.Image, minArea int) int {
	if minArea < 1 {
		return 0
	}
	seen := make([]bool, im.W*im.H)
	bands := 0
	stack := make([]int, 0, 64)
	for start := range im.Pix {
		if seen[start] {
			continue
		}
		v := im.Pix[start]
		area := 0
		stack = append(stack[:0], start)
		seen[start] = true
		for len(stack) > 0 {
			cur := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			area++
			x, y := cur%im.W, cur/im.W
			nb := [4]int{cur - 1, cur + 1, cur - im.W, cur + im.W}
			ok := [4]bool{x > 0, x < im.W-1, y > 0, y < im.H-1}
			for k, n := range nb {
				if ok[k] && !seen[n] && im.Pix[n] == v {
					seen[n] = true
					stack = append(stack, n)
				}
			}
		}
		if area >= minArea {
			bands++
		}
	}
	return bands
}

// MeanAbsError 返回两图逐像素绝对误差的均值（0..255 量纲）。
func MeanAbsError(a, b *raster.Image) (float64, error) {
	if a.W != b.W || a.H != b.H {
		return 0, errors.New("尺寸不一致")
	}
	if len(a.Pix) == 0 {
		return 0, nil
	}
	sum := 0.0
	for i := range a.Pix {
		d := float64(a.Pix[i]) - float64(b.Pix[i])
		if d < 0 {
			d = -d
		}
		sum += d
	}
	return sum / float64(len(a.Pix)), nil
}
