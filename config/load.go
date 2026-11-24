package config

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
)

// 配置键名一共经历过三代：
//   - 第 1 代（GenerationV1）：第一轮改名之前的旧名；
//   - 第 2 代（GenerationV2）：第一轮改名之后、第二轮改名之前的旧名；
//   - 第 3 代（GenerationCurrent）：当前键名，即 Config 字段的小写形式。
const (
	GenerationV1      = 1
	GenerationV2      = 2
	GenerationCurrent = 3
)

// KeyMigration 描述一条弃用键到当前键的映射。
type KeyMigration struct {
	OldKey     string // 旧文件里出现的键名
	NewKey     string // 当前代（v3）的规范键名
	Generation int    // 旧键所属代际：GenerationV1 或 GenerationV2
}

// DeprecatedKeys 是导出的弃用键映射表，覆盖两轮改名的全部旧键。
// 三代键名的字面量两两不相交，因此任何一个键的解释都是唯一的。
var DeprecatedKeys = []KeyMigration{
	// 第 1 代旧名（第一轮改名之前）
	{OldKey: "imgW", NewKey: "width", Generation: GenerationV1},
	{OldKey: "imgH", NewKey: "height", Generation: GenerationV1},
	{OldKey: "zoom", NewKey: "scale", Generation: GenerationV1},
	{OldKey: "angle", NewKey: "rotate", Generation: GenerationV1},
	{OldKey: "samples", NewKey: "supersample", Generation: GenerationV1},
	{OldKey: "fill", NewKey: "shading", Generation: GenerationV1},
	{OldKey: "gammaV", NewKey: "gamma", Generation: GenerationV1},
	{OldKey: "soften", NewKey: "blur", Generation: GenerationV1},
	{OldKey: "dest", NewKey: "output", Generation: GenerationV1},

	// 第 2 代旧名（第一轮改名后、第二轮改名前）
	{OldKey: "w", NewKey: "width", Generation: GenerationV2},
	{OldKey: "h", NewKey: "height", Generation: GenerationV2},
	{OldKey: "s", NewKey: "scale", Generation: GenerationV2},
	{OldKey: "rot", NewKey: "rotate", Generation: GenerationV2},
	{OldKey: "aa", NewKey: "supersample", Generation: GenerationV2},
	{OldKey: "shade", NewKey: "shading", Generation: GenerationV2},
	{OldKey: "g", NewKey: "gamma", Generation: GenerationV2},
	{OldKey: "b", NewKey: "blur", Generation: GenerationV2},
	{OldKey: "out", NewKey: "output", Generation: GenerationV2},
}

// WarningKind 区分告警类别。
type WarningKind string

const (
	// KindDeprecated：文件里出现了映射表中的弃用旧键，每个被替换的键恰好一条。
	KindDeprecated WarningKind = "deprecated"
	// KindConflict：同一目标字段被多个键同时提供，按优先级只取一个。
	KindConflict WarningKind = "conflict"
	// KindUnknown：键既不是当前键也不是已知旧键，过去会被静默丢弃。
	KindUnknown WarningKind = "unknown"
)

// Warning 是读旧配置时产生的一条告警。告警不是错误：配置仍可渲染，
// 但每一条都必须让调用方看到，杜绝"不认识的键被静默丢掉、出图跟着偏"。
type Warning struct {
	Key        string      // 触发告警的文件键名
	Kind       WarningKind // 告警类别
	Generation int         // 旧键代际；未知键为 0
	Target     string      // 映射到的当前键名；未知键为空
	Message    string      // 可直接展示给用户的中文说明
}

func (w Warning) String() string { return w.Message }

// canonicalKeys 是当前代（v3）合法键集合。
var canonicalKeys = map[string]bool{
	"width": true, "height": true, "scale": true, "rotate": true,
	"supersample": true, "shading": true, "gamma": true, "blur": true,
	"output": true,
}

// aliasIndex 由 DeprecatedKeys 构建：旧键字面量 -> 映射条目。
var aliasIndex = func() map[string]KeyMigration {
	m := make(map[string]KeyMigration, len(DeprecatedKeys))
	for _, km := range DeprecatedKeys {
		m[km.OldKey] = km
	}
	return m
}()

// CanonicalKeys 按字典序返回当前代（v3）的全部合法键。
func CanonicalKeys() []string {
	out := make([]string, 0, len(canonicalKeys))
	for k := range canonicalKeys {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// LookupDeprecated 查询旧键映射；ok 为 false 表示该键不在弃用映射表中。
func LookupDeprecated(oldKey string) (KeyMigration, bool) {
	km, ok := aliasIndex[oldKey]
	return km, ok
}

// assign 按当前键名把一段 JSON 原值写入配置；类型不符时错误里带上键名。
func assign(c *Config, key string, raw json.RawMessage) error {
	bad := func(err error) error {
		return fmt.Errorf("键 %q 取值非法: %w", key, err)
	}
	switch key {
	case "width":
		var v int
		if err := json.Unmarshal(raw, &v); err != nil {
			return bad(err)
		}
		c.Width = v
	case "height":
		var v int
		if err := json.Unmarshal(raw, &v); err != nil {
			return bad(err)
		}
		c.Height = v
	case "scale":
		var v float64
		if err := json.Unmarshal(raw, &v); err != nil {
			return bad(err)
		}
		c.Scale = v
	case "rotate":
		var v float64
		if err := json.Unmarshal(raw, &v); err != nil {
			return bad(err)
		}
		c.Rotate = v
	case "supersample":
		var v int
		if err := json.Unmarshal(raw, &v); err != nil {
			return bad(err)
		}
		c.Supersample = v
	case "shading":
		var v string
		if err := json.Unmarshal(raw, &v); err != nil {
			return bad(err)
		}
		c.Shading = v
	case "gamma":
		var v float64
		if err := json.Unmarshal(raw, &v); err != nil {
			return bad(err)
		}
		c.Gamma = v
	case "blur":
		var v int
		if err := json.Unmarshal(raw, &v); err != nil {
			return bad(err)
		}
		c.Blur = v
	case "output":
		var v string
		if err := json.Unmarshal(raw, &v); err != nil {
			return bad(err)
		}
		c.Output = v
	default:
		return fmt.Errorf("内部错误: assign 收到非当前键 %q", key)
	}
	return nil
}

// Load 从 JSON 读配置并完成旧键迁移。
//
// 优先级（冲突规则）：
//  1. 当前键（v3）优先级最高。同一字段若新旧键同时出现，新键生效，旧键忽略，
//     除弃用告警外再发一条 KindConflict 告警；
//  2. 只有旧键时，代际新者优先（v2 > v1），落败者发 KindConflict 告警；
//  3. 每个出现的弃用旧键都恰好发一条 KindDeprecated 告警，无论它最终是否生效；
//  4. 既非当前键也不在映射表中的键发 KindUnknown 告警，不再静默丢弃；
//  5. 取值类型错误直接返回 error，不猜、不兜底。
func Load(r io.Reader) (Config, []Warning, error) {
	cfg := Default()
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(r).Decode(&raw); err != nil {
		return cfg, nil, fmt.Errorf("配置解析失败: %w", err)
	}

	keys := make([]string, 0, len(raw))
	for k := range raw {
		keys = append(keys, k)
	}
	sort.Strings(keys) // 告警顺序确定，便于比对与排查

	var warnings []Warning
	// owner 记录每个当前字段当前由哪个键提供值。
	owner := map[string]string{}

	// 第一遍：当前键全部直接生效，先占住字段。
	for _, k := range keys {
		if !canonicalKeys[k] {
			continue
		}
		if err := assign(&cfg, k, raw[k]); err != nil {
			return cfg, warnings, err
		}
		owner[k] = k
	}

	// 第二遍：处理旧键与未知键。
	for _, k := range keys {
		if canonicalKeys[k] {
			continue
		}
		m, ok := aliasIndex[k]
		if !ok {
			warnings = append(warnings, Warning{
				Key:     k,
				Kind:    KindUnknown,
				Message: fmt.Sprintf("未知键 %q 已忽略：既不是当前键，也不在弃用键映射表中", k),
			})
			continue
		}

		// 规则 3：只要旧键出现，弃用告警必发一条。
		warnings = append(warnings, Warning{
			Key:        k,
			Kind:       KindDeprecated,
			Generation: m.Generation,
			Target:     m.NewKey,
			Message: fmt.Sprintf("弃用告警: 键 %q 是第 %d 代旧名，已替换为 %q，请更新配置文件",
				k, m.Generation, m.NewKey),
		})

		held, busy := owner[m.NewKey]
		if !busy {
			if err := assign(&cfg, m.NewKey, raw[k]); err != nil {
				return cfg, warnings, err
			}
			owner[m.NewKey] = k
			continue
		}

		// 同一字段已被占用：按优先级决定谁生效。
		if canonicalKeys[held] {
			// 规则 1：新旧键同名冲突（争抢同一字段），新键优先。
			warnings = append(warnings, Warning{
				Key:        k,
				Kind:       KindConflict,
				Generation: m.Generation,
				Target:     m.NewKey,
				Message: fmt.Sprintf(
					"冲突: 字段 %q 同时由旧键 %q 和新键 %q 提供；新键优先，旧值已忽略",
					m.NewKey, k, held),
			})
			continue
		}

		heldGen := aliasIndex[held].Generation
		if m.Generation > heldGen {
			// 规则 2：代际新者优先，后到的较新键替换旧值。
			if err := assign(&cfg, m.NewKey, raw[k]); err != nil {
				return cfg, warnings, err
			}
			owner[m.NewKey] = k
			warnings = append(warnings, Warning{
				Key:        k,
				Kind:       KindConflict,
				Generation: m.Generation,
				Target:     m.NewKey,
				Message: fmt.Sprintf(
					"冲突: 字段 %q 同时由第 %d 代旧键 %q 和第 %d 代旧键 %q 提供；代际新者优先，采用 %q",
					m.NewKey, heldGen, held, m.Generation, k, k),
			})
		} else {
			warnings = append(warnings, Warning{
				Key:        k,
				Kind:       KindConflict,
				Generation: m.Generation,
				Target:     m.NewKey,
				Message: fmt.Sprintf(
					"冲突: 字段 %q 同时由第 %d 代旧键 %q 和第 %d 代旧键 %q 提供；代际新者优先，保留 %q",
					m.NewKey, m.Generation, k, heldGen, held, held),
			})
		}
	}

	if err := cfg.Validate(); err != nil {
		return cfg, warnings, err
	}
	return cfg, warnings, nil
}

// LoadFile 从文件路径读配置，语义同 Load。
func LoadFile(path string) (Config, []Warning, error) {
	f, err := os.Open(path)
	if err != nil {
		return Default(), nil, err
	}
	defer f.Close()
	return Load(f)
}
