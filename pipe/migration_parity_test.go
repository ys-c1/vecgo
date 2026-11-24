package pipe

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"vecgo/config"
	"vecgo/geom"
)

// paritySquare 与 cmd、gengolden 使用同一固定场景，
// 排除场景差异，保证像素差只可能来自配置迁移。
var paritySquare = [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}}

var parityCases = []string{"v1_1", "v1_2", "v2_1", "v2_2"}

// readPGM 读取 P5 二进制 PGM，返回 (宽, 高, 像素)。只用于测试比对。
func readPGM(t *testing.T, path string) (int, int, []byte) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// P5 头: "P5\n<w> <h>\n255\n"，其后是裸灰度字节。
	headerEnd := 0
	nl := 0
	for i, b := range data {
		if b == '\n' {
			nl++
			if nl == 3 {
				headerEnd = i + 1
				break
			}
		}
	}
	if headerEnd == 0 {
		t.Fatalf("%s 不是合法 P5 PGM", path)
	}
	var magic string
	var w, h, maxv int
	if _, err := fmt.Sscanf(string(data[:headerEnd]), "%s\n%d %d\n%d\n", &magic, &w, &h, &maxv); err != nil || magic != "P5" || maxv != 255 {
		t.Fatalf("%s PGM 头解析失败: %q", path, string(data[:headerEnd]))
	}
	pix := data[headerEnd:]
	if len(pix) != w*h {
		t.Fatalf("%s 像素数 %d 与画幅 %dx%d 不符", path, len(pix), w, h)
	}
	return w, h, pix
}

// render 用指定配置文件渲染固定场景，返回 (宽, 高, 像素副本)。
func render(t *testing.T, cfgPath string) (int, int, []byte) {
	t.Helper()
	cfg, _, err := config.LoadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	im, err := Run(paritySquare, cfg)
	if err != nil {
		t.Fatal(err)
	}
	return im.W, im.H, append([]byte(nil), im.Pix...)
}

// TestLegacyConfigsPixelParity 是迁移验收的核心：
// 每份老配置出一张图，与 golden 基线（改动前由同值当前键 twin 渲出）
// 逐像素比对，同时与 twin 现渲结果交叉比对。任一像素不同即失败，
// 并报告第一个差异像素的坐标与差值，便于说明原因。
func TestLegacyConfigsPixelParity(t *testing.T) {
	legacyDir := filepath.Join("..", "config", "testdata", "legacy")
	goldenDir := filepath.Join("testdata", "golden")

	for _, name := range parityCases {
		// 老配置迁移后渲染。
		gw, gh, got := render(t, filepath.Join(legacyDir, name+".json"))
		// twin（同值当前键）现渲，交叉验证。
		tw, th, twin := render(t, filepath.Join(legacyDir, name+".twin.json"))
		// golden：改动前生成并提交的基线图。
		bw, bh, golden := readPGM(t, filepath.Join(goldenDir, name+".pgm"))

		if gw != tw || gw != bw || gh != th || gh != bh {
			t.Fatalf("%s 画幅不一致: legacy=%dx%d twin=%dx%d golden=%dx%d",
				name, gw, gh, tw, th, bw, bh)
		}
		if diff := bytes.Compare(got, twin); diff != 0 {
			i := firstDiff(got, twin)
			t.Fatalf("%s 老配置与 twin 渲染出现像素差: 第 %d 个像素 (x=%d,y=%d) legacy=%d twin=%d",
				name, i, i%gw, i/gw, got[i], twin[i])
		}
		if diff := bytes.Compare(got, golden); diff != 0 {
			i := firstDiff(got, golden)
			t.Fatalf("%s 与改动前 golden 基线出现像素差: 第 %d 个像素 (x=%d,y=%d) now=%d golden=%d",
				name, i, i%gw, i/gw, got[i], golden[i])
		}
		t.Logf("%s: 逐像素比对通过，%dx%d 共 %d 像素，差值为零", name, gw, gh, gw*gh)
	}
}

func firstDiff(a, b []byte) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return n
}
