package raster

import (
	"errors"
	"math"
	"testing"

	"vecgo/geom"
)

func rotPts(poly []geom.Point, deg float64) []geom.Point {
	c, s := math.Cos(deg*math.Pi/180), math.Sin(deg*math.Pi/180)
	out := make([]geom.Point, len(poly))
	for i, p := range poly {
		out[i] = geom.Point{X: p.X*c - p.Y*s, Y: p.X*s + p.Y*c}
	}
	return out
}

var (
	sqStraight = []geom.Point{{X: -40, Y: -40}, {X: 40, Y: -40}, {X: 40, Y: 40}, {X: -40, Y: 40}}
	sqSlant = rotPts(sqStraight, 45)
	// lShape 全轴对齐边，但内部边 x=27、y=-27 与子点网格错位，产生部分覆盖。
	lShape = []geom.Point{
		{X: -40, Y: -40}, {X: 40, Y: -40}, {X: 40, Y: 40},
		{X: 27, Y: 40}, {X: 27, Y: -27}, {X: -40, Y: -27},
	}
	slit   = [][]geom.Point{
		{{X: -50, Y: -40}, {X: -1, Y: -40}, {X: -1, Y: 40}, {X: -50, Y: 40}},
		{{X: 1, Y: -40}, {X: 50, Y: -40}, {X: 50, Y: 40}, {X: 1, Y: 40}},
	}
	corner = []geom.Point{{X: -45, Y: -8}, {X: 45, Y: 0}, {X: -45, Y: 8}}
	solid  = []geom.Point{{X: -50, Y: -50}, {X: 50, Y: -50}, {X: 50, Y: 50}, {X: -50, Y: 50}}
)

func render(t *testing.T, size int, polys [][]geom.Point, bits int, diffuse bool) *Image {
	t.Helper()
	cov, err := Coverage(size, size, polys, 8)
	if err != nil {
		t.Fatal(err)
	}
	im, err := New(size, size)
	if err != nil {
		t.Fatal(err)
	}
	if err := Quantize(cov, im, bits, diffuse); err != nil {
		t.Fatal(err)
	}
	return im
}

func only16Levels(t *testing.T, im *Image) {
	t.Helper()
	for _, v := range im.Pix {
		if v%17 != 0 {
			t.Fatalf("4 位输出出现非 16 级值: %d", v)
		}
	}
}

// ---- 独立错误码 ----

func TestErrBitDepth(t *testing.T) {
	im, _ := New(4, 4)
	cov := make([]float64, 16)
	for _, bad := range []int{0, 1, 2, 16, 32} {
		err := Quantize(cov, im, bad, true)
		if !errors.Is(err, ErrBitDepth) {
			t.Fatalf("bits=%d 应返回 ErrBitDepth，实际 %v", bad, err)
		}
	}
}

func TestErrSubZero(t *testing.T) {
	if _, err := Coverage(4, 4, [][]geom.Point{solid}, 0); !errors.Is(err, ErrSubZero) {
		t.Fatalf("sub=0 应返回 ErrSubZero，实际 %v", err)
	}
	if _, err := Coverage(4, 4, [][]geom.Point{solid}, -3); !errors.Is(err, ErrSubZero) {
		t.Fatalf("sub<0 应返回 ErrSubZero，实际 %v", err)
	}
	im, _ := New(4, 4)
	if err := im.Fill([][]geom.Point{solid}, 0); !errors.Is(err, ErrSubZero) {
		t.Fatalf("Fill sub=0 应返回 ErrSubZero，实际 %v", err)
	}
}

func TestErrEmptyImage(t *testing.T) {
	empty := &Image{}
	cov := make([]float64, 16)
	if err := Quantize(cov, empty, 8, false); !errors.Is(err, ErrEmptyImage) {
		t.Fatalf("空图量化应返回 ErrEmptyImage，实际 %v", err)
	}
	if _, err := Coverage(0, 4, [][]geom.Point{solid}, 8); !errors.Is(err, ErrEmptyImage) {
		t.Fatalf("宽 0 应返回 ErrEmptyImage")
	}
	if err := empty.SavePGM("/tmp/should_not_exist.pgm"); !errors.Is(err, ErrEmptyImage) {
		t.Fatalf("空图导出应返回 ErrEmptyImage，实际 %v", err)
	}
	if err := empty.SavePGM4("/tmp/should_not_exist.pgm"); !errors.Is(err, ErrEmptyImage) {
		t.Fatalf("空图 4 位导出应返回 ErrEmptyImage，实际 %v", err)
	}
	if _, err := empty.Pack4(); !errors.Is(err, ErrEmptyImage) {
		t.Fatalf("空图打包应返回 ErrEmptyImage，实际 %v", err)
	}
}

func TestErrorCodesDistinct(t *testing.T) {
	if errors.Is(ErrBitDepth, ErrSubZero) || errors.Is(ErrSubZero, ErrEmptyImage) ||
		errors.Is(ErrBitDepth, ErrEmptyImage) {
		t.Fatal("三个错误码必须相互独立")
	}
}

// ---- 8 位输出不得变动 ----

// TestEightBitByteIdentical 用历史整数公式 255*hit/den 逐场景比对新分层。
func TestEightBitByteIdentical(t *testing.T) {
	scenes := [][]geom.Point{sqStraight, sqSlant, slit[0], slit[1], corner, solid}
	for _, size := range []int{8, 16, 64} {
		for _, sub := range []int{1, 2, 3, 8} {
			for _, poly := range scenes {
				im, _ := New(size, size)
				if err := im.Fill([][]geom.Point{poly}, sub); err != nil {
					t.Fatal(err)
				}
				cov, err := Coverage(size, size, [][]geom.Point{poly}, sub)
				if err != nil {
					t.Fatal(err)
				}
				got, _ := New(size, size)
				if err := Quantize(cov, got, 8, false); err != nil {
					t.Fatal(err)
				}
				for i := range im.Pix {
					if im.Pix[i] != got.Pix[i] {
						t.Fatalf("8 位输出变动 size=%d sub=%d px=%d 旧=%d 新=%d",
							size, sub, i, im.Pix[i], got.Pix[i])
					}
				}
			}
		}
	}
}

// TestQuantize8MatchesIntegerFormula 穷举 hit/den 组合，证明浮点覆盖率路径
// 与 uint8(255*hit/den) 整数公式对任意子点网格都一致。
func TestQuantize8MatchesIntegerFormula(t *testing.T) {
	for _, sub := range []int{1, 2, 3, 4, 5, 7, 8} {
		den := sub * sub
		for hit := 0; hit <= den; hit++ {
			want := uint8(255 * hit / den)
			im, _ := New(1, 1)
			if err := Quantize([]float64{float64(hit) / float64(den)}, im, 8, false); err != nil {
				t.Fatal(err)
			}
			if im.Pix[0] != want {
				t.Fatalf("sub=%d hit=%d 旧公式=%d 新路径=%d", sub, hit, want, im.Pix[0])
			}
		}
	}
}

// ---- 场景测试：直边 ----

func TestStraightEdge(t *testing.T) {
	ref := render(t, 64, [][]geom.Point{lShape}, 8, false)
	flat := render(t, 64, [][]geom.Point{lShape}, 4, false)
	fs := render(t, 64, [][]geom.Point{lShape}, 4, true)
	only16Levels(t, flat)
	only16Levels(t, fs)
	// 轴对齐边只可能落在整列/整行上：边缘列之外的内芯像素在 8 位下全亮，
	// 两档 4 位也必须保持全亮，扩散不得把纯色内芯打散。
	interiorLit, interiorKept := 0, 0
	for i, v := range ref.Pix {
		if v != 255 {
			continue
		}
		interiorLit++
		if flat.Pix[i] == 255 && fs.Pix[i] == 255 {
			interiorKept++
		}
	}
	if interiorLit == 0 {
		t.Fatal("直边场景应有全亮内芯")
	}
	if interiorKept != interiorLit {
		t.Fatalf("直边内芯被量化破坏: %d/%d 保持全亮", interiorKept, interiorLit)
	}
	// 8 位基准中必须存在部分覆盖像素（直边抗锯齿生效）。
	partial := 0
	for _, v := range ref.Pix {
		if v > 0 && v < 255 {
			partial++
		}
	}
	if partial == 0 {
		t.Fatal("直边场景缺少部分覆盖像素")
	}
	// 关扩散确定性：直边上的部分覆盖列沿边应为同一量化值（无纵向噪声）。
	// 取任意一条边缘列，检查其部分覆盖像素值一致。
	w := 64
	edgeCol := -1
	for col := 0; col < w; col++ {
		v := ref.Pix[col] // 首行
		if v > 0 && v < 255 {
			edgeCol = col
			break
		}
	}
	if edgeCol >= 0 {
		want := flat.Pix[edgeCol]
		for row := 1; row < w; row++ {
			v := ref.Pix[row*w+edgeCol]
			if v > 0 && v < 255 && flat.Pix[row*w+edgeCol] != want {
				t.Fatalf("关扩散直边列 %d 出现纵向抖动: 行0=%d 行%d=%d", edgeCol, want, row, flat.Pix[row*w+edgeCol])
			}
		}
	}
}

// ---- 场景测试：斜边，台阶少四成 ----

func TestSlantEdgeStepsDrop40(t *testing.T) {
	for _, size := range []int{64, 256} {
		ref := render(t, size, [][]geom.Point{sqSlant}, 8, false)
		flat := render(t, size, [][]geom.Point{sqSlant}, 4, false)
		fs := render(t, size, [][]geom.Point{sqSlant}, 4, true)
		only16Levels(t, flat)
		only16Levels(t, fs)
		baseSteps := StaircaseCount(ref)
		fsSteps := StaircaseCount(fs)
		flatSteps := StaircaseCount(flat)
		if baseSteps == 0 {
			t.Fatalf("size=%d 斜边 8 位基准应有台阶", size)
		}
		drop := 1 - float64(fsSteps)/float64(baseSteps)
		if drop < 0.4 {
			t.Fatalf("size=%d 扩散台阶降幅 %.1f%% 未达 40%% (8位=%d 4位关=%d 4位开=%d)",
				size, drop*100, baseSteps, flatSteps, fsSteps)
		}
		// 扩散版相对 8 位的平均误差应优于（或至少不显著劣于）关扩散版。
		maeFlat, _ := MeanAbsError(ref, flat)
		maeFS, _ := MeanAbsError(ref, fs)
		if maeFS > maeFlat*1.25 {
			t.Fatalf("size=%d 扩散误差劣化过大: flat=%.2f fs=%.2f", size, maeFlat, maeFS)
		}
	}
}

// ---- 场景测试：细缝 ----

func TestSlit(t *testing.T) {
	for _, size := range []int{64, 256} {
		ref := render(t, size, slit, 8, false)
		flat := render(t, size, slit, 4, false)
		fs := render(t, size, slit, 4, true)
		// 缝两侧大面积墙体必须保持全亮，扩散不能把墙体内芯打散。
		brightRef, brightFlat, brightFS := 0, 0, 0
		for i := range ref.Pix {
			if ref.Pix[i] == 255 {
				brightRef++
			}
			if flat.Pix[i] == 255 {
				brightFlat++
			}
			if fs.Pix[i] == 255 {
				brightFS++
			}
		}
		if brightRef == 0 {
			t.Fatalf("size=%d 细缝场景墙体未点亮", size)
		}
		if brightFS < brightRef*8/10 {
			t.Fatalf("size=%d 扩散后墙体亮像素损失过大: ref=%d fs=%d", size, brightRef, brightFS)
		}
		if brightFlat < brightRef*8/10 {
			t.Fatalf("size=%d 关扩散后墙体亮像素损失过大: ref=%d flat=%d", size, brightRef, brightFlat)
		}
		// 缝区必须存在非全亮像素（缝没有被完全糊死）。
		hasGap := false
		for _, v := range fs.Pix {
			if v < 255 {
				hasGap = true
				break
			}
		}
		if !hasGap {
			t.Fatalf("size=%d 细缝被完全填平", size)
		}
	}
}

// ---- 场景测试：尖角 ----

func TestSharpCorner(t *testing.T) {
	for _, size := range []int{64, 256} {
		ref := render(t, size, [][]geom.Point{corner}, 8, false)
		flat := render(t, size, [][]geom.Point{corner}, 4, false)
		fs := render(t, size, [][]geom.Point{corner}, 4, true)
		// 尖点附近必须有部分覆盖像素，不能被量化成硬切。
		partial := 0
		for _, v := range ref.Pix {
			if v > 0 && v < 255 {
				partial++
			}
		}
		if partial == 0 {
			t.Fatalf("size=%d 尖角无抗锯齿过渡像素", size)
		}
		// 4 位两档的点亮总量与 8 位基准偏差不超过 5%（尖角不能被吃掉）。
		countOn := func(im *Image) int { return im.CountOn(1) }
		baseOn := countOn(ref)
		for name, im := range map[string]*Image{"flat": flat, "fs": fs} {
			n := countOn(im)
			if math.Abs(float64(n-baseOn))/float64(baseOn) > 0.05 {
				t.Fatalf("size=%d %s 点亮量偏差超 5%%: base=%d got=%d", size, name, baseOn, n)
			}
		}
	}
}

// ---- 场景测试：纯色 ----

func TestSolidColor(t *testing.T) {
	for _, bits := range []int{4, 8} {
		im := render(t, 64, [][]geom.Point{solid}, bits, true)
		for i, v := range im.Pix {
			if v != 255 {
				t.Fatalf("bits=%d 纯色场景 px=%d 值=%d，应全 255", bits, i, v)
			}
		}
	}
}

// ---- 扩散开关：可关闭，关闭为确定性就近取整，附基准值 ----

func TestDiffuseCanDisable(t *testing.T) {
	cov, err := Coverage(96, 96, [][]geom.Point{sqSlant}, 8)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := New(96, 96)
	b, _ := New(96, 96)
	if err := Quantize(cov, a, 4, false); err != nil {
		t.Fatal(err)
	}
	if err := Quantize(cov, b, 4, false); err != nil {
		t.Fatal(err)
	}
	// 关闭后必须确定性：同输入两次运行逐字节相同。
	for i := range a.Pix {
		if a.Pix[i] != b.Pix[i] {
			t.Fatalf("关扩散结果不确定 px=%d", i)
		}
	}
	// 关闭后必须等于独立就近取整基准。
	for i, f := range cov {
		lvl := int(f*15.0 + 0.5)
		if lvl < 0 {
			lvl = 0
		} else if lvl > 15 {
			lvl = 15
		}
		if a.Pix[i] != uint8(lvl*17) {
			t.Fatalf("关扩散 px=%d 偏离就近取整基准: 期望=%d 实际=%d", i, lvl*17, a.Pix[i])
		}
	}
	// 斜边场景开/关必须产生可见差异。
	fs, _ := New(96, 96)
	if err := Quantize(cov, fs, 4, true); err != nil {
		t.Fatal(err)
	}
	diff := 0
	for i := range a.Pix {
		if a.Pix[i] != fs.Pix[i] {
			diff++
		}
	}
	if diff == 0 {
		t.Fatal("斜边场景开扩散与关扩散应存在差异")
	}
}

// ---- 打包与 4 位 PGM ----

func TestPack4Roundtrip(t *testing.T) {
	im := render(t, 64, [][]geom.Point{sqSlant}, 4, true)
	packed, err := im.Pack4()
	if err != nil {
		t.Fatal(err)
	}
	// 原生体积 = 头 + ceil(N/2)。
	headerLen := len("VECGO4\n64 64\n")
	want := headerLen + (64*64+1)/2
	if len(packed) != want {
		t.Fatalf("4 位原生体积异常: %d, 期望 %d", len(packed), want)
	}
	for i, v := range im.Pix {
		var lvl uint8
		if i&1 == 0 {
			lvl = packed[headerLen+i/2] >> 4
		} else {
			lvl = packed[headerLen+i/2] & 0x0f
		}
		if lvl != v/17 {
			t.Fatalf("打包回读错误 px=%d", i)
		}
	}
}
