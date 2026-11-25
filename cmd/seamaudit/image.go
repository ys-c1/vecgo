package main

import (
	"image"
	"image/color"
	"image/png"
	"os"

	"vecgo/seam"
)

// 迷你 3×5 点阵字体，只覆盖本图用到的字符。
var font3x5 = map[byte][5]string{
	'0': {"111", "101", "101", "101", "111"},
	'1': {"010", "110", "010", "010", "111"},
	'2': {"111", "001", "111", "100", "111"},
	'3': {"111", "001", "111", "001", "111"},
	'4': {"101", "101", "111", "001", "001"},
	'5': {"111", "100", "111", "001", "111"},
	'6': {"111", "100", "111", "101", "111"},
	'7': {"111", "001", "010", "010", "010"},
	'8': {"111", "101", "111", "101", "111"},
	'9': {"111", "101", "111", "001", "111"},
	'A': {"010", "101", "111", "101", "101"},
	'B': {"110", "101", "110", "101", "110"},
	'C': {"111", "100", "100", "100", "111"},
	'D': {"110", "101", "101", "101", "110"},
	'E': {"111", "100", "110", "100", "111"},
	'F': {"111", "100", "110", "100", "100"},
	'G': {"111", "100", "101", "101", "111"},
	'H': {"101", "101", "111", "101", "101"},
	'I': {"111", "010", "010", "010", "111"},
	'K': {"101", "110", "100", "110", "101"},
	'L': {"100", "100", "100", "100", "111"},
	'M': {"101", "111", "111", "101", "101"},
	'N': {"110", "101", "101", "101", "101"},
	'O': {"111", "101", "101", "101", "111"},
	'P': {"111", "101", "111", "100", "100"},
	'Q': {"111", "101", "101", "111", "011"},
	'R': {"110", "101", "110", "101", "101"},
	'S': {"111", "100", "111", "001", "111"},
	'T': {"111", "010", "010", "010", "010"},
	'U': {"101", "101", "101", "101", "111"},
	'V': {"101", "101", "101", "101", "010"},
	'W': {"101", "101", "111", "111", "101"},
	'X': {"101", "101", "010", "010", "101"},
	'Y': {"101", "101", "010", "010", "010"},
	'+': {"000", "010", "111", "010", "000"},
	'-': {"000", "000", "111", "000", "000"},
	'=': {"000", "111", "000", "111", "000"},
	':': {"000", "010", "000", "010", "000"},
	'.': {"000", "000", "000", "000", "010"},
	'/': {"001", "001", "010", "100", "100"},
	'[': {"110", "100", "100", "100", "110"},
	']': {"011", "001", "001", "001", "011"},
	'(': {"010", "100", "100", "100", "010"},
	')': {"010", "001", "001", "001", "010"},
	',': {"000", "000", "000", "010", "100"},
	'<': {"001", "010", "100", "010", "001"},
	'>': {"100", "010", "001", "010", "100"},
	' ': {"000", "000", "000", "000", "000"},
}

func drawText(img *image.RGBA, s string, x, y, scale int, c color.RGBA) int {
	cx := x
	for i := 0; i < len(s); i++ {
		g, ok := font3x5[s[i]]
		if !ok {
			g = font3x5[' ']
		}
		for r := 0; r < 5; r++ {
			for col := 0; col < 3; col++ {
				if g[r][col] == '1' {
					fillRect(img, cx+col*scale, y+r*scale, scale, scale, c)
				}
			}
		}
		cx += 4 * scale
	}
	return cx - x
}

func textWidth(s string, scale int) int { return len(s) * 4 * scale }

func fillRect(img *image.RGBA, x, y, w, h int, c color.RGBA) {
	b := img.Bounds()
	for j := y; j < y+h; j++ {
		for i := x; i < x+w; i++ {
			if i < b.Min.X || i >= b.Max.X || j < b.Min.Y || j >= b.Max.Y {
				continue
			}
			img.SetRGBA(i, j, c)
		}
	}
}

// accumulation 按“每归属一次叠一档 tier 灰度”合成两块。
// 返回逐像素累计灰度（截断到 255），与灰度台账口径一致。
func accumulation(b seam.Band, l, r seam.Span, tier int) *image.Gray {
	img := image.NewGray(image.Rect(0, 0, b.W, b.H))
	for y := 0; y < b.H; y++ {
		for x := 0; x < b.W; x++ {
			v := 0
			if b.Covered(x, y) {
				if l.Has(x) {
					v += tier
				}
				if r.Has(x) {
					v += tier
				}
			}
			if v > 255 {
				v = 255
			}
			img.SetGray(x, y, color.Gray{Y: uint8(v)})
		}
	}
	return img
}

// scalePanel 把原窗口最近邻放大 z 倍，加像素网格；markerCol>=0 时给该列描彩色框。
func scalePanel(src *image.Gray, x0, x1, y0, y1, z int, markerCol int, marker color.RGBA) *image.RGBA {
	w, h := (x1-x0)*z, (y1-y0)*z
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	grid := color.RGBA{R: 0, G: 0, B: 0, A: 255}
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			g := src.GrayAt(x, y).Y
			c := color.RGBA{R: g, G: g, B: g, A: 255}
			fillRect(out, (x-x0)*z, (y-y0)*z, z, z, c)
		}
	}
	// 像素网格线。
	for x := 0; x <= x1-x0; x++ {
		fillRect(out, x*z, 0, 1, h, grid)
	}
	for y := 0; y <= y1-y0; y++ {
		fillRect(out, 0, y*z, w, 1, grid)
	}
	if markerCol >= x0 && markerCol < x1 {
		mx := (markerCol - x0) * z
		th := 3
		fillRect(out, mx+2, 2, z-4, th, marker)      // 上
		fillRect(out, mx+2, h-2-th, z-4, th, marker) // 下
		fillRect(out, mx+2, 2, th, h-4, marker)      // 左
		fillRect(out, mx+z-2-th, 2, th, h-4, marker) // 右
	}
	return out
}

func savePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

// paste 把 panel 贴到 dst 的 (dx,dy)。
func paste(dst *image.RGBA, panel image.Image, dx, dy int) {
	b := panel.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			dst.Set(dx+x-b.Min.X, dy+y-b.Min.Y, panel.At(x, y))
		}
	}
}
