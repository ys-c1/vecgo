package raster

import (
	"errors"
	"math/rand"

	"vecgo/geom"
)

// FillRandom 用每像素 n 个独立均匀的子像素随机点估计覆盖率，灰度量化口径
// 与 Fill 相同（uint8(255*hit/n) 整数截断）。rng 由调用方注入，固定种子
// 即可复现。n 必须为正。
//
// 注意：默认配置不走这条路径，因此默认产物与确定性超采样逐字节相同。
func (im *Image) FillRandom(polys [][]geom.Point, n int, rng *rand.Rand) error {
	if n <= 0 {
		return errors.New("random samples 必须为正")
	}
	if rng == nil {
		return errors.New("rand 源不能为 nil")
	}
	for _, p := range polys {
		if len(p) < 3 {
			return errors.New("多边形至少三个顶点")
		}
	}
	m := NewMapping(polys)
	for row := 0; row < im.H; row++ {
		for col := 0; col < im.W; col++ {
			hit := 0
			for k := 0; k < n; k++ {
				u := float64(col) + rng.Float64()
				v := float64(row) + rng.Float64()
				px, py := m.PixelToWorld(im.W, im.H, u, v)
				for _, p := range polys {
					if geom.Inside(p, px, py) {
						hit++
						break
					}
				}
			}
			im.Pix[row*im.W+col] = uint8(255 * hit / n)
		}
	}
	return nil
}
