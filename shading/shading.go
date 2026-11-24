package shading

import (
	"errors"
	"math"

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

// LocalFrame 是多边形局部坐标 (X,Y) 到像素坐标 (c,r) 的仿射变换，
// 采用两行三列矩阵（六个元素）：
//
//	c = A*X + B*Y + E
//	r = C*X + D*Y + F
//
// 线性部分 (A,B;C,D) 来自局部轴与画布轴的关系：
// 局部 X 轴方向 (1,0) 经 scale、rotate 后在画布上指向
// (scale*cosθ, scale*sinθ)，局部 Y 轴 (0,1) 指向
// (-scale*sinθ, scale*cosθ)；再由光栅化的包围盒归一化
// （画布跨度 spanx/spany 映射到 W/H 个像素）得到各系数。
type LocalFrame struct {
	A, B, E float64 // 像素列 c = A*X + B*Y + E
	C, D, F float64 // 像素行 r = C*X + D*Y + F
}

// NewLocalFrame 由场景变换参数与光栅化包围盒构造局部→像素变换。
// scale 为缩放，rotate 为弧度，bbox 为变换后多边形在画布上的包围盒。
func NewLocalFrame(scale, rotate, minx, miny, maxx, maxy float64, w, h int) LocalFrame {
	cos, sin := math.Cos(rotate), math.Sin(rotate)
	spanx, spany := maxx-minx, maxy-miny
	if spanx == 0 {
		spanx = 1
	}
	if spany == 0 {
		spany = 1
	}
	kx := float64(w) / spanx
	ky := float64(h) / spany
	return LocalFrame{
		A: scale * cos * kx,
		B: -scale * sin * kx,
		C: scale * sin * ky,
		D: scale * cos * ky,
		// 平移项来源：场景变换本身无平移，E、F 完全来自光栅化时
		// 包围盒原点 (minx,miny) 被归一化到像素 (0,0) 的偏移，
		// 即 E = -minx*kx，F = -miny*ky。
		E: -minx * kx,
		F: -miny * ky,
	}
}

// LocalXY 是 (c,r) -> (X,Y) 的逆变换，把像素坐标换算回多边形局部坐标。
func (f LocalFrame) LocalXY(col, row int) (x, y float64) {
	det := f.A*f.D - f.B*f.C
	ic, ir := float64(col)-f.E, float64(row)-f.F
	x = (f.D*ic - f.B*ir) / det
	y = (-f.C*ic + f.A*ir) / det
	return x, y
}

// row0Y / rowLastY 求像素行 0 与末行在局部系中的 Y 坐标，
// 用于把局部 Y 归一化到与旧版“按行渐变”完全相同的区间约定。
func (f LocalFrame) rowY(row int) float64 {
	_, y := f.LocalXY(0, row)
	return y
}

// ApplyGradient 在多边形局部坐标系中采样渐变：
// 渐变参数 t 只随局部 Y 变化，因此等 t 线平行于局部 X 轴，
// 渐变方向（强度变化最快的方向）即局部 Y 轴，垂直于给定轴。
func ApplyGradient(im *raster.Image, frame LocalFrame, top, bottom float64) error {
	if top < 0 || top > 1 || bottom < 0 || bottom > 1 {
		return errors.New("强度超出范围")
	}
	if im.H <= 1 {
		v := uint8(255 * top)
		for i := range im.Pix {
			if im.Pix[i] != 0 {
				im.Pix[i] = v
			}
		}
		return nil
	}
	y0 := frame.rowY(0)
	y1 := frame.rowY(im.H - 1)
	span := y1 - y0
	for row := 0; row < im.H; row++ {
		for col := 0; col < im.W; col++ {
			if im.Pix[row*im.W+col] == 0 {
				continue
			}
			_, y := frame.LocalXY(col, row)
			// 等效行号：把局部 Y 仿射映射回未旋转时的像素行，
			// 再沿用旧版 row/(H-1) 的归一化方式。
			eqRow := (y - y0) / span * float64(im.H-1)
			t := top + (bottom-top)*eqRow/float64(im.H-1)
			if t < 0 {
				t = 0
			} else if t > 1 {
				t = 1
			}
			im.Pix[row*im.W+col] = uint8(255 * t)
		}
	}
	return nil
}
