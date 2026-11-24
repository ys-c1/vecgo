package config

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
)

// 本文件提供“缺省即默认”的配置入口。仓库原本只有 Default()，cmd 直接调用它，
// 不存在任何“文件中省略某字段”的语义。Load/LoadFile 以 Default() 为底，
// 只覆盖文件中真实出现的键；未出现的键保持默认值，并在快照里标为“缺省”。

type fieldSpec struct {
	name string
	get  func(Config) string
	set  func(*Config, json.RawMessage) error
}

func quoted(v string) string { return strconv.Quote(v) }

// fields 的顺序就是 Config 的字段声明顺序，也是取值快照的行顺序。
var fields = []fieldSpec{
	{"width", func(c Config) string { return strconv.Itoa(c.Width) }, func(c *Config, raw json.RawMessage) error {
		var v int
		if err := json.Unmarshal(raw, &v); err != nil {
			return err
		}
		c.Width = v
		return nil
	}},
	{"height", func(c Config) string { return strconv.Itoa(c.Height) }, func(c *Config, raw json.RawMessage) error {
		var v int
		if err := json.Unmarshal(raw, &v); err != nil {
			return err
		}
		c.Height = v
		return nil
	}},
	{"scale", func(c Config) string { return strconv.FormatFloat(c.Scale, 'g', -1, 64) }, func(c *Config, raw json.RawMessage) error {
		var v float64
		if err := json.Unmarshal(raw, &v); err != nil {
			return err
		}
		c.Scale = v
		return nil
	}},
	{"rotate", func(c Config) string { return strconv.FormatFloat(c.Rotate, 'g', -1, 64) }, func(c *Config, raw json.RawMessage) error {
		var v float64
		if err := json.Unmarshal(raw, &v); err != nil {
			return err
		}
		c.Rotate = v
		return nil
	}},
	{"supersample", func(c Config) string { return strconv.Itoa(c.Supersample) }, func(c *Config, raw json.RawMessage) error {
		var v int
		if err := json.Unmarshal(raw, &v); err != nil {
			return err
		}
		c.Supersample = v
		return nil
	}},
	{"shading", func(c Config) string { return c.Shading }, func(c *Config, raw json.RawMessage) error {
		var v string
		if err := json.Unmarshal(raw, &v); err != nil {
			return err
		}
		c.Shading = v
		return nil
	}},
	{"gamma", func(c Config) string { return strconv.FormatFloat(c.Gamma, 'g', -1, 64) }, func(c *Config, raw json.RawMessage) error {
		var v float64
		if err := json.Unmarshal(raw, &v); err != nil {
			return err
		}
		c.Gamma = v
		return nil
	}},
	{"blur", func(c Config) string { return strconv.Itoa(c.Blur) }, func(c *Config, raw json.RawMessage) error {
		var v int
		if err := json.Unmarshal(raw, &v); err != nil {
			return err
		}
		c.Blur = v
		return nil
	}},
	{"output", func(c Config) string { return c.Output }, func(c *Config, raw json.RawMessage) error {
		var v string
		if err := json.Unmarshal(raw, &v); err != nil {
			return err
		}
		c.Output = v
		return nil
	}},
}

// FieldNames 按声明顺序返回全部配置字段名。
func FieldNames() []string {
	out := make([]string, len(fields))
	for i, f := range fields {
		out[i] = f.name
	}
	return out
}

func knownField(name string) bool {
	for _, f := range fields {
		if f.name == name {
			return true
		}
	}
	return false
}

// Load 从 JSON 读配置。语义是“以 Default() 为底，文件只覆盖出现的键”：
// 未出现的字段一律保持默认值。未知键、null、类型错误、越界值都返回 error，
// 不静默忽略，避免“配置写错但照常出图”。
func Load(r io.Reader) (Config, error) {
	cfg, _, err := LoadDetailed(r)
	return cfg, err
}

// LoadDetailed 同 Load，另外返回 provided：键名 -> 是否在文件中显式给出。
// 该信息只用于取值快照，不影响加载结果。
func LoadDetailed(r io.Reader) (Config, map[string]bool, error) {
	cfg := Default()
	provided := map[string]bool{}

	dec := json.NewDecoder(r)
	var raw map[string]json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		return cfg, provided, fmt.Errorf("配置解析失败: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return cfg, provided, fmt.Errorf("配置文件只能包含一个 JSON 对象")
	}
	if raw == nil { // 文件内容为 null
		return cfg, provided, fmt.Errorf("配置必须是 JSON 对象，不能为 null")
	}

	keys := make([]string, 0, len(raw))
	for k := range raw {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if !knownField(k) {
			return cfg, provided, fmt.Errorf("未知配置键 %q（合法键: %s）", k, strings.Join(FieldNames(), ", "))
		}
	}

	for _, f := range fields {
		rawMsg, ok := raw[f.name]
		if !ok {
			continue
		}
		if strings.TrimSpace(string(rawMsg)) == "null" {
			return cfg, provided, fmt.Errorf("键 %q 不能为 null；省略该键即使用默认值", f.name)
		}
		if err := f.set(&cfg, rawMsg); err != nil {
			return cfg, provided, fmt.Errorf("键 %q 取值非法: %w", f.name, err)
		}
		provided[f.name] = true
	}

	if err := cfg.Validate(); err != nil {
		return cfg, provided, err
	}
	return cfg, provided, nil
}

// LoadFile 从文件读配置，语义同 Load。
func LoadFile(path string) (Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return Default(), err
	}
	defer f.Close()
	return Load(f)
}

// Values 返回“一行一个字段”的取值清单（制表符分隔），顺序与字段声明一致。
// 该格式同时是 testdata/defaults_before.txt 的冻结基线格式。
func Values(c Config) string {
	var b strings.Builder
	for _, f := range fields {
		fmt.Fprintf(&b, "%s\t%s\n", f.name, f.get(c))
	}
	return b.String()
}

// Snapshot 返回带来源的取值快照：每行一个字段，形如
//
//	width = 64 [缺省]
//
// provided 来自 LoadDetailed；nil 表示全部按“缺省”标注。
func Snapshot(c Config, provided map[string]bool) string {
	var b strings.Builder
	for _, f := range fields {
		src := "缺省"
		if provided != nil && provided[f.name] {
			src = "显式"
		}
		val := f.get(c)
		if f.name == "shading" || f.name == "output" {
			val = quoted(val)
		}
		fmt.Fprintf(&b, "%s = %s [%s]\n", f.name, val, src)
	}
	return b.String()
}
