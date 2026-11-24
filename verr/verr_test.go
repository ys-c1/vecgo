package verr

import (
	"errors"
	"io/fs"
	"testing"
)

// 一个 Kind 的段位和它的常量必须自洽：每个常量都落在自己声明的段内，
// 且 Stage 由段位推导不会走样。
func TestKindStageConsistency(t *testing.T) {
	cases := map[Kind]Stage{
		KindConfigWidth: StageConfig, KindConfigOutput: StageConfig,
		KindGeomSegments: StageGeom, KindGeomNonFinite: StageGeom,
		KindRasterSize: StageRaster, KindRasterClose: StageRaster,
		KindShadingMode: StageShading, KindShadingNilImage: StageShading,
		KindPostGamma: StagePost, KindPostBlurImage: StagePost,
		KindSnapRead: StageSnap, KindSnapWrite: StageSnap,
	}
	for k, want := range cases {
		if got := k.Stage(); got != want {
			t.Fatalf("%s 的段: 期望 %v 实际 %v", k.Hex(), want, got)
		}
		if k.Stage().Base() != k&^Kind(0xFF) {
			t.Fatalf("%s 不在 %v 段内", k.Hex(), k.Stage())
		}
	}
}

// 五段之间不重叠，段位唯一。
func TestStagesDisjoint(t *testing.T) {
	stages := []Stage{StageConfig, StageGeom, StageRaster, StageShading, StagePost, StageSnap}
	seen := map[Kind]Stage{}
	for _, s := range stages {
		for i := 1; i <= 0xFF; i++ {
			k := Kind(uint32(s)<<8 | uint32(i))
			if prev, ok := seen[k]; ok {
				t.Fatalf("%s 同时属于 %v 与 %v", k.Hex(), prev, s)
			}
			seen[k] = s
		}
	}
}

// 已归一错误保留编码；外部错误原样可被 errors.Is/As 找到。
func TestWrapPreservesCause(t *testing.T) {
	inner := &fs.PathError{Op: "open", Path: "/nope", Err: fs.ErrNotExist}
	err := Wrap(KindRasterCreate, inner)
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("底层原因不可达")
	}
	var pe *fs.PathError
	if !errors.As(err, &pe) {
		t.Fatal("errors.As 不可达")
	}
	if k, ok := Of(err); !ok || k != KindRasterCreate {
		t.Fatalf("编码: %s", k.Hex())
	}
}

// 重复 Wrap 不应改变已有编码。
func TestWrapIdempotent(t *testing.T) {
	err := Errorf(KindPostGamma, "x")
	wrapped := Wrap(KindRasterWrite, err)
	if k, _ := Of(wrapped); k != KindPostGamma {
		t.Fatalf("重复包裹改变了编码: %s", k.Hex())
	}
}

// Multi 能让 errors.Is/As 命中其中任意一条，且文案列出各段编码。
func TestMulti(t *testing.T) {
	a := Errorf(KindConfigWidth, "a")
	b := Errorf(KindPostBlurRadius, "b")
	m := NewMulti(a, nil, b)
	if m == nil {
		t.Fatal("应返回 Multi")
	}
	if !errors.Is(m, a) || !errors.Is(m, b) {
		t.Fatal("Multi 应命中两个底层错误")
	}
	if _, ok := Of(m); !ok {
		t.Fatal("Multi 的 Of 应取到其中一条")
	}
	t.Log(m.Error())
	if m.Error() == "" {
		t.Fatal("文案为空")
	}
}

func TestNewMultiSingle(t *testing.T) {
	a := Errorf(KindGeomSegments, "a")
	if got := NewMulti(a); got != a {
		t.Fatal("单条应原样返回，不额外包一层")
	}
	if got := NewMulti(nil); got != nil {
		t.Fatal("全 nil 应返回 nil")
	}
}

func TestStageBase(t *testing.T) {
	if StageRaster.Base() != 0x0300 {
		t.Fatalf("raster base: %s", StageRaster.Base().Hex())
	}
	if StageNone.Base() != 0 || Stage(99).Base() != 0 {
		t.Fatal("越界段应返回 0")
	}
}
