// ditherreport 生成误差扩散抖动的对照报告：
//   - 打印扩散核权重表（原始权重 + 归一化权重）
//   - 输出与逐像素手算参考图对照（平均误差必须为 0）
//   - 两档位深（4/8 bit）下“现有实现（仅量化） vs 误差扩散”的环带数与平均误差
//   - 未归一化核的对照数据
package main

import (
	"fmt"
	"math"
	"os"

	"vecgo/post"
	"vecgo/raster"
)

// refDither 是逐像素手算的参考实现，刻意写得直白，与正式实现交叉核对。
func refDither(im *raster.Image, k post.Kernel, strength float64, depth int) []uint8 {
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
	out := make([]uint8, im.W*im.H)
	for y := 0; y < im.H; y++ {
		for xi := 0; xi < im.W; xi++ {
			x := xi
			if y%2 == 1 {
				x = im.W - 1 - xi
			}
			vc := buf[y*im.W+x]
			if vc < 0 {
				vc = 0
			}
			if vc > 255 {
				vc = 255
			}
			q := math.Round(vc/255*levels) * 255 / levels
			out[y*im.W+x] = uint8(q)
			e := (vc - q) * strength
			for _, o := range k.Offsets {
				dx := int(o[0])
				if y%2 == 1 {
					dx = -dx
				}
				nx, ny := x+dx, y+int(o[1])
				if nx >= 0 && nx < im.W && ny >= 0 && ny < im.H {
					buf[ny*im.W+nx] += e * o[2] * norm
				}
			}
		}
	}
	return out
}

func gradientImage(w, h int) *raster.Image {
	im, _ := raster.New(w, h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			im.Pix[y*w+x] = uint8(math.Round(255 * float64(x) / float64(w-1)))
		}
	}
	return im
}

func maePix(a []uint8, b []uint8) float64 {
	sum := 0.0
	for i := range a {
		d := float64(a[i]) - float64(b[i])
		if d < 0 {
			d = -d
		}
		sum += d
	}
	return sum / float64(len(a))
}

func main() {
	const W, H = 128, 32
	// 环带阈值：一个完整 4-bit 量化色带的面积（128 宽 / 16 级 = 8 列，x 32 行 = 256 像素）。
	// 等值连通区域达到这个面积才算一条可见环带；抖动打碎的小块不计。
	const minArea = 256
	src := gradientImage(W, H)
	fs := post.FloydSteinberg()

	fmt.Println("== 扩散核权重表 ==")
	fmt.Print(fs.WeightTable())
	fmt.Println()

	fmt.Println("== 与逐像素手算参考图对照 ==")
	for _, depth := range []int{4, 8} {
		d, err := post.Dither(src, fs, 1.0, depth)
		if err != nil {
			fmt.Fprintln(os.Stderr, "抖动失败:", err)
			os.Exit(1)
		}
		ref := refDither(src, fs, 1.0, depth)
		fmt.Printf("depth=%d-bit  对手算参考的平均误差 = %.4f\n", depth, maePix(d.Pix, ref))
	}
	fmt.Println()

	fmt.Println("== 两档位深：现有实现（仅量化） vs 误差扩散 ==")
	for _, depth := range []int{4, 8} {
		quant, _ := post.Quantize(src, depth)
		dith, _ := post.Dither(src, fs, 1.0, depth)
		qb := post.CountBands(quant, minArea)
		db := post.CountBands(dith, minArea)
		qe, _ := post.MeanAbsError(quant, src)
		de, _ := post.MeanAbsError(dith, src)
		fmt.Printf("depth=%d-bit  仅量化: 环带=%2d 平均误差=%.3f | 误差扩散: 环带=%2d 平均误差=%.3f\n",
			depth, qb, qe, db, de)
	}
	fmt.Println()

	fmt.Println("== 未归一化核对照（4-bit） ==")
	raw := post.FloydSteinberg()
	raw.Normalize = false
	fmt.Print(raw.WeightTable())
	u, _ := post.Dither(src, raw, 1.0, 4)
	ub := post.CountBands(u, minArea)
	ue, _ := post.MeanAbsError(u, src)
	d4, _ := post.Dither(src, fs, 1.0, 4)
	d4b := post.CountBands(d4, minArea)
	d4e, _ := post.MeanAbsError(d4, src)
	fmt.Printf("归一化:   环带=%2d 平均误差=%.3f\n", d4b, d4e)
	fmt.Printf("未归一化: 环带=%2d 平均误差=%.3f\n", ub, ue)
}
