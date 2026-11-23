package comp

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math"
	"strings"
	"testing"

	"vecgo/raster"
)

// 三张确定性 4x4 灰度图：水平梯度、垂直梯度、棋盘，均值均为 0.5。
func makeLayers(t *testing.T) []*raster.Image {
	t.Helper()
	mk := func(fill func(x, y int) uint8) *raster.Image {
		im, err := raster.New(4, 4)
		if err != nil {
			t.Fatal(err)
		}
		for y := 0; y < 4; y++ {
			for x := 0; x < 4; x++ {
				im.Pix[y*4+x] = fill(x, y)
			}
		}
		return im
	}
	return []*raster.Image{
		mk(func(x, y int) uint8 { return uint8(x * 85) }), // 0,85,170,255 行重复
		mk(func(x, y int) uint8 { return uint8(y * 85) }), // 列重复
		mk(func(x, y int) uint8 {
			if (x+y)%2 == 0 {
				return 255
			}
			return 0
		}), // 棋盘
	}
}

// targetFor 按图层均值 s̄ 与期望权重 w 构造目标亮度。
func targetFor(mode string, srcMean, w float64) float64 {
	switch mode {
	case ModeMultiply:
		return 1 - w*(1-srcMean)
	default:
		return w * srcMean
	}
}

func stack(t *testing.T, mode string, weights []float64) []Layer {
	t.Helper()
	imgs := makeLayers(t)
	layers := make([]Layer, len(weights))
	for i, w := range weights {
		mean := MeanLuminance(imgs[i])
		layers[i] = Layer{
			Image:  imgs[i],
			Mode:   mode,
			Target: targetFor(mode, mean, w),
		}
	}
	return layers
}

func pixHash(im *raster.Image) string {
	sum := sha256.Sum256(im.Pix)
	return hex.EncodeToString(sum[:])
}

var allModes = []string{ModeNormal, ModeMultiply, ModeAdd, ModeScreen}

// 用例取四种模式 × {全透明, 全不透明, 部分权重}。
func TestFourModesAndOpacityEndpoints(t *testing.T) {
	cases := map[string][]float64{
		"全透明":  {0, 0, 0},
		"全不透明": {1, 1, 1},
		"部分权重": {0.25, 0.5, 0.75},
	}
	for _, mode := range allModes {
		for name, weights := range cases {
			layers := stack(t, mode, weights)
			out, err := Composite(layers)
			if err != nil {
				t.Fatalf("%s/%s 合成失败: %v", mode, name, err)
			}
			if len(out.Pix) != 16 {
				t.Fatalf("%s/%s 输出尺寸错误", mode, name)
			}
			// 全透明栈：normal 无覆盖输出 0；multiply 连乘幺元为 1（白底透显）；
			// add/screen 幺元为 0。逐模式核对幺元输出。
			if name == "全透明" {
				for i, p := range out.Pix {
					var want uint8
					if mode == ModeMultiply {
						want = 255
					}
					if p != want {
						t.Fatalf("%s/全透明 像素 %d = %d，应为 %d", mode, i, p, want)
					}
				}
			}
		}
	}
}

// 同一对图层连续合成两次必须一致：给两次运行的字节哈希。
func TestIdempotentByteHash(t *testing.T) {
	for _, mode := range allModes {
		layers := stack(t, mode, []float64{0.4, 0.6})
		first, err := Composite(layers)
		if err != nil {
			t.Fatal(err)
		}
		second, err := Composite(layers)
		if err != nil {
			t.Fatal(err)
		}
		h1, h2 := pixHash(first), pixHash(second)
		t.Logf("幂等哈希 %-8s run1=%s run2=%s", mode, h1, h2)
		if h1 != h2 {
			t.Fatalf("%s 两次合成哈希不一致", mode)
		}
		if !bytes.Equal(first.Pix, second.Pix) {
			t.Fatalf("%s 两次合成字节不一致", mode)
		}
		// 对输出再合成一次自身（同样的图层对语义），确定性纯函数仍应一致。
		third, err := Composite(layers)
		if err != nil || pixHash(third) != h1 {
			t.Fatalf("%s 第三次合成漂移", mode)
		}
	}
}

// 结合律：三组顺序（左折叠、右折叠、幺元起步）对照，哈希必须全等。
func TestAssociativityThreeOrders(t *testing.T) {
	for _, mode := range allModes {
		layers := stack(t, mode, []float64{0.3, 0.6, 0.9})
		results := make([]*raster.Image, 3)
		hashes := make([]string, 3)
		names := []string{"左折叠(左结合)", "右折叠(右结合)", "幺元起步折叠"}
		for k, kind := range []FoldKind{FoldLeft, FoldRight, FoldIdentity} {
			out, err := CompositeFold(layers, kind)
			if err != nil {
				t.Fatalf("%s %s 失败: %v", mode, names[k], err)
			}
			results[k] = out
			hashes[k] = pixHash(out)
			t.Logf("结合律 %-8s %s hash=%s", mode, names[k], hashes[k])
		}
		if hashes[0] != hashes[1] || hashes[1] != hashes[2] {
			t.Fatalf("%s 三组顺序哈希不一致: %v", mode, hashes)
		}
		// 逐字节再确认一次，避免哈希碰撞掩盖差异。
		for i := 1; i < 3; i++ {
			if !bytes.Equal(results[0].Pix, results[i].Pix) {
				t.Fatalf("%s 与 %s 字节不一致", names[0], names[i])
			}
		}
	}
}

// multiply/add/screen 的合成算子同时满足交换律，图层轮换后三种排列也应对照。
// normal 是 source-over，顺序敏感，不在此列。
func TestCommutativeModesReordering(t *testing.T) {
	for _, mode := range []string{ModeMultiply, ModeAdd, ModeScreen} {
		base := stack(t, mode, []float64{0.3, 0.6, 0.9})
		perms := [][]int{{0, 1, 2}, {2, 0, 1}, {1, 2, 0}}
		var hashes []string
		for _, p := range perms {
			perm := []Layer{base[p[0]], base[p[1]], base[p[2]]}
			out, err := Composite(perm)
			if err != nil {
				t.Fatal(err)
			}
			hashes = append(hashes, pixHash(out))
		}
		t.Logf("交换律 %-8s 三种排列哈希: %v", mode, hashes)
		if hashes[0] != hashes[1] || hashes[1] != hashes[2] {
			t.Fatalf("%s 图层轮换后结果漂移", mode)
		}
	}
}

// 单图层路径不变：不进入两图层混合，输出等于权重烘焙后的单图层项。
func TestSingleLayerPathUnchanged(t *testing.T) {
	imgs := makeLayers(t)
	for _, mode := range allModes {
		for _, w := range []float64{0, 0.5, 1} {
			mean := MeanLuminance(imgs[0])
			layers := []Layer{{Image: imgs[0], Mode: mode, Target: targetFor(mode, mean, w)}}
			out, err := Composite(layers)
			if err != nil {
				t.Fatal(err)
			}
			for i, p := range imgs[0].Pix {
				s := float64(p) / 255.0
				var term float64
				switch mode {
				case ModeNormal, ModeAdd:
					// normal 输出预乘色 w·s；add 项同为 w·s。
					term = w * s
				case ModeScreen:
					// 库路径：单项 Q=1-ws，输出 1-Q（两趟浮点舍入，容忍 1 级）。
					term = 1 - (1 - w*s)
				case ModeMultiply:
					term = 1 - w*(1-s)
				}
				want := toByte(term)
				if diff := int(out.Pix[i]) - int(want); diff > 1 || diff < -1 {
					t.Fatalf("%s 单图层 w=%.2f 像素 %d = %d，应接近 %d", mode, w, i, out.Pix[i], want)
				}
			}
		}
	}
}

// 独立验证：在 0..255 整数域上按右括号逐对量化合成，与库里浮点规范路径
// 完全独立实现，两者最终字节必须相等（差不超过 1 个量化级，吸收钳制语义）。
func TestAssociativityIndependentIntegerReference(t *testing.T) {
	weights := []float64{0.3, 0.6, 0.9}
	for _, mode := range allModes {
		layers := stack(t, mode, weights)
		got, err := CompositeFold(layers, FoldRight)
		if err != nil {
			t.Fatal(err)
		}
		// 先把每个图层烘焙成量化项（与库同公式，但用整数算术）。
		terms := make([][]uint8, len(layers))
		alphas := make([]int, len(layers))
		for k, l := range layers {
			wk, err := SolveWeight(mode, l.Target, MeanLuminance(l.Image))
			if err != nil {
				t.Fatal(err)
			}
			alphas[k] = int(wk*255 + 0.5)
			terms[k] = make([]uint8, len(l.Image.Pix))
			for i, p := range l.Image.Pix {
				s := float64(p) / 255.0
				switch mode {
				case ModeNormal, ModeAdd:
					terms[k][i] = toByte(wk * s)
				case ModeScreen:
					terms[k][i] = toByte(1 - (1 - wk*s))
				case ModeMultiply:
					terms[k][i] = toByte(1 - wk*(1-s))
				}
			}
		}
		// 右括号：t1 ⊕ (t2 ⊕ t3)，整数域逐对算子。
		ref := integerRightFold(mode, terms, alphas)
		for i := range got.Pix {
			if d := int(got.Pix[i]) - int(ref[i]); d > 1 || d < -1 {
				t.Fatalf("%s 像素 %d 库结果 %d 与整数右折叠参考 %d 差超过 1 级", mode, i, got.Pix[i], ref[i])
			}
		}
	}
}

// integerRightFold 在 0..255 整数域按右括号结合，纯整数算术。
// normal 维护 (预乘色, 覆盖率) 两个通道，与库的 source-over 幺半群同构。
func integerRightFold(mode string, terms [][]uint8, alpha255 []int) []uint8 {
	n := len(terms[0])
	type state struct{ cov, alpha []int }
	var fold func(k int) state
	fold = func(k int) state {
		if k == len(terms)-1 {
			cov := append([]int{}, intsOf(terms[k])...)
			alpha := make([]int, n)
			for i := range alpha {
				alpha[i] = alpha255[k]
			}
			return state{cov, alpha}
		}
		right := fold(k + 1)
		top := terms[k]
		out := state{cov: make([]int, n), alpha: make([]int, n)}
		for i := 0; i < n; i++ {
			a := alpha255[k] // 顶层覆盖率 0..255
			termC := int(top[i])
			switch mode {
			case ModeNormal:
				// cov' = termC + cov·(255-a)/255；alpha' 同理。
				out.cov[i] = termC + right.cov[i]*(255-a)/255
				out.alpha[i] = a + right.alpha[i]*(255-a)/255
				if out.cov[i] > 255 {
					out.cov[i] = 255
				}
			case ModeMultiply:
				out.cov[i] = termC * right.cov[i] / 255
			case ModeAdd:
				v := termC + right.cov[i]
				if v > 255 {
					v = 255
				}
				out.cov[i] = v
			case ModeScreen:
				out.cov[i] = 255 - (255-termC)*(255-right.cov[i])/255
			}
		}
		if mode != ModeNormal {
			out.alpha = right.alpha // 其余模式不使用覆盖率通道
		}
		return out
	}
	final := fold(0)
	out := make([]uint8, n)
	for i := range out {
		v := final.cov[i]
		if v > 255 {
			v = 255
		}
		out[i] = uint8(v)
	}
	return out
}

func intsOf(p []uint8) []int {
	out := make([]int, len(p))
	for i, v := range p {
		out[i] = int(v)
	}
	return out
}

// 三类报错各报出图层号：模式非法、透明度越界、图层为空。
func TestErrorsReportLayerNumber(t *testing.T) {
	imgs := makeLayers(t)
	base := func() []Layer {
		return []Layer{
			{Image: imgs[0], Mode: ModeNormal, Target: 0.25},
			{Image: imgs[1], Mode: ModeNormal, Target: 0.25},
		}
	}

	// 模式非法，报错指向图层 2。
	badMode := base()
	badMode[1].Mode = "overlay"
	if _, err := Composite(badMode); err == nil || !strings.Contains(err.Error(), "图层 2") {
		t.Fatalf("模式非法应报图层 2，实际: %v", err)
	} else {
		t.Logf("模式非法: %v", err)
	}

	// 目标亮度本身越界，报错指向图层 2。
	badTarget := base()
	badTarget[1].Target = 1.5
	if _, err := Composite(badTarget); err == nil || !strings.Contains(err.Error(), "图层 2") {
		t.Fatalf("目标越界应报图层 2，实际: %v", err)
	} else {
		t.Logf("目标越界: %v", err)
	}

	// 目标可达性失败：normal 黑底上 T > s̄ 时反解 w>1，报错指向图层 1。
	badWeight := []Layer{
		{Image: imgs[0], Mode: ModeNormal, Target: 0.9}, // s̄=0.5，w=1.8 不可达
		{Image: imgs[1], Mode: ModeNormal, Target: 0.25},
	}
	if _, err := Composite(badWeight); err == nil || !strings.Contains(err.Error(), "图层 1") {
		t.Fatalf("透明度越界应报图层 1，实际: %v", err)
	} else if !strings.Contains(err.Error(), "透明度越界") {
		t.Fatalf("错误类型应为透明度越界，实际: %v", err)
	} else {
		t.Logf("透明度越界: %v", err)
	}

	// multiply 白底上目标过低同样不可达。
	badMul := []Layer{
		{Image: imgs[0], Mode: ModeMultiply, Target: 0.1}, // s̄=0.5，w=1.8 不可达
	}
	if _, err := Composite(badMul); err == nil || !strings.Contains(err.Error(), "图层 1") {
		t.Fatalf("multiply 透明度越界应报图层 1，实际: %v", err)
	}

	// 图层为空（nil / 零尺寸），报错带图层号。
	empty := base()
	empty[0].Image = nil
	if _, err := Composite(empty); err == nil || !strings.Contains(err.Error(), "图层 1") {
		t.Fatalf("空图层应报图层 1，实际: %v", err)
	} else {
		t.Logf("图层为空: %v", err)
	}

	zero, _ := raster.New(2, 2)
	zero.Pix = nil
	empty2 := base()
	empty2[1].Image = zero
	if _, err := Composite(empty2); err == nil || !strings.Contains(err.Error(), "图层 2") {
		t.Fatalf("空像素应报图层 2，实际: %v", err)
	}

	// 整栈为空也按图层 1 报。
	if _, err := Composite(nil); err == nil || !strings.Contains(err.Error(), "图层 1") {
		t.Fatalf("空栈应报图层 1，实际: %v", err)
	}
}

func TestSolveWeightEndpoints(t *testing.T) {
	// 全透明 w=0、全不透明 w=1 的反解闭环。
	for _, mode := range allModes {
		src := 0.5
		for _, w := range []float64{0, 1} {
			tgt := targetFor(mode, src, w)
			got, err := SolveWeight(mode, tgt, src)
			if err != nil {
				t.Fatal(err)
			}
			if math.Abs(got-w) > 1e-12 {
				t.Fatalf("%s w=%.1f 反解得 %v", mode, w, got)
			}
		}
		if _, err := SolveWeight(mode, -0.01, src); !errors.Is(err, errTargetRange) {
			t.Fatalf("%s 负目标应报目标越界", mode)
		}
	}
	if _, err := SolveWeight("bogus", 0.5, 0.5); !errors.Is(err, errIllegalMode) {
		t.Fatal("非法模式应报错")
	}
}

// 收尾：反解权重表，256 档，不可达为 NaN。
func TestWeightTable(t *testing.T) {
	// 规范源 s̄=192/255：
	//   黑底三模式 w=T/s̄，可达 T∈[0,192/255]：t=0 得 w=0，t=192 恰得 w=1；
	//   multiply 白底 w=(1-T)/(1-s̄)，连续可达 T∈[63.75/255,1]，量化后
	//   t=63（63/255）仍超界为 NaN，t=64 起可达，t=255 得 w=0。
	type spec struct {
		feasibleLo, feasibleHi int
		loWeight, hiWeight     float64
		nanBelow, nanAbove     bool
	}
	expect := map[string]spec{
		ModeNormal:   {feasibleLo: 0, feasibleHi: 192, loWeight: 0, hiWeight: 1, nanAbove: true},
		ModeAdd:      {feasibleLo: 0, feasibleHi: 192, loWeight: 0, hiWeight: 1, nanAbove: true},
		ModeScreen:   {feasibleLo: 0, feasibleHi: 192, loWeight: 0, hiWeight: 1, nanAbove: true},
		ModeMultiply: {feasibleLo: 64, feasibleHi: 255, loWeight: 252.0 / 255.0, hiWeight: 0, nanBelow: true},
	}
	for _, mode := range allModes {
		table, err := WeightTable(mode)
		if err != nil {
			t.Fatal(err)
		}
		exp := expect[mode]
		if math.Abs(table[exp.feasibleLo]-exp.loWeight) > 1e-12 {
			t.Fatalf("%s 档位 %d 应得 w=%v，实际 %v", mode, exp.feasibleLo, exp.loWeight, table[exp.feasibleLo])
		}
		if math.Abs(table[exp.feasibleHi]-exp.hiWeight) > 1e-12 {
			t.Fatalf("%s 档位 %d 应得 w=%v，实际 %v", mode, exp.feasibleHi, exp.hiWeight, table[exp.feasibleHi])
		}
		if exp.nanAbove && !math.IsNaN(table[exp.feasibleHi+1]) {
			t.Fatalf("%s 档位 %d 应为 NaN，实际 %v", mode, exp.feasibleHi+1, table[exp.feasibleHi+1])
		}
		if exp.nanBelow && !math.IsNaN(table[exp.feasibleLo-1]) {
			t.Fatalf("%s 档位 %d 应为 NaN，实际 %v", mode, exp.feasibleLo-1, table[exp.feasibleLo-1])
		}
		t.Logf("%-8s 权重表抽样 w(t=0)=%s w(t=63)=%s w(t=192)=%s w(t=255)=%s",
			mode, naAsText(table[0]), naAsText(table[63]), naAsText(table[192]), naAsText(table[255]))
	}
}

func naAsText(v float64) string {
	if math.IsNaN(v) {
		return "NaN(不可达)"
	}
	return trimText(v)
}

func trimText(v float64) string {
	return strings.TrimRight(strings.TrimRight(format4(v), "0"), ".")
}

func format4(v float64) string {
	b := make([]byte, 0, 8)
	whole := int(v)
	frac := v - float64(whole)
	digits := []byte{'0', '1', '2', '3', '4', '5', '6', '7', '8', '9'}
	b = append(b, digits[whole], '.')
	for i := 0; i < 4; i++ {
		frac *= 10
		d := int(frac + 1e-9)
		if d > 9 {
			d = 9
		}
		b = append(b, digits[d])
		frac -= float64(d)
	}
	return string(b)
}
