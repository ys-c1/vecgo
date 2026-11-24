package pipe

import (
	"crypto/sha256"
	"encoding/hex"
	"math"
	"strings"
	"testing"

	"vecgo/geom"
	"vecgo/scene"
)

func hashPix(pix []uint8) string {
	sum := sha256.Sum256(pix)
	return hex.EncodeToString(sum[:])
}

// 五类标准用例逐一渲染，并核验十次重跑哈希一致。
func TestStandardScenesHashStable(t *testing.T) {
	for _, c := range scene.Cases() {
		first, err := Run(c.Geom, c.Cfg)
		if err != nil {
			t.Fatalf("场景 %s 渲染失败: %v", c.Name, err)
		}
		want := hashPix(first.Pix)
		for i := 0; i < 10; i++ {
			im, err := Run(c.Geom, c.Cfg)
			if err != nil {
				t.Fatalf("场景 %s 第 %d 次渲染失败: %v", c.Name, i, err)
			}
			if got := hashPix(im.Pix); got != want {
				t.Fatalf("场景 %s 第 %d 次哈希漂移: %s != %s", c.Name, i, got, want)
			}
		}
		t.Logf("场景 %-10s 哈希=%s 像素数=%d", c.Name, want, len(first.Pix))
	}
}

func TestRunRejectsNaNWithField(t *testing.T) {
	bad := [][]geom.Point{{{X: 0, Y: 0}, {X: 1, Y: math.NaN()}, {X: 1, Y: 1}}}
	if _, err := Run(bad, scene.Cases()[0].Cfg); err == nil ||
		!strings.Contains(err.Error(), "polys[0][1]") {
		t.Fatalf("NaN 应透传字段名错误: %v", err)
	}
}

func TestRunRejectsEmptyWithField(t *testing.T) {
	if _, err := Run(nil, scene.Cases()[0].Cfg); err == nil ||
		!strings.Contains(err.Error(), "polys") {
		t.Fatalf("空数组应透传字段名错误: %v", err)
	}
}
