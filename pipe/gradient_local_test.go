package pipe

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"vecgo/config"
	"vecgo/geom"
	"vecgo/raster"
)

func gradientScene() [][]geom.Point {
	return [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}}
}

// 未旋转（rotate=0）的渐变产物必须与改造前金标准逐字节相同。
func TestGradientRotationZeroByteIdentical(t *testing.T) {
	golden, err := os.ReadFile(filepath.Join("testdata", "gradient_rot0_32.pgm"))
	if err != nil {
		t.Fatal(err)
	}
	header := []byte("P5\n32 32\n255\n")
	want := golden[len(header):]

	cfg := config.Default()
	cfg.Width, cfg.Height = 32, 32
	cfg.Shading = "gradient"
	im, err := Run(gradientScene(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(im.Pix) != len(want) {
		t.Fatalf("像素数不一致: %d vs %d", len(im.Pix), len(want))
	}
	for i, v := range im.Pix {
		if v != want[i] {
			t.Fatalf("第 %d 字节不一致: %d vs %d", i, v, want[i])
		}
	}
}

// gradientAxisAngle 用最小二乘把亮像素强度拟合成平面 v=a+bx*x+by*y，
// 返回等强度线法线（即渐变方向）在画布坐标系中的朝向角（度）。
func gradientAxisAngle(t *testing.T, im *raster.Image, polys [][]geom.Point, scale, rotate float64) float64 {
	t.Helper()
	moved := make([][]geom.Point, len(polys))
	for i, poly := range polys {
		moved[i] = Transform(poly, scale, rotate)
	}
	minx, miny, maxx, maxy := raster.BBox(moved)
	spanx, spany := maxx-minx, maxy-miny
	if spanx == 0 {
		spanx = 1
	}
	if spany == 0 {
		spany = 1
	}
	// 正规方程累加量。
	var sx, sy, sv, sxx, syy, sxy, sxv, syv float64
	n := 0.0
	for row := 0; row < im.H; row++ {
		for col := 0; col < im.W; col++ {
			v := im.Pix[row*im.W+col]
			if v == 0 {
				continue
			}
			cx := minx + (float64(col)+0.5)*spanx/float64(im.W)
			cy := miny + (float64(row)+0.5)*spany/float64(im.H)
			fv := float64(v)
			sx += cx
			sy += cy
			sv += fv
			sxx += cx * cx
			syy += cy * cy
			sxy += cx * cy
			sxv += cx * fv
			syv += cy * fv
			n++
		}
	}
	if n < 3 {
		t.Fatal("亮像素不足，无法拟合")
	}
	// 中心化后解 2x2 正规方程。
	mx, my := sx/n, sy/n
	sxx -= sx * mx
	syy -= sy * my
	sxy -= sx * my
	sxv -= sv * mx
	syv -= sv * my
	det := sxx*syy - sxy*sxy
	bx := (sxv*syy - syv*sxy) / det
	by := (syv*sxx - sxv*sxy) / det
	return math.Atan2(by, bx) * 180 / math.Pi
}

// 两轴（无向直线）之间的夹角，结果落在 [0,90] 度。
func axisAngleDeg(a, b float64) float64 {
	d := math.Abs(a - b)
	for d > 180 {
		d -= 180
	}
	if d > 90 {
		d = 180 - d
	}
	return d
}

// 两档旋转角各验一次：实测渐变方向应与局部 Y 轴（垂直于给定的局部 X 轴）一致。
func TestGradientFollowsLocalAxisAtTwoRotations(t *testing.T) {
	angles := []float64{30, 45}
	for _, deg := range angles {
		theta := deg * math.Pi / 180
		cfg := config.Default()
		cfg.Width, cfg.Height = 64, 64
		cfg.Shading = "gradient"
		cfg.Rotate = theta
		im, err := Run(gradientScene(), cfg)
		if err != nil {
			t.Fatal(err)
		}
		measured := gradientAxisAngle(t, im, gradientScene(), cfg.Scale, theta)
		// 期望方向：局部 +Y 经旋转后在画布上为 (-sinθ, cosθ)。
		expected := math.Atan2(math.Cos(theta), -math.Sin(theta)) * 180 / math.Pi
		dev := axisAngleDeg(measured, expected)
		t.Logf("旋转 %.0f°: 实测方向=%.2f° 期望=%.2f° 偏差=%.2f 度", deg, measured, expected, dev)
		if dev > 1.0 {
			t.Fatalf("旋转 %.0f° 渐变方向偏差过大: %.2f 度", deg, dev)
		}

		// 对照：旧实现按画布行采样，方向恒为画布 Y 轴（90°），
		// 其偏差恰好等于旋转角，证明本测试确实能抓住原缺陷。
		legacyDev := axisAngleDeg(90, expected)
		if math.Abs(legacyDev-deg) > 1e-9 {
			t.Fatalf("旧实现偏差对照异常: %.2f vs %.2f", legacyDev, deg)
		}
	}
}
