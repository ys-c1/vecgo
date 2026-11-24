package config

import (
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// TestCommentsMatchDefault 直接解析 config.go 源码：Config 每个字段行尾注释
// 里的“默认值: X”必须与 Default() 的实际取值一致。这是“注释写错”的拦截点。
func TestCommentsMatchDefault(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "config.go", nil, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	def := Default()

	var structComments, defaultComments map[string]string
	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.TypeSpec:
			if x.Name.Name == "Config" {
				st, ok := x.Type.(*ast.StructType)
				if !ok {
					return true
				}
				structComments = fieldComments(fset, st)
			}
		case *ast.FuncDecl:
			if x.Name.Name == "Default" {
				defaultComments = literalComments(fset, f.Comments, x)
			}
		}
		return true
	})
	if structComments == nil || defaultComments == nil {
		t.Fatal("未找到 Config 结构体或 Default()")
	}

	v := reflect.ValueOf(def)
	typ := v.Type()
	for i := 0; i < typ.NumField(); i++ {
		name := typ.Field(i).Name
		key := strings.ToLower(name)
		sc, ok := structComments[key]
		if !ok {
			t.Errorf("字段 %s 缺少“默认值: …”注释", name)
			continue
		}
		dc, ok := defaultComments[key]
		if !ok {
			t.Errorf("Default() 中字段 %s 缺少行尾“默认值: …”注释", name)
			continue
		}
		if sc != dc {
			t.Errorf("字段 %s 注释不一致: 结构体 %q, Default() %q", name, sc, dc)
			continue
		}
		if got := literalOf(v.Field(i)); got != sc {
			t.Errorf("字段 %s 注释写的是 %q，Default() 实际为 %q", name, sc, got)
		}
	}
}

// fieldComments 收集结构体字段行尾注释里的默认值字面量。
func fieldComments(fset *token.FileSet, st *ast.StructType) map[string]string {
	out := map[string]string{}
	for _, fl := range st.Fields.List {
		if fl.Comment == nil || len(fl.Names) != 1 {
			continue
		}
		if v, ok := parseDefaultComment(fl.Comment.Text()); ok {
			out[strings.ToLower(fl.Names[0].Name)] = v
		}
	}
	return out
}

// literalComments 收集 Default() 复合字面量每个元素的键与行尾注释。
// KeyValueExpr 自身不挂注释，这里按“注释起始行 == 元素结束行”匹配行尾注释。
func literalComments(fset *token.FileSet, groups []*ast.CommentGroup, fn *ast.FuncDecl) map[string]string {
	byLine := map[int]string{}
	for _, g := range groups {
		line := fset.Position(g.Pos()).Line
		byLine[line] = g.Text()
	}
	out := map[string]string{}
	ast.Inspect(fn, func(n ast.Node) bool {
		cl, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		for _, el := range cl.Elts {
			kv, ok := el.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			id, ok := kv.Key.(*ast.Ident)
			if !ok {
				continue
			}
			text, ok := byLine[fset.Position(kv.End()).Line]
			if !ok {
				continue
			}
			if v, ok := parseDefaultComment(text); ok {
				out[strings.ToLower(id.Name)] = v
			}
		}
		return true
	})
	return out
}

// parseDefaultComment 从 "// 默认值: 64" 里取出 "64"。
func parseDefaultComment(text string) (string, bool) {
	text = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(text), "//"))
	text = strings.TrimSpace(strings.TrimPrefix(text, "默认值:"))
	text = strings.TrimSpace(strings.TrimPrefix(text, "默认值："))
	if text == "" {
		return "", false
	}
	return text, true
}

// literalOf 按 Default() 注释的字面量风格渲染字段值。
func literalOf(v reflect.Value) string {
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(v.Int(), 10)
	case reflect.Float64:
		return strconv.FormatFloat(v.Float(), 'g', -1, 64)
	case reflect.String:
		return strconv.Quote(v.String())
	}
	return v.String()
}

// TestDefaultUnchanged 固定改动前的默认值。任何改动都必须显式更新此表。
func TestDefaultUnchanged(t *testing.T) {
	want := Config{
		Width: 64, Height: 64, Scale: 1, Rotate: 0, Supersample: 2,
		Shading: "flat", Gamma: 1, Blur: 0, Output: "out.pgm",
	}
	if got := Default(); got != want {
		t.Fatalf("Default() 被改动\n got=%+v\nwant=%+v", got, want)
	}
}

// TestLoadEmptyUsesDefaults 全缺省：{} 的每个字段都必须等于 Default()。
func TestLoadEmptyUsesDefaults(t *testing.T) {
	cfg, provided, err := LoadDetailed(strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg != Default() {
		t.Fatalf("{} 未得到默认配置:\n got=%+v\nwant=%+v", cfg, Default())
	}
	if len(provided) != 0 {
		t.Fatalf("{} 不应有显式字段: %v", provided)
	}
}

// TestLoadSingleField 逐字段：只写一个键，其余字段必须保持默认值。
// 覆盖“代码没读默认值”：若 Load 从零值 Config 出发或漏读某键，这里会失败。
func TestLoadSingleField(t *testing.T) {
	def := Default()
	cases := []struct {
		key   string
		value string
		check func(Config) bool
	}{
		{"width", `"width": 48`, func(c Config) bool { return c.Width == 48 }},
		{"height", `"height": 48`, func(c Config) bool { return c.Height == 48 }},
		{"scale", `"scale": 2`, func(c Config) bool { return c.Scale == 2 }},
		{"rotate", `"rotate": 0.5`, func(c Config) bool { return c.Rotate == 0.5 }},
		{"supersample", `"supersample": 1`, func(c Config) bool { return c.Supersample == 1 }},
		{"shading", `"shading": "gradient"`, func(c Config) bool { return c.Shading == "gradient" }},
		{"gamma", `"gamma": 2`, func(c Config) bool { return c.Gamma == 2 }},
		{"blur", `"blur": 1`, func(c Config) bool { return c.Blur == 1 }},
		{"output", `"output": "x.pgm"`, func(c Config) bool { return c.Output == "x.pgm" }},
	}
	for _, tc := range cases {
		cfg, provided, err := LoadDetailed(strings.NewReader("{" + tc.value + "}"))
		if err != nil {
			t.Fatalf("%s: %v", tc.key, err)
		}
		if !tc.check(cfg) {
			t.Fatalf("%s 未按文件取值: %+v", tc.key, cfg)
		}
		if !provided[tc.key] || len(provided) != 1 {
			t.Fatalf("%s 的 provided 标记错误: %v", tc.key, provided)
		}
		// 其余 8 个字段必须仍为默认值。
		got := reflect.ValueOf(cfg)
		want := reflect.ValueOf(def)
		for i := 0; i < got.NumField(); i++ {
			name := strings.ToLower(got.Type().Field(i).Name)
			if name == tc.key {
				continue
			}
			if !reflect.DeepEqual(got.Field(i).Interface(), want.Field(i).Interface()) {
				t.Fatalf("%s 缺省时字段 %s 应为默认值 %v，实际 %v",
					tc.key, name, want.Field(i).Interface(), got.Field(i).Interface())
			}
		}
	}
}

// TestLoadRejectsBadInput 未知键、null、类型错误、越界值都必须报错，不能静默回退默认值。
func TestLoadRejectsBadInput(t *testing.T) {
	cases := map[string]string{
		"未知键":  `{"widht": 32}`,
		"null": `{"blur": null}`,
		"类型错误": `{"width": "wide"}`,
		"越界":   `{"gamma": 0}`,
		"非对象":  `[1,2,3]`,
		"尾随内容": `{}{}`,
	}
	for name, doc := range cases {
		if _, err := Load(strings.NewReader(doc)); err == nil {
			t.Errorf("%s 应报错: %s", name, doc)
		}
	}
}

// TestLoadZeroIsNotNull 省略某字段与显式写 0 必须可区分：显式 0 走校验并报错。
func TestLoadZeroIsNotNull(t *testing.T) {
	if _, err := Load(strings.NewReader(`{"width": 0}`)); err == nil {
		t.Fatal("显式 width=0 应被 Validate 拒绝")
	}
	cfg, err := Load(strings.NewReader(`{"blur": 0, "rotate": 0}`))
	if err != nil {
		t.Fatalf("显式写合法零值不应报错: %v", err)
	}
	if cfg.Blur != 0 || cfg.Rotate != 0 {
		t.Fatalf("显式零值未生效: %+v", cfg)
	}
}

// TestSnapshotLines 快照必须一行一个字段，顺序与声明一致，且标出来源。
func TestSnapshotLines(t *testing.T) {
	cfg, provided, err := LoadDetailed(strings.NewReader(`{"gamma": 2}`))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(Snapshot(cfg, provided), "\n"), "\n")
	if len(lines) != len(FieldNames()) {
		t.Fatalf("快照行数 %d，期望 %d", len(lines), len(FieldNames()))
	}
	for i, name := range FieldNames() {
		if !strings.HasPrefix(lines[i], name+" = ") {
			t.Fatalf("第 %d 行应以 %q 开头: %q", i+1, name, lines[i])
		}
	}
	if !strings.Contains(lines[6], "[显式]") || !strings.Contains(lines[0], "[缺省]") {
		t.Fatalf("来源标注错误:\n%s", Snapshot(cfg, provided))
	}
}

// TestZeroValueBaseIsNotDefault 记录“为什么 Load 必须从 Default() 出发”。
// 若用零值 Config 作为底再解 JSON，省略的字段会变成 Go 零值：9 个字段里
// 有 7 个零值不等于默认值，其中 7 个还会被 Validate 拒绝；只有 Rotate、
// Blur 的零值恰好等于默认值。这正是“默认值没生效”的经典来源。
func TestZeroValueBaseIsNotDefault(t *testing.T) {
	def := Default()
	dv := reflect.ValueOf(def)
	zv := reflect.ValueOf(Config{})
	same, differ, invalid := 0, 0, 0
	var differNames []string
	for i := 0; i < dv.NumField(); i++ {
		name := strings.ToLower(dv.Type().Field(i).Name)
		if reflect.DeepEqual(dv.Field(i).Interface(), zv.Field(i).Interface()) {
			same++
			continue
		}
		differ++
		differNames = append(differNames, name)
		// 只把该字段置零，其余仍为默认值，看是否还能通过校验。
		full := def
		reflect.ValueOf(&full).Elem().Field(i).Set(zv.Field(i))
		if err := full.Validate(); err != nil {
			invalid++
		}
	}
	if same != 2 || differ != 7 {
		t.Fatalf("零值与默认值的关系变了：相同 %d、不同 %d（%v）", same, differ, differNames)
	}
	if invalid != 7 {
		t.Fatalf("零值导致校验失败的字段数应为 7，实际 %d", invalid)
	}
	for _, name := range []string{"rotate", "blur"} {
		if contains(differNames, name) {
			t.Fatalf("%s 的零值本应等于默认值，实际被算作不同: %v", name, differNames)
		}
	}
}

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}
