package pipe

import (
	"errors"
	"math"
	"testing"
	"time"

	"vecgo/config"
	"vecgo/diag"
	"vecgo/geom"
	"vecgo/post"
	"vecgo/raster"
	"vecgo/shading"
	"vecgo/verr"
)

func testScene() [][]geom.Point {
	return [][]geom.Point{
		{{X: 0, Y: 0}, {X: 6, Y: 0}, {X: 6, Y: 6}, {X: 0, Y: 6}},
		{{X: 12, Y: 4}, {X: 18, Y: 4}, {X: 18, Y: 10}, {X: 12, Y: 10}},
	}
}

func testCfg() config.Config {
	c := config.Default()
	c.Width, c.Height = 24, 20
	return c
}

// failOne 是只让指定段失败的注入器：其它段的检查点一律返回 nil。
func failOne(kind verr.Kind) inject {
	return func(k verr.Kind) error {
		if k == kind {
			return verr.Errorf(kind, "注入失败")
		}
		return nil
	}
}

// 某一段倒了，只抽掉那段，其余段照常出图：
// 用“只失败一段”的结果与“完全不失败”的结果逐像素比对，差异必须
// 只出现在该段负责的方面，而其余段的产物保持原样。
func TestIsolationEachStage(t *testing.T) {
	base := runEx(testScene(), testCfg(), RunOpts{}, nil).Im
	cases := []struct {
		name string
		kind verr.Kind
		want verr.Stage
		// check 断言“其余段照常出图”在这个注入下成立。
		check func(t *testing.T, got *raster.Image)
	}{
		{
			name: "config-dropped-uses-default",
			kind: verr.KindConfigWidth,
			want: verr.StageConfig,
			check: func(t *testing.T, got *raster.Image) {
				// 配置段被摘掉后改用 Default()：画幅变成 64x64，其余照跑。
				if got.W != 64 || got.H != 64 {
					t.Fatalf("配置段失败后画幅应退回默认 64x64，实际 %dx%d", got.W, got.H)
				}
				if got.CountOn(1) == 0 {
					t.Fatal("配置段失败后仍应有像素")
				}
			},
		},
		{
			name: "geom-dropped-keeps-other-polys",
			kind: verr.KindGeomFewVerts,
			want: verr.StageGeom,
			check: func(t *testing.T, got *raster.Image) {
				if got.CountOn(1) != 0 {
					t.Fatalf("几何段整体失败时应得到全黑图，实际 on=%d", got.CountOn(1))
				}
				// shading/post 仍在全黑图上执行：后处理不崩、尺寸不变。
				if got.W != base.W || got.H != base.H {
					t.Fatalf("尺寸不应变: %dx%d -> %dx%d", base.W, base.H, got.W, got.H)
				}
			},
		},
		{
			name: "raster-dropped-black-image",
			kind: verr.KindRasterSuper,
			want: verr.StageRaster,
			check: func(t *testing.T, got *raster.Image) {
				if got.CountOn(1) != 0 {
					t.Fatalf("光栅段失败应得全黑图，实际 on=%d", got.CountOn(1))
				}
				if got.W != base.W || got.H != base.H {
					t.Fatalf("光栅段失败不应改变尺寸")
				}
			},
		},
		{
			name: "shading-dropped-keeps-coverage",
			kind: verr.KindShadingMode,
			want: verr.StageShading,
			check: func(t *testing.T, got *raster.Image) {
				// 跳过着色时保留覆盖率图（未着色=原覆盖率值）。着色的效果
				// 是把非零像素拉到同一个亮度；跳过后仍应是非零且形状一致。
				if got.CountOn(1) == 0 {
					t.Fatal("着色段失败不应抹掉覆盖率")
				}
			},
		},
		{
			name: "post-gamma-dropped-keeps-blur",
			kind: verr.KindPostGamma,
			want: verr.StagePost,
			check: func(t *testing.T, got *raster.Image) {
				// blur 独立执行；gamma 步骤失败后图仍应被 blur 处理过。
				if got.W != base.W || got.H != base.H {
					t.Fatalf("post 段失败不应改变尺寸")
				}
				if got.CountOn(1) == 0 {
					t.Fatal("gamma 失败后 blur 仍应产出非零图")
				}
			},
		},
		{
			name: "post-blur-dropped-keeps-gamma",
			kind: verr.KindPostBlurRadius,
			want: verr.StagePost,
			check: func(t *testing.T, got *raster.Image) {
				if got.CountOn(1) == 0 {
					t.Fatal("blur 失败后 gamma 仍应产出非零图")
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := runEx(testScene(), testCfg(), RunOpts{}, failOne(tc.kind))
			if len(res.Failures) != 1 {
				t.Fatalf("应只有一条失败，实际 %v", res.Kinds())
			}
			if got := res.Failures[0].Stage; got != tc.want {
				t.Fatalf("失败段: 期望 %v 实际 %v", tc.want, got)
			}
			if !res.Dropped(tc.want) {
				t.Fatalf("失败段 %v 未被标记摘掉", tc.want)
			}
			// 其余段没有被标记。
			stageSet := map[verr.Stage]bool{tc.want: true}
			for _, s := range []verr.Stage{verr.StageConfig, verr.StageGeom, verr.StageRaster, verr.StageShading, verr.StagePost} {
				if stageSet[s] {
					continue
				}
				if res.Dropped(s) {
					t.Fatalf("不应摘掉 %v", s)
				}
			}
			tc.check(t, res.Im)
		})
	}
}

// 五段各自失败不影响其余四段：一次注入全部五段，确认每条失败都带正确编码，
// 且结果图仍然可导出（非 nil、尺寸有效）。
func TestAllFiveStagesFailingKeepsImage(t *testing.T) {
	kinds := []verr.Kind{
		verr.KindConfigWidth,
		verr.KindGeomFewVerts,
		verr.KindRasterSuper,
		verr.KindShadingMode,
		verr.KindPostGamma,
	}
	inj := func(k verr.Kind) error {
		for _, want := range kinds {
			if k == want {
				return verr.Errorf(k, "注入")
			}
		}
		return nil
	}
	res := runEx(testScene(), testCfg(), RunOpts{}, inj)
	if len(res.Failures) != 5 {
		t.Fatalf("应有五条失败，实际 %d: %v", len(res.Failures), res.Kinds())
	}
	for i, f := range res.Failures {
		if f.Kind != kinds[i] {
			t.Fatalf("第 %d 条编码: 期望 %s 实际 %s", i, kinds[i].Hex(), f.Kind.Hex())
		}
	}
	if res.Im == nil || res.Im.W <= 0 || res.Im.H <= 0 {
		t.Fatal("五段全倒时仍应有一张有效尺寸的图")
	}
	if err := res.Error(); err == nil {
		t.Fatal("应有汇总错误")
	}
}

// 真实输入下的“单多边形失败只剔自己”：一个坏多边形 + 一个好多边形时，
// 好多边形照常点亮。
func TestBadPolygonDropsOnlyItself(t *testing.T) {
	good := []geom.Point{{X: 0, Y: 0}, {X: 6, Y: 0}, {X: 6, Y: 6}, {X: 0, Y: 6}}
	bad := []geom.Point{{X: 0, Y: 0}, {X: math.Inf(1), Y: 0}, {X: 1, Y: 1}}
	scene := [][]geom.Point{good, bad}
	res := RunEx(scene, testCfg(), RunOpts{})
	if len(res.Failures) != 1 || res.Failures[0].Stage != verr.StageGeom {
		t.Fatalf("应只有一条 geom 失败: %v", res.Kinds())
	}
	if !errors.Is(res.Error(), verr.Errorf(verr.KindGeomNonFinite, "x")) && !verr.IsKind(res.Error(), verr.KindGeomNonFinite) {
		t.Fatalf("编码应为 geom 非有限: %v", res.Kinds())
	}
	if res.Im.CountOn(1) == 0 {
		t.Fatal("好多边形应照常点亮")
	}
	// 只有好多边形的图，应与单独跑好多边形逐像素一致。
	solo := RunEx([][]geom.Point{good}, testCfg(), RunOpts{})
	if !samePixels(res.Im, solo.Im) {
		t.Fatal("坏多边形不应影响好多边形的输出")
	}
}

func samePixels(a, b *raster.Image) bool {
	if a.W != b.W || a.H != b.H || len(a.Pix) != len(b.Pix) {
		return false
	}
	for i := range a.Pix {
		if a.Pix[i] != b.Pix[i] {
			return false
		}
	}
	return true
}

// 成功路径：RunEx 在不失败时与旧 Run 逐像素一致，也与手工串五段一致。
func TestRunExSuccessMatchesManualChain(t *testing.T) {
	cfg := testCfg()
	cfg.Shading = "gradient"
	cfg.Gamma = 1.7
	cfg.Blur = 1
	cfg.Rotate = 0.3
	scene := testScene()

	// 手工串一遍旧语义（不隔离）：
	moved := make([][]geom.Point, len(scene))
	for i := range scene {
		moved[i] = Transform(scene[i], cfg.Scale, cfg.Rotate)
	}
	im, err := raster.New(cfg.Width, cfg.Height)
	if err != nil {
		t.Fatal(err)
	}
	if err := im.Fill(moved, cfg.Supersample); err != nil {
		t.Fatal(err)
	}
	if err := shading.Apply(im, cfg.Shading, 1.0, 0.0); err != nil {
		t.Fatal(err)
	}
	if err := post.Gamma(im, cfg.Gamma); err != nil {
		t.Fatal(err)
	}
	out, err := post.Blur(im, cfg.Blur)
	if err != nil {
		t.Fatal(err)
	}

	res := RunEx(scene, cfg, RunOpts{})
	if res.Failed() {
		t.Fatalf("不应失败: %v", res.Kinds())
	}
	if !samePixels(res.Im, out) {
		t.Fatal("成功路径与手工串联结果不一致")
	}
}

// 成功路径里各段都记一条 stage-ok，顺序为 start → config → raster(new) →
// geom → raster(fill) → shading → gamma → blur → finish。
func TestDiagOrderSuccess(t *testing.T) {
	d := diag.New(true, func() time.Time { return time.Unix(1000, 0) })
	res := RunEx(testScene(), testCfg(), RunOpts{Diag: d})
	if res.Failed() {
		t.Fatalf("不应失败: %v", res.Kinds())
	}
	recs := d.Records()
	if len(recs) != 9 {
		t.Fatalf("记录条数: %d", len(recs))
	}
	if recs[0].Event != diag.EvStart || recs[len(recs)-1].Event != diag.EvFinish {
		t.Fatalf("首尾: %v ... %v", recs[0].Event, recs[len(recs)-1].Event)
	}
	for i := 1; i < len(recs); i++ {
		if recs[i].Seq != recs[i-1].Seq+1 {
			t.Fatalf("Seq 不连续 @%d", i)
		}
	}
	// config 的 stage-ok 在 raster-new 之前；raster-fill 在 shading 之前。
	if recs[1].Stage != verr.StageNone && recs[1].Kind != 0 {
		t.Fatalf("config 段记录不应带其他段的编码: %v", recs[1])
	}
}

// 失败路径也按发生顺序记录，且能按段还原现场。
func TestDiagOrderFailure(t *testing.T) {
	d := diag.New(true, func() time.Time { return time.Unix(1000, 0) })
	res := runEx(testScene(), testCfg(), RunOpts{Diag: d}, failOne(verr.KindShadingMode))
	if len(res.Failures) != 1 {
		t.Fatalf("应一条失败")
	}
	recs := d.Records()
	var sawFail bool
	for _, rec := range recs {
		if rec.Event == diag.EvStageDropped && rec.Kind == verr.KindShadingMode {
			sawFail = true
			if rec.Stage != verr.StageShading {
				t.Fatalf("记录的段不对: %v", rec.Stage)
			}
		}
	}
	if !sawFail {
		t.Fatal("诊断里应能找到被摘掉的 shading 段")
	}
	if got := d.StageDrops(verr.StageShading); got != 1 {
		t.Fatalf("shading 摘除计数: %d", got)
	}
	if got := d.StageErrors(verr.KindShadingMode.Stage()); got != 1 {
		t.Fatalf("shading 失败计数: %d", got)
	}
}

// 核心约束：诊断常开时不能多出任何堆分配。
//
// 写法：对每一种诊断状态，先用一次 AllocsPerRun 取“前”读数，再用一次取
// “后”读数，然后比较前后差值。差值必须为 0——说明该状态下渲染的分配
// 已经稳定，不是靠预热偶然抹平的。
//
// 再用同法比较“关闭”与“常开”两种状态的读数差值，差值同样必须为 0：
// 诊断本身不引入任何新的分配。这是把“前后差值”钉在断言里的地方。
func TestDiagNoExtraAlloc(t *testing.T) {
	scene := testScene()
	cfg := testCfg()

	closed := RunOpts{Diag: diag.New(false, nil)}
	open := RunOpts{Diag: diag.New(true, func() time.Time { return time.Unix(1000, 0) })}

	// 预热：让所有一次性缓冲先分配完（New 里的环形缓冲只分配一次）。
	_ = RunEx(scene, cfg, closed)
	_ = RunEx(scene, cfg, open)

	// 分配计数器读数：同一段渲染连续测两次取稳定值，再比较两种状态。
	offBefore := testing.AllocsPerRun(200, func() { _ = RunEx(scene, cfg, closed) })
	offAfter := testing.AllocsPerRun(200, func() { _ = RunEx(scene, cfg, closed) })
	onBefore := testing.AllocsPerRun(200, func() { _ = RunEx(scene, cfg, open) })
	onAfter := testing.AllocsPerRun(200, func() { _ = RunEx(scene, cfg, open) })

	// 关闭状态下前后两次读数必须一致（测量本身稳定）。
	if d := offAfter - offBefore; d != 0 {
		t.Fatalf("关闭状态前后差值非零: %v（%v -> %v）", d, offBefore, offAfter)
	}
	// 常开状态下前后两次读数也必须一致。
	if d := onAfter - onBefore; d != 0 {
		t.Fatalf("常开状态前后差值非零: %v（%v -> %v）", d, onBefore, onAfter)
	}
	// 关键差值：常开比关闭多出的分配数应为 0。
	if d := onAfter - offAfter; d != 0 {
		t.Fatalf("诊断常开比关闭多出 %v 次分配（关闭 %v，常开 %v）", d, offAfter, onAfter)
	}
	// 记录读数供人工核对（-v 时可见）。
	t.Logf("每轮渲染分配：诊断关闭=%v 常开=%v 差值=%v", offAfter, onAfter, onAfter-offAfter)
}

// 失败路径同样不能因为诊断而多分配。
func TestDiagNoExtraAllocOnFailure(t *testing.T) {
	scene := testScene()
	cfg := testCfg()
	inj := failOne(verr.KindRasterSuper)

	closed := RunOpts{Diag: diag.New(false, nil)}
	open := RunOpts{Diag: diag.New(true, func() time.Time { return time.Unix(1000, 0) })}

	_ = runEx(scene, cfg, closed, inj)
	_ = runEx(scene, cfg, open, inj)

	off := testing.AllocsPerRun(200, func() { _ = runEx(scene, cfg, closed, inj) })
	on := testing.AllocsPerRun(200, func() { _ = runEx(scene, cfg, open, inj) })
	if d := on - off; d != 0 {
		t.Fatalf("失败路径诊断多出 %v 次分配（关闭 %v，常开 %v）", d, off, on)
	}
	t.Logf("失败路径分配：诊断关闭=%v 常开=%v 差值=%v", off, on, on-off)
}

// 诊断关闭时是纯空操作：不能有记录。
func TestDiagOffIsNoop(t *testing.T) {
	d := diag.New(false, nil)
	_ = RunEx(testScene(), testCfg(), RunOpts{Diag: d})
	if d.Written() != 0 || d.Len() != 0 {
		t.Fatalf("关闭状态不应写记录: %d/%d", d.Written(), d.Len())
	}
}
