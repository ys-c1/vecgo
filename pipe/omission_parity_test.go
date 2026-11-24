package pipe_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"vecgo/config"
	"vecgo/geom"
	"vecgo/pipe"
)

// frozenDefaults 是改动前 HEAD 的 config.Default() 取值，原样抄自
// artifacts/defaults_before.txt。它故意不引用 config.Default()：
// 缺省实验要比对“缺省值”与“改动前就存在的显式值”，不能拿运行中的
// 默认配置给自己作证。
var frozenDefaults = map[string]string{
	"width":       "64",
	"height":      "64",
	"scale":       "1",
	"rotate":      "0",
	"supersample": "2",
	"shading":     `"flat"`,
	"gamma":       "1",
	"blur":        "0",
	"output":      `"out.pgm"`,
}

// probeScene 与 internal/probe.Scene 相同；放在这里是为了避免
// probe -> pipe 的导入环（probe 依赖 pipe）。
func probeScene() [][]geom.Point {
	return [][]geom.Point{{
		{X: 2.256, Y: -0.898},
		{X: 10.897, Y: 2.256},
		{X: 7.744, Y: 10.897},
		{X: -0.897, Y: 7.744},
	}}
}

// explicitJSON 生成除 omit 外全部显式、值为冻结默认值的 JSON。
func explicitJSON(t *testing.T, omit ...string) string {
	t.Helper()
	skip := map[string]bool{}
	for _, k := range omit {
		skip[k] = true
	}
	out := "{"
	first := true
	for _, name := range config.FieldNames() {
		if skip[name] {
			continue
		}
		if !first {
			out += ","
		}
		first = false
		out += fmt.Sprintf("%q:%s", name, frozenDefaults[name])
	}
	return out + "}"
}

// TestOmissionPixelParity 逐个字段验证单缺省、双缺省、全缺省与显式给值
// 逐像素一致。这是 cmd/omission 报告的同款判据，作为回归测试常驻。
func TestOmissionPixelParity(t *testing.T) {
	render := func(doc string) []uint8 {
		t.Helper()
		cfg, err := config.Load(newReader(doc))
		if err != nil {
			t.Fatalf("加载 %s 失败: %v", doc, err)
		}
		im, err := pipe.Run(probeScene(), cfg)
		if err != nil {
			t.Fatal(err)
		}
		return im.Pix
	}
	base := render(explicitJSON(t))

	if got := render(explicitJSON(t, config.FieldNames()...)); !equal(base, got) {
		t.Fatal("全缺省 {} 与显式默认值不一致")
	}
	for _, name := range config.FieldNames() {
		if got := render(explicitJSON(t, name)); !equal(base, got) {
			t.Fatalf("%s 单缺省与显式值不一致", name)
		}
		// 双缺省：被测字段 + 下一个字段（环形）。
		names := config.FieldNames()
		var next string
		for i, n := range names {
			if n == name {
				next = names[(i+1)%len(names)]
			}
		}
		if got := render(explicitJSON(t, name, next)); !equal(base, got) {
			t.Fatalf("%s+%s 双缺省与显式值不一致", name, next)
		}
	}
}

// TestDefaultSceneUnchanged 锁定改动前 cmd 无参数运行的产物。
// 期望 sha256 由改动前 HEAD 的二进制渲染同一场景得到（artifacts/before.pgm）。
func TestDefaultSceneUnchanged(t *testing.T) {
	const want = "fbda3e5665174433272beab4f25172bc03466e3f8700bcf6007b32c3636f2dc3"
	scene := [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}}
	im, err := pipe.Run(scene, config.Default())
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "out.pgm")
	if err := im.SavePGM(path); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(b)
	if got := hex.EncodeToString(sum[:]); got != want {
		t.Fatalf("默认场景产物变了\n got=%s\nwant=%s", got, want)
	}
}

func equal(a, b []uint8) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func newReader(s string) *strings.Reader { return strings.NewReader(s) }
