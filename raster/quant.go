package raster

import (
	"errors"
	"fmt"
	"os"

	"vecgo/geom"
)

// 独立错误码：调用方可用 errors.Is 精确区分三类非法输入。
var (
	// ErrBitDepth 位深非法：仅支持 4 或 8。
	ErrBitDepth = errors.New("位深非法: 仅支持 4 或 8")
	// ErrSubZero 子点为零：子采样数必须为正。
	ErrSubZero = errors.New("子点为零: 子采样数必须为正")
	// ErrEmptyImage 图为空：宽高必须为正且像素缓冲非空。
	ErrEmptyImage = errors.New("图为空: 宽高必须为正")
)

// Coverage 按 sub×sub 子点（抗锯齿网格）计算每个像素的覆盖率，取值 [0,1]。
// sub 即每维子点数：8 表示 8×8=64 子点。
func Coverage(w, h int, polys [][]geom.Point, sub int) ([]float64, error) {
	if w <= 0 || h <= 0 {
		return nil, ErrEmptyImage
	}
	if sub <= 0 {
		return nil, ErrSubZero
	}
	for _, p := range polys {
		if len(p) < 3 {
			return nil, errors.New("多边形至少三个顶点")
		}
	}
	cov := make([]float64, w*h)
	if len(polys) == 0 {
		return cov, nil
	}
	minx, miny := polys[0][0].X, polys[0][0].Y
	maxx, maxy := minx, miny
	for _, p := range polys {
		for _, q := range p {
			if q.X < minx {
				minx = q.X
			}
			if q.X > maxx {
				maxx = q.X
			}
			if q.Y < miny {
				miny = q.Y
			}
			if q.Y > maxy {
				maxy = q.Y
			}
		}
	}
	spanx, spany := maxx-minx, maxy-miny
	if spanx == 0 {
		spanx = 1
	}
	if spany == 0 {
		spany = 1
	}
	den := float64(sub * sub)
	for row := 0; row < h; row++ {
		for col := 0; col < w; col++ {
			hit := 0
			for sj := 0; sj < sub; sj++ {
				for si := 0; si < sub; si++ {
					px := minx + (float64(col)+(float64(si)+0.5)/float64(sub))*spanx/float64(w)
					py := miny + (float64(row)+(float64(sj)+0.5)/float64(sub))*spany/float64(h)
					for _, p := range polys {
						if geom.Inside(p, px, py) {
							hit++
							break
						}
					}
				}
			}
			cov[row*w+col] = float64(hit) / den
		}
	}
	return cov, nil
}

// Quantize 把覆盖率量化进 out.Pix。
// bits=8：逐像素 256 级（与历史 Fill 输出字节级一致，diffuse 被忽略）。
// bits=4：16 级，输出值为 level*17（即 0,17,...,255）；
//
//	diffuse=true 做 Floyd–Steinberg 误差扩散，false 关闭扩散仅做就近取整。
func Quantize(cov []float64, out *Image, bits int, diffuse bool) error {
	if out == nil || out.W <= 0 || out.H <= 0 || len(out.Pix) == 0 {
		return ErrEmptyImage
	}
	if bits != 4 && bits != 8 {
		return ErrBitDepth
	}
	if len(cov) != len(out.Pix) {
		return fmt.Errorf("覆盖率与画幅不匹配: %d 个覆盖率 vs %d 像素", len(cov), len(out.Pix))
	}
	switch bits {
	case 8:
		for i, f := range cov {
			v := int(f * 255.0)
			if v > 255 {
				v = 255
			}
			out.Pix[i] = uint8(v)
		}
	case 4:
		if diffuse {
			quantize4Diffuse(cov, out)
		} else {
			quantize4Plain(cov, out)
		}
	}
	return nil
}

// quantize4Plain 关闭扩散：每个像素独立就近取整到 16 级。
func quantize4Plain(cov []float64, out *Image) {
	for i, f := range cov {
		lvl := int(f*15.0 + 0.5)
		if lvl < 0 {
			lvl = 0
		} else if lvl > 15 {
			lvl = 15
		}
		out.Pix[i] = uint8(lvl * 17)
	}
}

// quantize4Diffuse Floyd–Steinberg 扩散：误差按 7/16、3/16、5/16、1/16
// 分给右、左下、正下、右下。缓冲保留浮点累加，仅输出端夹取到 16 级。
func quantize4Diffuse(cov []float64, out *Image) {
	buf := append([]float64(nil), cov...)
	w, h := out.W, out.H
	const (
		wRight = 7.0 / 16.0
		wBL    = 3.0 / 16.0
		wB     = 5.0 / 16.0
		wBR    = 1.0 / 16.0
	)
	for row := 0; row < h; row++ {
		for col := 0; col < w; col++ {
			i := row*w + col
			lvl := int(buf[i]*15.0 + 0.5)
			if lvl < 0 {
				lvl = 0
			} else if lvl > 15 {
				lvl = 15
			}
			out.Pix[i] = uint8(lvl * 17)
			err := buf[i] - float64(lvl)/15.0
			if col+1 < w {
				buf[i+1] += err * wRight
			}
			if row+1 < h {
				if col > 0 {
					buf[i+w-1] += err * wBL
				}
				buf[i+w] += err * wB
				if col+1 < w {
					buf[i+w+1] += err * wBR
				}
			}
		}
	}
}

// MeanAbsError 以 ref 为基准，返回 cmp 的逐像素平均绝对误差（0..255 量级）。
func MeanAbsError(ref, cmp *Image) (float64, error) {
	if ref == nil || cmp == nil || ref.W != cmp.W || ref.H != cmp.H || len(ref.Pix) != len(cmp.Pix) {
		return 0, ErrEmptyImage
	}
	sum := 0
	for i := range ref.Pix {
		a, b := int(ref.Pix[i]), int(cmp.Pix[i])
		if a > b {
			sum += a - b
		} else {
			sum += b - a
		}
	}
	return float64(sum) / float64(len(ref.Pix)), nil
}

// StaircaseCount 统计台阶数：逐行取该行最靠近图形内芯的部分覆盖像素
// （0<v<255）的量化电平作为该行的边缘代表电平；相邻两行边缘电平相同，
// 即计一个台阶。粗量化使相邻行落在同一电平形成阶梯平台，计数升高；
// 误差扩散打散平台，计数随之下降。边缘中断的行不参与比较。
func StaircaseCount(im *Image) int {
	if im == nil || im.W <= 0 || im.H <= 0 {
		return 0
	}
	steps := 0
	prevOK := false
	prevLevel := 0
	for row := 0; row < im.H; row++ {
		base := row * im.W
		rep := -1
		for col := 0; col < im.W; col++ {
			v := im.Pix[base+col]
			if v > 0 && v < 255 {
				rep = int(v / 17) // 4 位电平；8 位值除 17 后等价按 16 级归并比较
			}
		}
		if rep >= 0 {
			if prevOK && rep == prevLevel {
				steps++
			}
			prevOK, prevLevel = true, rep
		} else {
			prevOK = false
		}
	}
	return steps
}

// SavePGM4 导出 4 位 PGM（P5，maxval=15，每像素 1 字节存放 0..15 电平）。
func (im *Image) SavePGM4(path string) error {
	if im.W == 0 || im.H == 0 {
		return ErrEmptyImage
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := fmt.Fprintf(f, "P5\n%d %d\n15\n", im.W, im.H); err != nil {
		return err
	}
	buf := make([]byte, len(im.Pix))
	for i, v := range im.Pix {
		buf[i] = v / 17
	}
	_, err = f.Write(buf)
	return err
}

// Pack4 把 4 位电平紧密打包（每字节两个 nibble，高 nibble 在前），
// 带 "VECGO4\n宽 高\n" 头，返回完整字节。4 位档的原生体积。
func (im *Image) Pack4() ([]byte, error) {
	if im.W == 0 || im.H == 0 {
		return nil, ErrEmptyImage
	}
	n := im.W * im.H
	header := fmt.Sprintf("VECGO4\n%d %d\n", im.W, im.H)
	payload := make([]byte, (n+1)/2)
	for i, v := range im.Pix {
		lvl := v / 17
		if i&1 == 0 {
			payload[i/2] = lvl << 4
		} else {
			payload[i/2] |= lvl
		}
	}
	return append([]byte(header), payload...), nil
}
