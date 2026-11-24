package shading

import (
	"errors"
	"math"

	"vecgo/level"
	"vecgo/raster"
)

func Apply(im *raster.Image, mode string, top, bottom float64) error {
	if mode != "flat" && mode != "gradient" {
		return errors.New("未知着色模式")
	}
	if top < 0 || top > 1 || bottom < 0 || bottom > 1 {
		return errors.New("强度超出范围")
	}
	for row := 0; row < im.H; row++ {
		var v uint8
		if mode == "flat" {
			v = uint8(255 * top)
		} else {
			t := top
			if im.H > 1 {
				t = top + (bottom-top)*float64(row)/float64(im.H-1)
			}
			v = uint8(255 * t)
		}
		for col := 0; col < im.W; col++ {
			if im.Pix[row*im.W+col] != 0 {
				im.Pix[row*im.W+col] = v
			}
		}
	}
	return nil
}

// ApplyDist 与 Apply 相同，但渐变参数不再线性映射到灰阶，而是按 level.Dist 分布：
// 先把该行归一化成“由暗到亮”的线性分量 s，再经感知反函数 F(s) 重排。
// dist 为空或 linear 时，数值结果与 Apply 完全一致（pipe 默认仍直接调用 Apply，
// 以保证历史产物逐字节不变）。
func ApplyDist(im *raster.Image, mode string, top, bottom float64, dist string, alpha float64) error {
	if mode != "flat" && mode != "gradient" {
		return errors.New("未知着色模式")
	}
	if top < 0 || top > 1 || bottom < 0 || bottom > 1 {
		return errors.New("强度超出范围")
	}
	d, err := level.Normalize(dist)
	if err != nil {
		return err
	}
	a, err := level.CleanAlpha(alpha)
	if err != nil {
		return err
	}
	span := top - bottom // pipe 固定 top=1 > bottom=0；一般情形也按 top 亮、bottom 暗处理
	for row := 0; row < im.H; row++ {
		var v uint8
		if mode == "flat" {
			v = uint8(255 * top)
		} else {
			q := 0.0
			if im.H > 1 {
				q = float64(row) / float64(im.H-1) // 0=顶行, 1=底行
			}
			lin := top + (bottom-top)*q
			s := 0.0
			if math.Abs(span) > 1e-15 {
				s = (lin - bottom) / span // 0=暗端(bottom), 1=亮端(top)
			}
			if s < 0 {
				s = 0
			} else if s > 1 {
				s = 1
			}
			mapped := s
			switch d {
			case level.Perceptual:
				mapped = math.Pow(s, 1/a)
			case level.Lab:
				mapped = level.LabEncode(level.LfromStar(100 * s))
			}
			t := bottom + span*mapped
			v = uint8(255 * t)
		}
		for col := 0; col < im.W; col++ {
			if im.Pix[row*im.W+col] != 0 {
				im.Pix[row*im.W+col] = v
			}
		}
	}
	return nil
}
