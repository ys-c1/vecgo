package batch

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"vecgo/config"
	"vecgo/geom"
	"vecgo/pipe"
)

func okScenes() []Scene {
	return []Scene{
		{Name: "square", Polys: [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}}},
		{Name: "triangle", Polys: [][]geom.Point{{{X: 0, Y: 0}, {X: 20, Y: 0}, {X: 10, Y: 18}}}},
	}
}

func mixedScenes() []Scene {
	return []Scene{
		okScenes()[0],
		{Name: "bad-degenerate", Polys: [][]geom.Point{{{X: 0, Y: 0}, {X: 1, Y: 1}}}},
		{Name: "bad-empty", Polys: nil},
		okScenes()[1],
	}
}

// 隔离开启：异常场景被记账跳过，批次不中断，其余场景照常出图。
func TestIsolateSkipsFailures(t *testing.T) {
	dir := t.TempDir()
	results, err := RunBatch(mixedScenes(), config.Default(), dir, true)
	if err != nil {
		t.Fatalf("隔离开启时不应返回错误: %v", err)
	}
	if len(results) != 4 {
		t.Fatalf("应记账 4 行，实际 %d", len(results))
	}
	wantCodes := []int{CodeOK, CodeRender, CodePanic, CodeOK}
	for i, r := range results {
		if r.Code != wantCodes[i] {
			t.Errorf("场景 %s 错误码 = %d，期望 %d", r.Name, r.Code, wantCodes[i])
		}
		if r.Dur < 0 {
			t.Errorf("场景 %s 耗时不应为负", r.Name)
		}
	}
	// 正常场景出图，异常场景无产物。
	for _, name := range []string{"square", "triangle"} {
		if _, err := os.Stat(filepath.Join(dir, name+".pgm")); err != nil {
			t.Errorf("正常场景 %s 应出图: %v", name, err)
		}
	}
	for _, name := range []string{"bad-degenerate", "bad-empty"} {
		if _, err := os.Stat(filepath.Join(dir, name+".pgm")); !os.IsNotExist(err) {
			t.Errorf("失败场景 %s 不应有产物", name)
		}
	}
}

// 隔离后正常场景的产物与逐个单跑逐字节相同。
func TestByteIdenticalToSingleRun(t *testing.T) {
	cfg := config.Default()
	batchDir := t.TempDir()
	if _, err := RunBatch(okScenes(), cfg, batchDir, true); err != nil {
		t.Fatal(err)
	}
	singleDir := t.TempDir()
	for _, sc := range okScenes() {
		im, err := pipe.Run(sc.Polys, cfg)
		if err != nil {
			t.Fatal(err)
		}
		if err := im.SavePGM(filepath.Join(singleDir, sc.Name+".pgm")); err != nil {
			t.Fatal(err)
		}
		a, err := os.ReadFile(filepath.Join(batchDir, sc.Name+".pgm"))
		if err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(filepath.Join(singleDir, sc.Name+".pgm"))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(a, b) {
			t.Errorf("场景 %s 批次产物与单跑不一致", sc.Name)
		}
	}
}

// 隔离关闭：首个失败即中断，后续场景不渲染，与原行为一致。
func TestNoIsolateStopsOnError(t *testing.T) {
	dir := t.TempDir()
	results, err := RunBatch(mixedScenes(), config.Default(), dir, false)
	if err == nil {
		t.Fatal("隔离关闭时应在首个失败处返回错误")
	}
	if len(results) != 2 {
		t.Fatalf("应只记账到失败场景为止（2 行），实际 %d", len(results))
	}
	if _, err := os.Stat(filepath.Join(dir, "triangle.pgm")); !os.IsNotExist(err) {
		t.Error("失败之后的场景不应被渲染")
	}
}

// 隔离关闭：panic 不被 recover，原样崩溃，与原行为一致。
func TestNoIsolatePropagatesPanic(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("隔离关闭时 panic 应原样传播")
		}
	}()
	_, _ = RunBatch([]Scene{{Name: "bad-empty", Polys: nil}}, config.Default(), t.TempDir(), false)
}
