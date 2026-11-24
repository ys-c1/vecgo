package gamut

import (
	"math"
	"testing"
)

// dirColor 在给定明度 L、平面色相角 theta、归一化越界量 y 处构造像素：
// 先取该方向上的边界点（y=1），再沿等明度方向按 y 缩放偏离向量。
func dirColor(L, theta, y float64) RGB {
	d := RGB{
		R: e1.R*math.Cos(theta) + e2.R*math.Sin(theta),
		G: e1.G*math.Cos(theta) + e2.G*math.Sin(theta),
		B: e1.B*math.Cos(theta) + e2.B*math.Sin(theta),
	}
	probe := add(scale(grayOne, L), d)
	_, yb := Boundary(probe)
	if yb <= 0 {
		return scale(grayOne, L)
	}
	// yb=1/yboundary，边界点 = g + d/yb；目标点 = g + y*(边界点-g)。
	return add(scale(grayOne, L), scale(d, y/yb))
}

func TestOrthonormalBasis(t *testing.T) {
	un := math.Sqrt(dot(lumaW, lumaW))
	u := scale(lumaW, 1/un)
	if got := math.Abs(dot(u, u) - 1); got > 1e-12 {
		t.Fatalf("灰轴单位向量错误: %v", got)
	}
	for name, v := range map[string]RGB{"e1": e1, "e2": e2} {
		if got := math.Abs(dot(v, v) - 1); got > 1e-12 {
			t.Errorf("%s 非单位: %v", name, got)
		}
		if got := math.Abs(dot(v, u)); got > 1e-12 {
			t.Errorf("%s 与灰轴不正交: %v", name, got)
		}
		if got := math.Abs(dot(v, lumaW)); got > 1e-12 {
			t.Errorf("%s 不满足等明度约束 w·v=0: %v", name, got)
		}
	}
	if got := math.Abs(dot(e1, e2)); got > 1e-12 {
		t.Errorf("e1,e2 不正交: %v", got)
	}
}

func TestBoundaryKnown(t *testing.T) {
	// 纯红 (1.6,0,0)：L=0.2126*1.6，d=(1.25984,-0.34016,-0.34016)。
	// 上界面（红=1）先碰到：yb=(1-L)/dR，归一化 y=1/yb。
	c := RGB{R: 1.6, G: 0, B: 0}
	L, y := Boundary(c)
	if math.Abs(L-0.2126*1.6) > 1e-12 {
		t.Errorf("Luma 错误: got %v", L)
	}
	dR := 1.6 - L
	wantY := dR / (1 - L)
	if math.Abs(y-wantY) > 1e-12 || y <= 1 {
		t.Errorf("Boundary 错误: got %v want %v", y, wantY)
	}
	// 域内像素 y<=1。
	if _, yi := Boundary(RGB{R: 0.2, G: 0.4, B: 0.7}); yi > 1 {
		t.Errorf("域内像素被判超域: y=%v", yi)
	}
	// 灰像素 y=0。
	if _, yg := Boundary(RGB{R: 0.5, G: 0.5, B: 0.5}); yg != 0 {
		t.Errorf("灰像素 y 应为 0: got %v", yg)
	}
}

func TestCurveContinuityAndBoundedness(t *testing.T) {
	// 恒等段 R(y)=y、h=1。
	if R, h := curve(y0); R != y0 || h != 1 {
		t.Error("拐点处值错误")
	}
	if R, h := curve(0.5); R != 0.5 || h != 1 {
		t.Error("恒等段错误")
	}
	// y=y0 处 C1：越界侧 h(y0)=1 且导数为 0（与恒等段同值同导）。
	const eps = 1e-9
	_, hR := curve(y0 + eps)
	if math.Abs(hR-1) > 1e-7 {
		t.Errorf("y0 处值不连续: h=%v", hR)
	}
	if d := (hR - 1) / eps; math.Abs(d) > 1e-5 {
		t.Errorf("y0 处导数不为 0: %v", d)
	}
	// 对一切有限 y，R(y)<=A<1（浮点容差），且 R 单调递增、以 A 为渐近上界。
	if math.Abs((y0+w)-A) > 1e-12 {
		t.Fatalf("参数约束 y0+w=A 不成立: %v", y0+w)
	}
	prev := 0.0
	for k := 0; k <= 600; k++ {
		y := y0 + float64(k)*0.05
		R, _ := curve(y)
		if R > A+1e-15 {
			t.Fatalf("压缩后越过渐近线: y=%v R=%v A=%v", y, R, A)
		}
		if R < prev-1e-12 {
			t.Fatalf("R 非单调: y=%v %v<%v", y, R, prev)
		}
		prev = R
	}
	// 远场趋近 A（指数逼近，用 1e-3 容差）。
	Rinf, _ := curve(100)
	if math.Abs(Rinf-A) > 1e-3 {
		t.Errorf("远场渐近错误: got %v want %v", Rinf, A)
	}
}

func TestCompressZeroOutOfGamut(t *testing.T) {
	for li := 1; li < 20; li++ {
		L := float64(li) / 21
		for k := 0; k < 360; k += 15 {
			theta := float64(k) * math.Pi / 180
			for _, over := range []float64{1.01, 1.2, 1.5, 2.0, 3.0, 5.0} {
				c := dirColor(L, theta, over)
				if !OutOfGamut(c) && over > 1 {
					t.Fatalf("测试构造失败: L=%v theta=%d 未超域", L, k)
				}
				got := Compress(c)
				if OutOfGamut(got) {
					t.Fatalf("压缩后仍超域: L=%v theta=%d over=%v -> %v", L, k, over, got)
				}
				for _, ch := range [3]float64{got.R, got.G, got.B} {
					if ch < -1e-12 {
						t.Fatalf("出现负通道: %v", got)
					}
				}
			}
		}
	}
}

func TestCompressPreservesLumaAndHue(t *testing.T) {
	for li := 1; li < 20; li++ {
		L := float64(li) / 21
		for k := 0; k < 360; k += 10 {
			theta := float64(k) * math.Pi / 180
			for _, over := range []float64{1.1, 1.5, 2.5, 4.0} {
				c := dirColor(L, theta, over)
				got := Compress(c)
				if d := math.Abs(Luma(got) - L); d > 1e-10 {
					t.Fatalf("明度被改变: %v", d)
				}
				if d := HueDistance(HueDeg(got), HueDeg(c)); d > 1e-9 {
					t.Fatalf("工作空间色相偏移: %v 度", d)
				}
			}
		}
	}
}

func TestIdentityInsideKnee(t *testing.T) {
	// y<=y0 的像素必须原样返回（h=1，浮点上逐位相等）。
	for li := 1; li < 20; li++ {
		L := float64(li) / 21
		for k := 0; k < 360; k += 30 {
			c := dirColor(L, float64(k)*math.Pi/180, 0.5*y0)
			got := Compress(c)
			if math.Abs(got.R-c.R)+math.Abs(got.G-c.G)+math.Abs(got.B-c.B) > 1e-15 {
				t.Fatalf("恒等段像素被修改: %v -> %v", c, got)
			}
		}
	}
}

func TestClipPathRetained(t *testing.T) {
	c := RGB{R: 1.6, G: -0.2, B: 0.5}
	got := Clip(c)
	if got != (RGB{R: 1, G: 0, B: 0.5}) {
		t.Fatalf("Clip 路径行为错误: %v", got)
	}
	if OutOfGamut(got) {
		t.Fatal("Clip 输出仍超域")
	}
}

// TestClipFlattensCompressionDoesNot 直接对照两条路径在超域区的层次：
// 截断把越界通道钉成常数（死色），压缩必须保证每个通道关于输入严格单调。
func TestClipFlattensCompressionDoesNot(t *testing.T) {
	L := 0.5
	// 取 e1 方向，其边界点偏离向量 bd 同时含正、负分量。
	probe := add(scale(grayOne, L), e1)
	_, yb := Boundary(probe)
	bd := scale(e1, 1/yb)

	const n = 500
	const yMax = 2.5
	var prevClip, prevComp RGB
	flatClip := [3]int{}
	monoComp := [3]bool{true, true, true}
	for k := 0; k <= n; k++ {
		y := 1 + (yMax-1)*float64(k)/n
		c := add(scale(grayOne, L), scale(bd, y))
		cc, cm := Clip(c), Compress(c)
		if k > 0 {
			cv, pv := [3]float64{cc.R, cc.G, cc.B}, [3]float64{prevClip.R, prevClip.G, prevClip.B}
			mv, mp := [3]float64{cm.R, cm.G, cm.B}, [3]float64{prevComp.R, prevComp.G, prevComp.B}
			bv := [3]float64{bd.R, bd.G, bd.B}
			for i := 0; i < 3; i++ {
				if cv[i] == pv[i] {
					flatClip[i]++
				}
				if bv[i] > 0 && mv[i] <= mp[i] {
					monoComp[i] = false
				}
				if bv[i] < 0 && mv[i] >= mp[i] {
					monoComp[i] = false
				}
			}
		}
		prevClip, prevComp = cc, cm
	}
	// 正分量通道在截断路径上应全程平坦（首点恰在边界 y=1，不计）。
	positive := 0
	for i := 0; i < 3; i++ {
		if [3]float64{bd.R, bd.G, bd.B}[i] > 0 {
			positive++
			if flatClip[i] < n-1 {
				t.Errorf("截断未把越界通道 %d 钉平: 平坦 %d/%d", i, flatClip[i], n)
			}
		}
		if !monoComp[i] {
			t.Errorf("压缩通道 %d 不严格单调，层次丢失", i)
		}
	}
	if positive == 0 {
		t.Fatal("测试构造错误：边界方向无正分量")
	}
}

func TestHueDistanceWraparound(t *testing.T) {
	if d := HueDistance(179, -179); math.Abs(d-2) > 1e-9 {
		t.Errorf("跨零点夹角错误: %v", d)
	}
	if d := HueDistance(10, 200); math.Abs(d-170) > 1e-9 {
		t.Errorf("夹角错误: %v", d)
	}
}
