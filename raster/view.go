package raster

import (
	"errors"

	"vecgo/geom"
)

// View 是固定的「世界坐标 → 像素」视口。
//
// 与 Image.Fill 每次按多边形包围盒重新归一化不同，View 不随场景缩放变化：
// 同一个 View 下，缩放多边形会让边在固定的像素网格上真实移动，因此可以
// 跨缩放档考察同一条边的覆盖率/灰度。
//
// 像素 (col,row) 的子样本中心映射为：
//
//	worldX = MinX + (col + (si+0.5)/Supersample) / PPU
//	worldY = MinY + (row + (sj+0.5)/Supersample) / PPU
//
// PPU（pixels per world unit）必须为正；像素为正方形。
type View struct {
	MinX, MinY float64
	PPU        float64
}

func (v View) worldX(sampleCol float64) float64 { return v.MinX + sampleCol/v.PPU }
func (v View) worldY(sampleRow float64) float64 { return v.MinY + sampleRow/v.PPU }

// WorldX / WorldY 把连续的采样本坐标（如 col+0.5）映射到世界坐标，
// 供审计代码计算边与扫描线的交点，保证与采样循环同一映射口径。
func (v View) WorldX(sampleCol float64) float64 { return v.worldX(sampleCol) }
func (v View) WorldY(sampleRow float64) float64 { return v.worldY(sampleRow) }

// CountView 与 FillView 使用完全相同的确定性网格采样，区别是返回每个像素
// 命中的子样本数 hits（范围 0..den，den=supersample²）而不是 uint8 灰度，
// 便于审计覆盖率与取整误差。
func (im *Image) CountView(polys [][]geom.Point, supersample int, v View) (hits []int, den int, err error) {
	if supersample < 1 {
		return nil, 0, errors.New("supersample 必须为正")
	}
	if v.PPU <= 0 {
		return nil, 0, errors.New("view PPU 必须为正")
	}
	for _, p := range polys {
		if len(p) < 3 {
			return nil, 0, errors.New("多边形至少三个顶点")
		}
	}
	den = supersample * supersample
	hits = make([]int, im.W*im.H)
	for row := 0; row < im.H; row++ {
		for col := 0; col < im.W; col++ {
			n := 0
			for sj := 0; sj < supersample; sj++ {
				for si := 0; si < supersample; si++ {
					sx := float64(col) + (float64(si)+0.5)/float64(supersample)
					sy := float64(row) + (float64(sj)+0.5)/float64(supersample)
					px := v.worldX(sx)
					py := v.worldY(sy)
					for _, p := range polys {
						if geom.Inside(p, px, py) {
							n++
							break
						}
					}
				}
			}
			hits[row*im.W+col] = n
		}
	}
	return hits, den, nil
}

// FillView 用固定 View 采样并写入 uint8 灰度，量化口径与 Fill 完全一致：
//
//	Pix = uint8(255 * hit / supersample²)
func (im *Image) FillView(polys [][]geom.Point, supersample int, v View) error {
	hits, den, err := im.CountView(polys, supersample, v)
	if err != nil {
		return err
	}
	for i, h := range hits {
		im.Pix[i] = uint8(255 * h / den)
	}
	return nil
}
