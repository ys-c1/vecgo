package raster

import (
	"fmt"
	"os"

	"vecgo/geom"
	"vecgo/param"
)

type Image struct {
	W, H int
	Pix  []uint8
}

// New 建一张空画布。宽高必须落在 [1, 4096]，否则返回 *param.Error。
func New(w, h int) (*Image, error) {
	if err := param.All(
		param.IntIn("raster.New.w", w, param.SideMin, param.SideMax),
		param.IntIn("raster.New.h", h, param.SideMin, param.SideMax),
	); err != nil {
		return nil, err
	}
	return &Image{W: w, H: h, Pix: make([]uint8, w*h)}, nil
}

// ValidateImage 校验画布本身：非 nil、宽高在 [1, 4096]、Pix 长度等于 W*H。
// entry 是调用方入口名（如 "raster.Fill"），错误字段为 <entry>.im 系列。
// 各包（shading、post）都经由它校验画布，保证同一套口径。
func ValidateImage(entry string, im *Image) error {
	if err := param.NotNil(entry+".im", im); err != nil {
		return err
	}
	if err := param.All(
		param.IntIn(entry+".im.W", im.W, param.SideMin, param.SideMax),
		param.IntIn(entry+".im.H", im.H, param.SideMin, param.SideMax),
	); err != nil {
		return err
	}
	if len(im.Pix) != im.W*im.H {
		return param.Invalid(entry+".im.Pix", len(im.Pix),
			fmt.Sprintf("像素数必须等于 W*H=%d", im.W*im.H))
	}
	return nil
}

// Fill 用超采样把一组多边形填进画布。supersample 须在 [1, 8]，polys 不得为空，
// 每个多边形至少三个顶点、坐标有限且包围盒宽高为正；否则返回 *param.Error。
// 非法输入不会被夹取或跳过。
func (im *Image) Fill(polys [][]geom.Point, supersample int) error {
	if err := ValidateImage("raster.Fill", im); err != nil {
		return err
	}
	if err := param.All(
		param.IntIn("raster.Fill.supersample", supersample, param.SupersampleMin, param.SupersampleMax),
		param.AtLeast("raster.Fill.polys", len(polys), param.FillMinPolys),
	); err != nil {
		return err
	}

	shapes := make([]geom.Polygon, len(polys))
	for i, p := range polys {
		field := fmt.Sprintf("raster.Fill.polys[%d]", i)
		poly, err := geom.NewPolygonFor(field, p)
		if err != nil {
			return err
		}
		if err := spanError(field, p); err != nil {
			return err
		}
		shapes[i] = poly
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
	den := supersample * supersample
	for row := 0; row < im.H; row++ {
		for col := 0; col < im.W; col++ {
			hit := 0
			for sj := 0; sj < supersample; sj++ {
				for si := 0; si < supersample; si++ {
					px := minx + (float64(col)+(float64(si)+0.5)/float64(supersample))*spanx/float64(im.W)
					py := miny + (float64(row)+(float64(sj)+0.5)/float64(supersample))*spany/float64(im.H)
					for _, p := range shapes {
						if p.Contains(px, py) {
							hit++
							break
						}
					}
				}
			}
			im.Pix[row*im.W+col] = uint8(255 * hit / den)
		}
	}
	return nil
}

// spanError 拒绝包围盒宽或高不为正的多边形。光栅化需要正的映射范围，
// 旧实现会静默把 0 换成 1，把退化多边形拉伸成整幅图像。
func spanError(field string, p []geom.Point) error {
	minx, miny := p[0].X, p[0].Y
	maxx, maxy := minx, miny
	for _, q := range p[1:] {
		minx = minF(minx, q.X)
		maxx = maxF(maxx, q.X)
		miny = minF(miny, q.Y)
		maxy = maxF(maxy, q.Y)
	}
	if sx := maxx - minx; sx <= 0 {
		return param.Invalid(field, fmt.Sprintf("包围盒宽 %v", sx), "包围盒宽必须为正（不夹取）")
	}
	if sy := maxy - miny; sy <= 0 {
		return param.Invalid(field, fmt.Sprintf("包围盒高 %v", sy), "包围盒高必须为正（不夹取）")
	}
	return nil
}

func minF(a, b float64) float64 {
	if b < a {
		return b
	}
	return a
}

func maxF(a, b float64) float64 {
	if b > a {
		return b
	}
	return a
}

// SavePGM 以 PGM(P5) 导出。路径不能为空，画布宽高须合法，Pix 长度须为 W*H，
// 否则返回 *param.Error。
func (im *Image) SavePGM(path string) error {
	if err := ValidateImage("raster.SavePGM", im); err != nil {
		return err
	}
	if err := param.NonEmpty("raster.SavePGM.path", path); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := fmt.Fprintf(f, "P5\n%d %d\n255\n", im.W, im.H); err != nil {
		return err
	}
	_, err = f.Write(im.Pix)
	return err
}

// CountOn 统计亮度不低于 threshold 的像素数。im 为 nil 或 Pix 长度不符时返回
// *param.Error，不做“按 0 处理”的兜底。
func (im *Image) CountOn(threshold uint8) (int, error) {
	if err := ValidateImage("raster.CountOn", im); err != nil {
		return 0, err
	}
	n := 0
	for _, v := range im.Pix {
		if v >= threshold {
			n++
		}
	}
	return n, nil
}
