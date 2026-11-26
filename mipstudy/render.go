package mipstudy

import "vecgo/raster"

// Texture 是灰度纹理及其 box 平均得到的 mip 链；chain[0] 为原纹理。
type Texture struct {
	Size  int
	Chain [][]uint8 // 每层边长 Size>>level，最小为 1
}

// NewCheckerTexture 生成边长 size 的棋盘格纹理，每个格子 cell 个纹素。
// size 必须是 2 的幂且能被 cell 整除。
func NewCheckerTexture(size, cell int) *Texture {
	pix := make([]uint8, size*size)
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if ((x/cell)+(y/cell))%2 == 0 {
				pix[y*size+x] = 255
			}
		}
	}
	t := &Texture{Size: size, Chain: [][]uint8{pix}}
	for s := size; s > 1; s >>= 1 {
		ns := s >> 1
		prev := t.Chain[len(t.Chain)-1]
		next := make([]uint8, ns*ns)
		for y := 0; y < ns; y++ {
			for x := 0; x < ns; x++ {
				sum := int(prev[(2*y)*s+2*x]) +
					int(prev[(2*y)*s+2*x+1]) +
					int(prev[(2*y+1)*s+2*x]) +
					int(prev[(2*y+1)*s+2*x+1])
				next[y*ns+x] = uint8(sum / 4)
			}
		}
		t.Chain = append(t.Chain, next)
	}
	return t
}

// Sample 在第 level 层做最近邻采样，uv 为纹素坐标（0..size），越界按重复纹理回绕。
func (t *Texture) Sample(level int, u, v float64) uint8 {
	if level < 0 {
		level = 0
	}
	if level >= len(t.Chain) {
		level = len(t.Chain) - 1
	}
	s := t.Size >> level
	layer := t.Chain[level]
	x := int(u) % s
	y := int(v) % s
	if x < 0 {
		x += s
	}
	if y < 0 {
		y += s
	}
	return layer[y*s+x]
}

// MaxLevel 返回可用的最高层号。
func (t *Texture) MaxLevel() int { return len(t.Chain) - 1 }

// RenderBand 把纹理按给定足迹（纹素单位的 a、b，a>=b）缩小采样，输出宽 w、高 h 的一条横带。
// corrected=false 用未修正的面积公式选层，true 用轴比修正后的层号。
func (t *Texture) RenderBand(w, h int, a, b float64, corrected bool) *raster.Image {
	im, _ := raster.New(w, h)
	level := LevelArea(a, b)
	if corrected {
		level = LevelCorrected(a, b)
	}
	k := FloorLevel(level, t.MaxLevel())
	// 每条带都采样纹理的同一区域（0..size 全覆盖），足迹 a、b 只决定选哪一层；
	// 这样三档之间、修正前后的差异纯粹来自层号选择，不含构图差异。
	for y := 0; y < h; y++ {
		v := (float64(y) + 0.5) / float64(h) * float64(t.Size)
		for x := 0; x < w; x++ {
			u := (float64(x) + 0.5) / float64(w) * float64(t.Size)
			im.Pix[y*w+x] = t.Sample(k, u, v)
		}
	}
	return im
}
