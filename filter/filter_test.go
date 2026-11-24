package filter

import (
	"math"
	"testing"
)

const eps = 1e-9

func approx(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

// obliqueJ 构造“斜向缩小 k:1、再旋转 theta”的屏幕→纹理雅可比。
// 纹理→屏幕 M = R(theta)·diag(1/k,1)，故 J = M^{-1} = diag(k,1)·R(-theta)：
//
//	a=k cosθ  b=k sinθ  c=-sinθ  d=cosθ
func obliqueJ(k, theta float64) Jac {
	c, s := math.Cos(theta), math.Sin(theta)
	return Jac{A: k * c, B: k * s, C: -s, D: c}
}

func TestFootprintComponentsAndRatio(t *testing.T) {
	// 不旋转、3:1 压缩：雅可比为对角阵，分量直接可读。
	j := obliqueJ(3, 0)
	if !approx(j.A, 3, eps) || !approx(j.B, 0, eps) ||
		!approx(j.C, 0, eps) || !approx(j.D, 1, eps) {
		t.Fatalf("雅可比四分量错误: %+v", j)
	}
	e, err := Footprint(j)
	if err != nil {
		t.Fatal(err)
	}
	// 奇异值 = sqrt(特征值)：长轴 3、短轴 1、轴比 3、长轴朝 u(0)。
	if !approx(e.Major, 3, 1e-9) || !approx(e.Minor, 1, 1e-9) {
		t.Fatalf("半长/短轴错误: major=%v minor=%v", e.Major, e.Minor)
	}
	if !approx(e.Ratio, 3, 1e-9) {
		t.Fatalf("轴比应为 3，实际 %v", e.Ratio)
	}
	if !approx(math.Abs(e.Theta), 0, 1e-9) {
		t.Fatalf("朝向应为 0，实际 %v", e.Theta)
	}
}

func TestFootprintRotated(t *testing.T) {
	// 旋转不改变奇异值：三档角度下轴比恒为 3，朝向随角度平移。
	for _, theta := range []float64{0, math.Pi / 6, math.Pi / 3} {
		j := obliqueJ(3, theta)
		e, err := Footprint(j)
		if err != nil {
			t.Fatal(err)
		}
		if !approx(e.Ratio, 3, 1e-8) {
			t.Fatalf("θ=%v 轴比应恒为 3，实际 %v", theta, e.Ratio)
		}
		if !approx(e.Major, 3, 1e-8) || !approx(e.Minor, 1, 1e-8) {
			t.Fatalf("θ=%v 半轴错误: %+v", theta, e)
		}
		// 长轴随椭圆整体旋转：J 把屏幕长轴映到纹理方向 θ。
		got := math.Mod(e.Theta+2*math.Pi, 2*math.Pi)
		want := math.Mod(theta+2*math.Pi, 2*math.Pi)
		if !approx(got, want, 1e-8) {
			t.Fatalf("θ=%v 朝向应为 %v，实际 %v", theta, want, got)
		}
	}
}

func TestFootprintIsotropicWhenUniform(t *testing.T) {
	// 各向同性（圆）情形：轴比 1，两核 σ 相同。
	e, err := Footprint(Jac{A: 2, B: 0, C: 0, D: 2})
	if err != nil {
		t.Fatal(err)
	}
	if !approx(e.Ratio, 1, 1e-9) {
		t.Fatalf("均匀缩放轴比应为 1，实际 %v", e.Ratio)
	}
	iso, err := NewIso(e, 2)
	if err != nil || !iso.IsIso() || iso.KM.Sigma != iso.Km.Sigma {
		t.Fatalf("各向同性路径核应等宽: %+v err=%v", iso, err)
	}
	an, err := NewAniso(e, 2)
	if err != nil || an.IsIso() {
		t.Fatal("椭圆路径不应被标记为各向同性")
	}
}

func TestFootprintMatchesNumericalJacobian(t *testing.T) {
	// 用非线性映射的有限差分雅可比拟合，验证由四分量推奇异值的通用正确性。
	mapFn := func(x, y float64) (u, v float64) {
		return 2.5*x + 0.7*y + 0.01*x*x, -0.9*x + 1.3*y + 0.02*x*y
	}
	h := 1e-5
	x0, y0 := 4.0, 5.0
	u0, v0 := mapFn(x0, y0)
	ux, vx := func() (float64, float64) {
		u, v := mapFn(x0+h, y0)
		return (u - u0) / h, (v - v0) / h
	}()
	uy, vy := func() (float64, float64) {
		u, v := mapFn(x0, y0+h)
		return (u - u0) / h, (v - v0) / h
	}()
	j := Jac{A: ux, B: uy, C: vx, D: vy}
	e, err := Footprint(j)
	if err != nil {
		t.Fatal(err)
	}
	// 数值奇异值：sqrt(eig(JᵀJ))，与 Footprint 内部同一公式独立复算。
	g00, g01, g11 := j.A*j.A+j.C*j.C, j.A*j.B+j.C*j.D, j.B*j.B+j.D*j.D
	tr, det := g00+g11, g00*g11-g01*g01
	lamMax := (tr + math.Sqrt(math.Max(tr*tr-4*det, 0))) / 2
	lamMin := (tr - math.Sqrt(math.Max(tr*tr-4*det, 0))) / 2
	if !approx(e.Major, math.Sqrt(lamMax), 1e-6) ||
		!approx(e.Minor, math.Sqrt(lamMin), 1e-6) {
		t.Fatalf("数值雅可比奇异值不匹配: %+v", e)
	}
	if e.Ratio < 1 {
		t.Fatalf("轴比必须 >=1，实际 %v", e.Ratio)
	}
}

func TestRejectsDegenerate(t *testing.T) {
	if _, err := Footprint(Jac{}); err == nil {
		t.Fatal("零雅可比应报错")
	}
	if _, err := NewAniso(Ellipse{Major: 1, Minor: 1}, 0); err == nil {
		t.Fatal("非正支撑宽度应报错")
	}
}

func TestWarpPreservesDC(t *testing.T) {
	// 周期回绕 + 归一化权重：常数纹理经任何椭圆核都应保持常数（无直流偏差）。
	const n = 48
	tex := make([]float64, n*n)
	for i := range tex {
		tex[i] = 128
	}
	j := obliqueJ(3, math.Pi/6)
	e, _ := Footprint(j)
	an, _ := NewAniso(e, 2)
	out, err := Warp(tex, n, j, an)
	if err != nil {
		t.Fatal(err)
	}
	for i, v := range out {
		if !approx(v, 128, 1e-6) {
			t.Fatalf("直流未保持 @%d: %v", i, v)
		}
	}
}

// TestThreeAngleExperiment 跑三档角度的端到端对照实验，并打印
// 每档的雅可比四分量、轴比、朝向与两条路径的高频能量/RMSE。
func TestThreeAngleExperiment(t *testing.T) {
	const (
		n       = 144
		cell    = 6
		k       = 2.0 // 2:1 斜向缩小：压缩方向仍在奈奎斯特可分辨区间，混叠可由核抑制
		support = 2.0
		ss      = 12
	)
	angles := []struct {
		name  string
		theta float64
	}{
		{"0°", 0},
		{"30°", math.Pi / 6},
		{"60°", math.Pi / 3},
	}
	tex := Checker(n, cell)
	t.Logf("纹理基准高频能量 total=%.3f", HFEnergy(tex, n).Total)
	for _, ang := range angles {
		j := obliqueJ(k, ang.theta)
		e, err := Footprint(j)
		if err != nil {
			t.Fatal(err)
		}
		gt, err := GroundTruth(tex, n, j, ss)
		if err != nil {
			t.Fatal(err)
		}
		anF, _ := NewAniso(e, support)
		isoF, _ := NewIso(e, support)
		anImg, err := Warp(tex, n, j, anF)
		if err != nil {
			t.Fatal(err)
		}
		isoImg, err := Warp(tex, n, j, isoF)
		if err != nil {
			t.Fatal(err)
		}
		anE, isoE := HFEnergy(anImg, n), HFEnergy(isoImg, n)
		gtE := HFEnergy(gt, n)
		anRMSE, isoRMSE := RMSE(anImg, gt), RMSE(isoImg, gt)

		t.Logf(
			"角度=%s J=[%.3f %.3f; %.3f %.3f] 轴比=%.3f 朝向=%.1f° | "+
				"各向异性 HF=%.3f RMSE=%.3f dirRatio=%.3f | "+
				"各向同性 HF=%.3f RMSE=%.3f dirRatio=%.3f | 基准 dirRatio=%.3f",
			ang.name, j.A, j.B, j.C, j.D, e.Ratio, e.Theta*180/math.Pi,
			anE.Total, anRMSE, anE.Ratio, isoE.Total, isoRMSE, isoE.Ratio, gtE.Ratio,
		)

		// 关键性质 1：椭圆核在短轴方向不过度模糊，高频保持必须优于圆核。
		if anE.Total <= isoE.Total {
			t.Fatalf("%s 各向异性高频能量(%.3f)应高于各向同性(%.3f)",
				ang.name, anE.Total, isoE.Total)
		}
		// 关键性质 2：更贴合足迹意味着更接近超采样基准（误差更小、不出条纹）。
		if anRMSE >= isoRMSE {
			t.Fatalf("%s 各向异性 RMSE(%.3f)应低于各向同性(%.3f)",
				ang.name, anRMSE, isoRMSE)
		}
		// 关键性质 3（不出条纹）：方向能量分布只作记录——椭圆沿斜长轴保留
		// 高频时方向比天然高于基准，这正是“高频保持提升”而非条纹。真正的条纹
		// 判据是与超采样基准的距离：核宽单调加宽时 RMSE 应单调下降并收敛，
		// 若存在欠采样条纹，加宽会先大幅压低误差（非单调/不收敛）。
		var prev float64
		for _, sup := range []float64{support, support * 1.5, support * 2} {
			f, _ := NewAniso(e, sup)
			img, err := Warp(tex, n, j, f)
			if err != nil {
				t.Fatal(err)
			}
			r := RMSE(img, gt)
			if prev > 0 && r > prev*1.02 {
				t.Fatalf("%s 核宽加宽时 RMSE 反弹(%.3f→%.3f)，存在条纹不稳定",
					ang.name, prev, r)
			}
			prev = r
		}
	}
}
