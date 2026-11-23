package layer

import (
	"strings"
	"testing"
)

func TestAll(t *testing.T) {
	m, err := All(4)
	if err != nil {
		t.Fatal(err)
	}
	if m != Mask(0b1111) {
		t.Fatalf("All(4) = %b，应为 1111", uint64(m))
	}
	m, err = All(MaxLayers)
	if err != nil {
		t.Fatal(err)
	}
	if m != Mask(^uint64(0)) {
		t.Fatalf("All(64) 应全位置 1，得到 %x", uint64(m))
	}
	if _, err := All(65); err == nil || !strings.Contains(err.Error(), "64") {
		t.Fatalf("All(65) 应报位数超限并指出位 64: %v", err)
	}
}

func TestCheckOverflow(t *testing.T) {
	issues := Check(Mask(1), 66)
	if len(issues) != 1 || issues[0].Kind != KindOverflow {
		t.Fatalf("应只有位数超限一条诊断: %v", issues)
	}
	want := []int{64, 65}
	if len(issues[0].Bits) != 2 || issues[0].Bits[0] != want[0] || issues[0].Bits[1] != want[1] {
		t.Fatalf("超限位号应为 %v，得到 %v", want, issues[0].Bits)
	}
}

func TestCheckUnknownBits(t *testing.T) {
	// 3 层场景，掩码置了位 1、4、6：位 4、6 是未知图层位。
	issues := Check(Mask(0b1010010), 3)
	if len(issues) != 1 || issues[0].Kind != KindUnknown {
		t.Fatalf("应只有未知图层位一条诊断: %v", issues)
	}
	want := []int{4, 6}
	if len(issues[0].Bits) != 2 || issues[0].Bits[0] != want[0] || issues[0].Bits[1] != want[1] {
		t.Fatalf("未知位号应为 %v，得到 %v", want, issues[0].Bits)
	}
}

func TestCheckAllOff(t *testing.T) {
	issues := Check(Mask(0), 3)
	if len(issues) != 1 || issues[0].Kind != KindAllOff {
		t.Fatalf("应只有掩码全为零一条诊断: %v", issues)
	}
	want := []int{0, 1, 2}
	if len(issues[0].Bits) != 3 {
		t.Fatalf("全零诊断应列出位 %v，得到 %v", want, issues[0].Bits)
	}
	for i, b := range want {
		if issues[0].Bits[i] != b {
			t.Fatalf("全零诊断应列出位 %v，得到 %v", want, issues[0].Bits)
		}
	}
}

func TestCheckClean(t *testing.T) {
	if issues := Check(Mask(0b0101), 4); len(issues) != 0 {
		t.Fatalf("合法掩码不应有诊断: %v", issues)
	}
}

func TestVisible(t *testing.T) {
	// nil 掩码：全部可见（默认行为）。
	vis, err := Visible(nil, 3)
	if err != nil {
		t.Fatal(err)
	}
	for i, v := range vis {
		if !v {
			t.Fatalf("nil 掩码下层 %d 应可见", i)
		}
	}
	m := Mask(0b0101)
	vis, err = Visible(&m, 4)
	if err != nil {
		t.Fatal(err)
	}
	want := []bool{true, false, true, false}
	for i := range want {
		if vis[i] != want[i] {
			t.Fatalf("可见性应为 %v，得到 %v", want, vis)
		}
	}
	if _, err := Visible(&m, 65); err == nil || !strings.Contains(err.Error(), "64") {
		t.Fatalf("65 层应报位数超限并指出位 64: %v", err)
	}
}

func TestMigrateGrow(t *testing.T) {
	old := Mask(0b0101)
	got, table, err := Migrate(old, 4, 6)
	if err != nil {
		t.Fatal(err)
	}
	// 旧位保留，新增位 4、5 默认可见。
	if got != Mask(0b110101) {
		t.Fatalf("扩容迁移应为 110101，得到 %b", uint64(got))
	}
	if len(table) != 6 {
		t.Fatalf("迁移表应有 6 行，得到 %d", len(table))
	}
	if table[5].NewBit != 5 || !strings.Contains(table[5].Note, "新增") {
		t.Fatalf("位 5 应标记为新增: %v", table[5])
	}
}

func TestMigrateShrink(t *testing.T) {
	old := Mask(0b1111)
	got, table, err := Migrate(old, 4, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got != Mask(0b11) {
		t.Fatalf("收缩迁移应为 11，得到 %b", uint64(got))
	}
	if table[2].NewBit != -1 || table[3].NewBit != -1 {
		t.Fatalf("位 2、3 应被丢弃: %v %v", table[2], table[3])
	}
}

func TestMigrateFailurePreservesState(t *testing.T) {
	old := Mask(0b1011)
	got, table, err := Migrate(old, 4, 65)
	if err == nil {
		t.Fatal("超过 64 层应迁移失败")
	}
	if got != old {
		t.Fatalf("迁移失败应保留旧掩码 %b，得到 %b", uint64(old), uint64(got))
	}
	if len(table) != MaxLayers {
		t.Fatalf("失败时应报出能算出的迁移表（%d 行），得到 %d 行", MaxLayers, len(table))
	}
	if !strings.Contains(err.Error(), "64") {
		t.Fatalf("错误应指出超限位号 64: %v", err)
	}
}
