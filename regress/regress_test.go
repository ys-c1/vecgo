package regress

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"vecgo/post"
	"vecgo/raster"
)

var update = flag.Bool("update", false, "重写基准图")

func baselinePath(name string) string {
	return filepath.Join("testdata", "baseline", name+".pgm")
}

// 基准锚点：每个场景的渲染结果必须与存档基准逐像素一致。
// go test ./regress -update 可重写基准（改动渲染器后人工确认再执行）。
func TestBaselineAnchors(t *testing.T) {
	for _, sc := range Scenes() {
		im, err := Render(sc)
		if err != nil {
			t.Fatalf("%s: 渲染失败: %v", sc.Name, err)
		}
		path := baselinePath(sc.Name)
		if *update {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := im.SavePGM(path); err != nil {
				t.Fatalf("%s: 写基准失败: %v", sc.Name, err)
			}
		}
		base, err := raster.LoadPGM(path)
		if err != nil {
			t.Fatalf("%s: 读基准失败（先跑 go test ./regress -update）: %v", sc.Name, err)
		}
		rep, err := Diff(base, im)
		if err != nil {
			t.Fatalf("%s: %v", sc.Name, err)
		}
		if rep.Count != 0 {
			t.Errorf("%s: 与基准有 %d 处差异，归因=%s，首处 (%d,%d) %d->%d",
				sc.Name, rep.Count, Label(rep.Category),
				rep.Top[0].X, rep.Top[0].Y, rep.Top[0].Base, rep.Top[0].Cur)
		}
	}
}

// 渲染必须确定：同一场景渲染两次，差异为零。
func TestDeterministic(t *testing.T) {
	for _, sc := range Scenes() {
		a, err := Render(sc)
		if err != nil {
			t.Fatalf("%s: %v", sc.Name, err)
		}
		b, err := Render(sc)
		if err != nil {
			t.Fatalf("%s: %v", sc.Name, err)
		}
		rep, err := Diff(a, b)
		if err != nil {
			t.Fatalf("%s: %v", sc.Name, err)
		}
		if rep.Count != 0 {
			t.Errorf("%s: 两次渲染有 %d 处差异，渲染不确定", sc.Name, rep.Count)
		}
		if rep.Category != CategoryNone {
			t.Errorf("%s: 零差异应归为 none，得到 %s", sc.Name, rep.Category)
		}
	}
}

// 注入偏差一：旋转 0.08 弧度（变换偏了），归应应指向 transform。
func TestAttributeTransform(t *testing.T) {
	sc, err := SceneByName("single")
	if err != nil {
		t.Fatal(err)
	}
	base, err := Render(sc)
	if err != nil {
		t.Fatal(err)
	}
	sc.Cfg.Rotate += 0.08
	cur, err := Render(sc)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := Diff(base, cur)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Category != CategoryTransform {
		t.Fatalf("注入变换偏差，应归为 transform，得到 %s（丢失 %d 新增 %d 幅度 %.1f 实心度 %.2f）",
			Label(rep.Category), rep.Lost, rep.Gained, rep.MeanMag, rep.Solidity)
	}
}

// 注入偏差二：重叠场景去掉中间竖条（填充漏了），归因应指向 fill。
// 外框由上下两条横条决定，去掉竖条不改变包围盒，差异是孤立的实心块。
func TestAttributeFill(t *testing.T) {
	sc, err := SceneByName("overlap")
	if err != nil {
		t.Fatal(err)
	}
	base, err := Render(sc)
	if err != nil {
		t.Fatal(err)
	}
	sc.Polys = append(sc.Polys[:1], sc.Polys[2:]...) // 去掉中间竖条
	cur, err := Render(sc)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := Diff(base, cur)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Category != CategoryFill {
		t.Fatalf("注入填充缺失，应归为 fill，得到 %s（丢失 %d 新增 %d 幅度 %.1f 实心度 %.2f）",
			Label(rep.Category), rep.Lost, rep.Gained, rep.MeanMag, rep.Solidity)
	}
	// 顺便验证前十处位置都是真实差异点
	if len(rep.Top) == 0 || len(rep.Top) > 10 {
		t.Fatalf("Top 长度异常: %d", len(rep.Top))
	}
	for _, p := range rep.Top {
		if base.Pix[p.Y*base.W+p.X] != p.Base || cur.Pix[p.Y*cur.W+p.X] != p.Cur || p.Base == p.Cur {
			t.Fatalf("Top 中 (%d,%d) 不是真实差异点", p.X, p.Y)
		}
	}
}

// 注入偏差三：渲染结果额外做一次半径 1 的模糊（后处理糊了），归因应指向 post。
func TestAttributePost(t *testing.T) {
	sc, err := SceneByName("single")
	if err != nil {
		t.Fatal(err)
	}
	base, err := Render(sc)
	if err != nil {
		t.Fatal(err)
	}
	cur, err := Render(sc)
	if err != nil {
		t.Fatal(err)
	}
	blurred, err := post.Blur(cur, 1)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := Diff(base, blurred)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Category != CategoryPost {
		t.Fatalf("注入后处理模糊，应归为 post，得到 %s（丢失 %d 新增 %d 幅度 %.1f 实心度 %.2f）",
			Label(rep.Category), rep.Lost, rep.Gained, rep.MeanMag, rep.Solidity)
	}
}
