package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestDeprecatedMappingTable 导出映射表必须结构完整：
// 9 个当前字段、每个恰好有两代旧名、旧键不与当前键撞字面量、旧键不重复。
func TestDeprecatedMappingTable(t *testing.T) {
	if len(DeprecatedKeys) != 18 {
		t.Fatalf("映射表应有 18 条（9 字段 × 2 代），实际 %d", len(DeprecatedKeys))
	}
	seen := map[string]bool{}
	perField := map[string]map[int]bool{}
	for _, km := range DeprecatedKeys {
		if seen[km.OldKey] {
			t.Fatalf("旧键重复: %q", km.OldKey)
		}
		seen[km.OldKey] = true
		if canonicalKeys[km.OldKey] {
			t.Fatalf("旧键 %q 与当前键同名，会导致解释歧义", km.OldKey)
		}
		if !canonicalKeys[km.NewKey] {
			t.Fatalf("映射目标 %q 不是合法当前键", km.NewKey)
		}
		if km.Generation != GenerationV1 && km.Generation != GenerationV2 {
			t.Fatalf("旧键 %q 代际非法: %d", km.OldKey, km.Generation)
		}
		if perField[km.NewKey] == nil {
			perField[km.NewKey] = map[int]bool{}
		}
		perField[km.NewKey][km.Generation] = true
	}
	if len(perField) != len(canonicalKeys) {
		t.Fatalf("映射覆盖 %d 个字段，应为 %d 个", len(perField), len(canonicalKeys))
	}
	for key, gens := range perField {
		if !gens[GenerationV1] || !gens[GenerationV2] {
			t.Fatalf("字段 %q 缺少完整两代旧名: %v", key, gens)
		}
	}
	// LookupDeprecated 与表内容一致。
	km, ok := LookupDeprecated("zoom")
	if !ok || km.NewKey != "scale" || km.Generation != GenerationV1 {
		t.Fatalf("LookupDeprecated(zoom) 结果错误: %+v ok=%v", km, ok)
	}
	if _, ok := LookupDeprecated("不存在"); ok {
		t.Fatal("未知键不应命中映射")
	}
}

// loadFixture 从 testdata 读一份配置。
func loadFixture(t *testing.T, name string) (Config, []Warning) {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", "legacy", name))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	cfg, warnings, err := Load(f)
	if err != nil {
		t.Fatalf("加载 %s 失败: %v", name, err)
	}
	return cfg, warnings
}

// countWarnings 按类别统计告警。
func countWarnings(ws []Warning) map[WarningKind]int {
	out := map[WarningKind]int{}
	for _, w := range ws {
		out[w.Kind]++
	}
	return out
}

// TestLegacyFixturesMigrateToTwins 四份老配置（v1/v2 各两份）迁移结果必须与
// 同值当前键 twin 完全一致，且 9 个旧键各打一条弃用告警、无其他告警。
func TestLegacyFixturesMigrateToTwins(t *testing.T) {
	for _, name := range []string{"v1_1", "v1_2", "v2_1", "v2_2"} {
		got, warnings := loadFixture(t, name+".json")
		want, _ := loadFixture(t, name+".twin.json")
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s 迁移后配置与 twin 不一致\n got=%+v\nwant=%+v", name, got, want)
		}
		counts := countWarnings(warnings)
		if counts[KindDeprecated] != 9 {
			t.Fatalf("%s 应有 9 条弃用告警，实际 %d: %+v", name, counts[KindDeprecated], warnings)
		}
		if counts[KindConflict] != 0 || counts[KindUnknown] != 0 {
			t.Fatalf("%s 不应有冲突/未知告警: %+v", name, warnings)
		}
		for _, w := range warnings {
			if w.Kind != KindDeprecated || w.Target == "" || w.Generation == 0 {
				t.Fatalf("%s 告警字段不完整: %+v", name, w)
			}
		}
	}
}

// TestNewKeyWinsOnConflict 冲突规则样例（新旧键同时出现）：
// 同一字段 scale 同时由第 2 代旧键 "s" 和当前键 "scale" 提供，
// 规则是"新键优先"——当前键值 3.5 生效，旧键值 9.0 忽略。
//
// 取舍理由：旧键已进入弃用周期，写入新键意味着作者在更新配置，
// 新键代表当前意图；若让旧键赢，迁移层反而会把"已经修好的配置"改回去，
// 正是本次要消灭的静默出图偏移。
func TestNewKeyWinsOnConflict(t *testing.T) {
	doc := `{
	  "width": 32, "height": 32,
	  "s": 9.0,
	  "scale": 3.5,
	  "shading": "flat", "output": "conflict.pgm"
	}`
	cfg, warnings, err := Load(strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Scale != 3.5 {
		t.Fatalf("冲突时新键应优先：scale 应为 3.5，实际 %v", cfg.Scale)
	}
	counts := countWarnings(warnings)
	if counts[KindDeprecated] != 1 {
		t.Fatalf("旧键 s 应发 1 条弃用告警: %+v", warnings)
	}
	if counts[KindConflict] != 1 {
		t.Fatalf("应发 1 条冲突告警: %+v", warnings)
	}
	var cw *Warning
	for i := range warnings {
		if warnings[i].Kind == KindConflict {
			cw = &warnings[i]
		}
	}
	if cw == nil || cw.Key != "s" || cw.Target != "scale" {
		t.Fatalf("冲突告警指向错误: %+v", cw)
	}
	if !strings.Contains(cw.Message, "新键优先") {
		t.Fatalf("冲突告警未说明取舍: %q", cw.Message)
	}
}

// TestNewerGenerationWinsOnConflict 两代旧键同场时，代际新者（v2）优先。
func TestNewerGenerationWinsOnConflict(t *testing.T) {
	doc := `{
	  "zoom": 9.0,
	  "s": 2.5,
	  "width": 16, "height": 16, "shading": "flat", "output": "g.pgm"
	}`
	cfg, warnings, err := Load(strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Scale != 2.5 {
		t.Fatalf("v2 旧键应压过 v1：scale 应为 2.5，实际 %v", cfg.Scale)
	}
	counts := countWarnings(warnings)
	if counts[KindDeprecated] != 2 {
		t.Fatalf("两个旧键各应一条弃用告警: %+v", warnings)
	}
	if counts[KindConflict] != 1 {
		t.Fatalf("应发 1 条冲突告警: %+v", warnings)
	}
	if !strings.Contains(warnings[len(warnings)-1].Message, "代际新者优先") {
		t.Fatalf("冲突告警未说明代际规则: %q", warnings[len(warnings)-1].Message)
	}
}

// TestUnknownKeyWarnsInsteadOfSilentDrop 未知键不再静默丢弃。
func TestUnknownKeyWarnsInsteadOfSilentDrop(t *testing.T) {
	doc := `{"width": 16, "height": 16, "blur_radius": 2, "output": "u.pgm"}`
	cfg, warnings, err := Load(strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Blur != 0 {
		t.Fatalf("未知键不应写入任何字段，blur 应保持默认 0，实际 %d", cfg.Blur)
	}
	counts := countWarnings(warnings)
	if counts[KindUnknown] != 1 || warnings[0].Key != "blur_radius" {
		t.Fatalf("应对未知键发一条告警: %+v", warnings)
	}
}

// TestBadValueTypeRejected 类型错误显式失败，不猜、不兜底。
func TestBadValueTypeRejected(t *testing.T) {
	doc := `{"width": "very-wide", "height": 16}`
	if _, _, err := Load(strings.NewReader(doc)); err == nil {
		t.Fatal("width 给字符串应报错")
	}
	doc = `{"zoom": []}`
	if _, _, err := Load(strings.NewReader(doc)); err == nil {
		t.Fatal("zoom 给数组应报错")
	}
}

// TestInvalidJSONAndValidation 语法错误与取值越界都要失败。
func TestInvalidJSONAndValidation(t *testing.T) {
	if _, _, err := Load(strings.NewReader(`{not json`)); err == nil {
		t.Fatal("非法 JSON 应报错")
	}
	doc := `{"width": 16, "height": 16, "gamma": 0, "output": "x.pgm"}`
	if _, _, err := Load(strings.NewReader(doc)); err == nil {
		t.Fatal("gamma 越界应报错")
	}
	// 越界通过旧键迁移后同样要被 Validate 拦住。
	doc = `{"w": 16, "h": 16, "g": 0, "out": "x.pgm"}`
	if _, _, err := Load(strings.NewReader(doc)); err == nil {
		t.Fatal("旧键迁移后的越界值应报错")
	}
}

// TestCurrentKeyOnlyHasNoWarnings 全当前键文件零告警。
func TestCurrentKeyOnlyHasNoWarnings(t *testing.T) {
	_, warnings := loadFixture(t, "v1_1.twin.json")
	if len(warnings) != 0 {
		t.Fatalf("当前键文件不应产生告警: %+v", warnings)
	}
}
