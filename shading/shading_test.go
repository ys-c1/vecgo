package shading

import (
	"math"
	"testing"

	"vecgo/raster"
)

// 未旋转、单位缩放、包围盒即像素域时，矩阵应退化为纯平移的恒等映射。
func TestLocalFrameIdentity(t *testing.T) {
	f := NewLocalFrame(1, 0, 0, 0, 10, 10, 10, 10)
	if math.Abs(f.A-1) > 1e-12 || math.Abs(f.D-1) > 1e-12 ||
		math.Abs(f.B) > 1e-12 || math.Abs(f.C) > 1e-12 ||
		math.Abs(f.E) > 1e-12 || math.Abs(f.F) > 1e-12 {
		t.Fatalf("恒等映射系数错误: %+v", f)
	}
	x, y := f.LocalXY(3, 7)
	if math.Abs(x-3) > 1e-12 || math.Abs(y-7) > 1e-12 {
		t.Fatalf("逆变换错误: (%v,%v)", x, y)
	}
}

// 平移项必须只来自包围盒原点偏移 E=-minx*kx、F=-miny*ky。
func TestLocalFrameTranslationFromBBox(t *testing.T) {
	w, h := 20, 20
	minx, miny := 4.0, -2.0
	maxx, maxy := 14.0, 8.0
	f := NewLocalFrame(1, 0, minx, miny, maxx, maxy, w, h)
	wantE := -minx * float64(w) / (maxx - minx)
	wantF := -miny * float64(h) / (maxy - miny)
	if math.Abs(f.E-wantE) > 1e-12 || math.Abs(f.F-wantF) > 1e-12 {
		t.Fatalf("平移项错误: E=%v 期望%v F=%v 期望%v", f.E, wantE, f.F, wantF)
	}
	// 包围盒原点映射到像素 (0,0)。
	x, y := f.LocalXY(0, 0)
	if math.Abs(x-minx) > 1e-12 || math.Abs(y-miny) > 1e-12 {
		t.Fatalf("包围盒原点反算错误: (%v,%v)", x, y)
	}
}

// 旋转 90° 后局部 Y 轴应与画布 X 轴平行：等 t 线（垂直于渐变方向）
// 与局部 X 轴平行，逆变换保持正交，距离不缩放变形（仅统一缩放）。
func TestLocalFrameRotate90(t *testing.T) {
	f := NewLocalFrame(1, math.Pi/2, -10, -10, 10, 10, 20, 20)
	// 局部 (1,0) 正向映射：c=A, r=C；90° 时 A≈0, C=1（局部X指向画布+Y）。
	if math.Abs(f.A) > 1e-12 || math.Abs(f.C-1) > 1e-12 ||
		math.Abs(f.B+1) > 1e-12 || math.Abs(f.D) > 1e-12 {
		t.Fatalf("90° 旋转系数错误: %+v", f)
	}
	// 逆变换往返一致性。
	for _, p := range [][2]float64{{3, 5}, {0, 0}, {19, 19}, {7, 11}} {
		x, y := f.LocalXY(int(p[0]), int(p[1]))
		c := f.A*x + f.B*y + f.E
		r := f.C*x + f.D*y + f.F
		if math.Abs(c-p[0]) > 1e-10 || math.Abs(r-p[1]) > 1e-10 {
			t.Fatalf("往返不一致: 像素%v -> 局部(%v,%v) -> (%v,%v)", p, x, y, c, r)
		}
	}
}

func TestApplyGradientRejectsRange(t *testing.T) {
	im, _ := raster.New(4, 4)
	f := NewLocalFrame(1, 0, 0, 0, 4, 4, 4, 4)
	if err := ApplyGradient(im, f, 1.2, 0); err == nil {
		t.Fatal("超范围强度应报错")
	}
}
