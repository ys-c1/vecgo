// gamutdemo 生成三档饱和度（低/中/高）测试图，对每条等明度色相条做
// 原截断路径（Clip）与压缩曲线路径（Compress）的对照，输出：
//   - artifacts/gamut/ 下 9 张 PNG（每档：原图、截断、压缩）
//   - stdout 上的并列统计表（超域像素数、色相偏移上界）
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"

	"vecgo/gamut"
)

const (
	bars   = 12 // 12 个均匀分布的色相方向
	barW   = 96
	imgH   = 64
	lightL = 0.5
)

// levels 是三档饱和度：沿各色相方向允许的最大越界量 y。
// 低档全部域内，中/高档分别有部分、大量超域像素。
var levels = []struct {
	name string
	yMax float64
}{
	{"低饱和", 0.8},
	{"中饱和", 1.6},
	{"高饱和", 2.5},
}

// dirAt 给定平面色相角 theta，取明度 lightL 下该方向的边界点（y=1）。
func dirAt(theta float64) gamut.RGB {
	d := rotateInChromaPlane(math.Cos(theta), math.Sin(theta))
	g := gamut.RGB{R: lightL, G: lightL, B: lightL} // 等明度灰点 (L,L,L)
	// 求该方向边界缩放：c = g + t*d，最先碰到立方体界面的通道给出 t。
	// 用 gamut.Boundary 反解：先构造一点再归一化到 y=1。
	probe := gamut.RGB{
		R: g.R + d.R,
		G: g.G + d.G,
		B: g.B + d.B,
	}
	_, y := gamut.Boundary(probe)
	if y <= 0 {
		return gamut.RGB{}
	}
	t := 1 / y
	return gamut.RGB{
		R: g.R + t*d.R,
		G: g.G + t*d.G,
		B: g.B + t*d.B,
	}
}

// rotateInChromaPlane 在等明度平面 w·d=0 内按 (ct,st) 合成方向向量。
// 标准正交基不导出，故在此独立重算一组（与包内构造一致：
// x 轴种子对归一化灰轴做 Gram-Schmidt，再取叉积）。
func rotateInChromaPlane(ct, st float64) gamut.RGB {
	w := gamut.RGB{R: 0.2126, G: 0.7152, B: 0.0722}
	nw := math.Sqrt(w.R*w.R + w.G*w.G + w.B*w.B)
	u := gamut.RGB{R: w.R / nw, G: w.G / nw, B: w.B / nw}
	seed := gamut.RGB{R: 1, G: 0, B: 0}
	dot := seed.R*u.R + seed.G*u.G + seed.B*u.B
	p1 := gamut.RGB{
		R: seed.R - dot*u.R,
		G: seed.G - dot*u.G,
		B: seed.B - dot*u.B,
	}
	n1 := math.Sqrt(p1.R*p1.R + p1.G*p1.G + p1.B*p1.B)
	p1 = gamut.RGB{R: p1.R / n1, G: p1.G / n1, B: p1.B / n1}
	p2 := gamut.RGB{
		R: u.G*p1.B - u.B*p1.G,
		G: u.B*p1.R - u.R*p1.B,
		B: u.R*p1.G - u.G*p1.R,
	}
	return gamut.RGB{
		R: ct*p1.R + st*p2.R,
		G: ct*p1.G + st*p2.G,
		B: ct*p1.B + st*p2.B,
	}
}

// rampColor 在第 bar 条色相、参数 t∈[0,1] 处生成像素，t 映射到 y∈[0,yMax]。
func rampColor(bar int, t, yMax float64) gamut.RGB {
	theta := 2 * math.Pi * float64(bar) / bars
	boundary := dirAt(theta)
	g := gamut.RGB{R: lightL, G: lightL, B: lightL}
	// boundary = g + d；恢复 d。
	d := gamut.RGB{
		R: boundary.R - g.R,
		G: boundary.G - g.G,
		B: boundary.B - g.B,
	}
	y := t * yMax
	return gamut.RGB{
		R: g.R + y*d.R,
		G: g.G + y*d.G,
		B: g.B + y*d.B,
	}
}

type stats struct {
	total, oogSrc, oogClip, oogComp int
	maxShiftClip, maxShiftComp      float64 // 度，仅对源超域像素统计的 Oklab 色相偏移上界
}

func main() {
	outDir := filepath.Join("artifacts", "gamut")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		panic(err)
	}

	fmt.Println("档位   | 总像素 | 源超域 | 截断后超域 | 压缩后超域 | 截断色相偏移 | 压缩色相偏移")
	fmt.Println("-------|--------|--------|------------|------------|--------------|--------------")

	for _, lv := range levels {
		var st stats
		src := image.NewRGBA(image.Rect(0, 0, bars*barW, imgH))
		clip := image.NewRGBA(src.Rect)
		comp := image.NewRGBA(src.Rect)

		for bar := 0; bar < bars; bar++ {
			for sx := 0; sx < barW; sx++ {
				t := float64(sx) / float64(barW-1)
				c := rampColor(bar, t, lv.yMax)
				cc := gamut.Clip(c)
				cm := gamut.Compress(c)
				st.total++
				if gamut.OutOfGamut(c) {
					st.oogSrc++
					h0 := gamut.OklabDeg(c)
					if d := gamut.HueDistance(h0, gamut.OklabDeg(cc)); d > st.maxShiftClip {
						st.maxShiftClip = d
					}
					if d := gamut.HueDistance(h0, gamut.OklabDeg(cm)); d > st.maxShiftComp {
						st.maxShiftComp = d
					}
				}
				if gamut.OutOfGamut(cc) {
					st.oogClip++
				}
				if gamut.OutOfGamut(cm) {
					st.oogComp++
				}
				setEnc(src, sx, bar, c)
				setEnc(clip, sx, bar, cc)
				setEnc(comp, sx, bar, cm)
			}
		}

		writePNG(filepath.Join(outDir, lv.name+"_源图.png"), src)
		writePNG(filepath.Join(outDir, lv.name+"_截断.png"), clip)
		writePNG(filepath.Join(outDir, lv.name+"_压缩.png"), comp)

		fmt.Printf("%s | %6d | %6d | %10d | %10d | %10.4f° | %10.4f°\n",
			lv.name, st.total, st.oogSrc, st.oogClip, st.oogComp,
			st.maxShiftClip, st.maxShiftComp)
	}
}

func setEnc(img *image.RGBA, sx, bar int, c gamut.RGB) {
	for y := 0; y < imgH; y++ {
		x := bar*barW + sx
		img.SetRGBA(x, y, color.RGBA{
			R: gamut.EncodeSRGB(c.R),
			G: gamut.EncodeSRGB(c.G),
			B: gamut.EncodeSRGB(c.B),
			A: 255,
		})
	}
}

func writePNG(path string, img *image.RGBA) {
	f, err := os.Create(path)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		panic(err)
	}
}
