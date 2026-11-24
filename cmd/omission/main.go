// Command omission 执行配置默认值的逐字段缺省实验。
//
// 质疑点是“默认值到底生不生效，没人验过”。本工具把它拆成可证伪的实验：
//
//  1. 显式对照：9 个字段全部写进 JSON，值取自改动前代码冻结的默认值
//     （artifacts/defaults_before.txt，由 HEAD 的 config.Default() 导出）。
//  2. 单缺省：只省略被测字段，其余 8 个字段仍显式写出上述默认值。
//  3. 双缺省：省略被测字段 + 一个搭档字段，其余显式。
//  4. 全缺省：JSON 为 {}，9 个字段全部省略。
//
// 每个缺省产物都与显式对照图逐像素比较，差异必须为零。全缺省图另与
// 改动前代码渲染的同一场景逐字节比对。
//
// 显式值来自改动前的冻结文件而不是运行中的 Default()，实验不会自证：
// 若 Default() 被改坏，缺省图就会偏离显式图。
//
// 零差不是充分证据：两个取值本就可能产生同一张图。因此每个字段再做敏感性
// 对照（换成非默认值），只有换值确实改变了输出，“缺省=显式”的零差才说明
// 该字段真的被读到；否则如实标注“像素不可观测”，改由取值快照与单元测试支撑。
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"vecgo/config"
	"vecgo/geom"
	"vecgo/internal/probe"
	"vecgo/pipe"
)

// pairWith 是“双缺省”的搭档字段。原则：选一个在本场景中像素敏感、且与
// 被测字段处于不同处理阶段的字段，使双缺省测例不退化。
var pairWith = map[string]string{
	"width":       "shading",
	"height":      "shading",
	"scale":       "rotate",
	"rotate":      "supersample",
	"supersample": "gamma",
	"shading":     "blur",
	"gamma":       "blur",
	"blur":        "gamma",
	"output":      "rotate",
}

// kind 记录字段的 JSON 字面量类型，用于把冻结文件的值写回 JSON。
var kind = map[string]string{
	"width": "int", "height": "int", "scale": "float", "rotate": "float",
	"supersample": "int", "shading": "string", "gamma": "float",
	"blur": "int", "output": "string",
}

type caseRow struct {
	Case      string
	Omit      string
	Written   string
	Image     string
	SHA256    string
	PixDiff   int
	DimsOK    bool
	FirstDiff string
	Note      string
}

type fieldResult struct {
	Field       string
	Documented  string
	Live        string
	Single      caseRow
	Double      caseRow
	All         caseRow
	SensDef     probe.Sens
	SensWide    probe.Sens
	Observable  string
	Verdict     string
	Detail      string
	SameDefault bool // 冻结默认值与当前 Default() 是否同值
}

func main() {
	outDir := flag.String("out", "artifacts/omission", "输出目录")
	defPath := flag.String("defaults", "artifacts/defaults_before.txt", "改动前冻结的默认值文件")
	beforePath := flag.String("before", "artifacts/omission/00_before_HEAD.pgm", "改动前代码对实验场景的渲染产物")
	cmdBefore := flag.String("cmd-before", "artifacts/omission/00_before_HEAD_cmdscene.pgm", "改动前 cmd 默认场景的渲染产物")
	flag.Parse()
	if err := run(*outDir, *defPath, *beforePath, *cmdBefore); err != nil {
		fmt.Fprintln(os.Stderr, "失败:", err)
		os.Exit(1)
	}
}

func run(outDir, defPath, beforePath, cmdBefore string) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	docDefaults, err := readDefaults(defPath)
	if err != nil {
		return err
	}
	names := config.FieldNames()

	// 显式对照：9 个字段全部显式写出冻结默认值。
	expJSON, err := marshalExplicit(docDefaults, nil)
	if err != nil {
		return err
	}
	expCfg, expProvided, err := loadJSON(expJSON)
	if err != nil {
		return err
	}
	expImg, err := probe.Render(expCfg)
	if err != nil {
		return err
	}
	expPath := filepath.Join(outDir, "00_explicit.pgm")
	if err := expImg.SavePGM(expPath); err != nil {
		return err
	}
	writeText(filepath.Join(outDir, "00_explicit.json"), string(expJSON))
	writeText(filepath.Join(outDir, "00_explicit.snapshot.txt"), config.Snapshot(expCfg, expProvided))

	// 全缺省：{} 走正式加载路径。
	allJSON := []byte("{}\n")
	allCfg, allProvided, err := loadJSON(allJSON)
	if err != nil {
		return err
	}
	allImg, err := probe.Render(allCfg)
	if err != nil {
		return err
	}
	allPath := filepath.Join(outDir, "99_all_omitted.pgm")
	if err := allImg.SavePGM(allPath); err != nil {
		return err
	}
	writeText(filepath.Join(outDir, "99_all_omitted.json"), string(allJSON))
	writeText(filepath.Join(outDir, "99_all_omitted.snapshot.txt"), config.Snapshot(allCfg, allProvided))
	allDiff := probe.Compare(expImg, allImg)

	beforeAll := "未找到改动前产物，跳过基线比对"
	if b, err := os.ReadFile(beforePath); err == nil {
		cur := mustRead(allPath)
		if string(b) == string(cur) {
			beforeAll = "与改动前产物逐字节相同"
		} else if bi, err := probe.LoadPGM(beforePath); err == nil {
			d := probe.Compare(bi, allImg)
			beforeAll = fmt.Sprintf("与改动前产物不一致：%d 像素不同，首个 (%s)", d.N, firstDiffText(d))
		} else {
			beforeAll = "与改动前产物字节不一致（基线无法解码）"
		}
	}

	results := make([]fieldResult, 0, len(names))
	for _, name := range names {
		fr := fieldResult{
			Field:      name,
			Documented: docDefaults[name],
			Live:       fieldValue(allCfg, name),
		}
		fr.SameDefault, err = sameValue(name, docDefaults[name], fr.Live)
		if err != nil {
			return err
		}
		for _, cs := range []struct {
			kind  string
			omits []string
		}{
			{"single", []string{name}},
			{"double", []string{name, pairWith[name]}},
		} {
			omit := map[string]bool{}
			for _, k := range cs.omits {
				omit[k] = true
			}
			j, err := marshalExplicit(docDefaults, omit)
			if err != nil {
				return err
			}
			cfg, provided, err := loadJSON(j)
			if err != nil {
				return fmt.Errorf("%s/%s 加载失败: %w", name, cs.kind, err)
			}
			im, err := probe.Render(cfg)
			if err != nil {
				return err
			}
			base := name + "_" + cs.kind
			imgPath := filepath.Join(outDir, base+".pgm")
			if err := im.SavePGM(imgPath); err != nil {
				return err
			}
			writeText(filepath.Join(outDir, base+".json"), string(j))
			writeText(filepath.Join(outDir, base+".snapshot.txt"), config.Snapshot(cfg, provided))
			d := probe.Compare(expImg, im)
			r := caseRow{
				Case: cs.kind, Omit: strings.Join(cs.omits, ","),
				Written: strings.Join(writtenKeys(provided), ","),
				Image:   base + ".pgm", SHA256: sha256Hex(mustRead(imgPath)),
				PixDiff: d.N, DimsOK: d.DimsMatch, FirstDiff: firstDiffText(d),
				Note: omittedNote(provided, name),
			}
			if cs.kind == "single" {
				fr.Single = r
			} else {
				fr.Double = r
			}
		}
		fr.All = caseRow{
			Case: "all", Omit: strings.Join(names, ","), Written: "",
			Image: "99_all_omitted.pgm", SHA256: sha256Hex(mustRead(allPath)),
			PixDiff: allDiff.N, DimsOK: allDiff.DimsMatch, FirstDiff: firstDiffText(allDiff),
			Note: "全缺省图由 9 个字段共享",
		}

		defSens, err := probe.Sensitivity(name, allCfg)
		if err != nil {
			return err
		}
		wideSens, err := probe.Sensitivity(name, wideContext())
		if err != nil {
			return err
		}
		fr.SensDef, fr.SensWide = defSens, wideSens

		// 宽上下文敏感性产物保留，便于复核。
		alt, err := probe.AltFor(name, wideContext())
		if err != nil {
			return err
		}
		altCfg, err := probe.WithAlt(wideContext(), name, alt)
		if err != nil {
			return err
		}
		altImg, err := probe.Render(altCfg)
		if err != nil {
			return err
		}
		widePath := filepath.Join(outDir, name+"_sensitivity_wide.pgm")
		if err := altImg.SavePGM(widePath); err != nil {
			return err
		}
		writeDefJSON(filepath.Join(outDir, name+"_sensitivity_wide.json"), altCfg)

		fr.Observable = observability(fr.SensDef, fr.SensWide)
		fr.Verdict, fr.Detail = verdict(fr)
		results = append(results, fr)
	}

	// 两个特殊字段的独立旁证。
	scaleDiffs, transformHonors, err := probe.ScaleProbe()
	if err != nil {
		return err
	}
	gammaBad, gammaExamples, err := probe.GammaProbe()
	if err != nil {
		return err
	}

	all := caseRow{
		Case: "all", Omit: strings.Join(names, ","), Written: "",
		Image: "99_all_omitted.pgm", SHA256: sha256Hex(mustRead(allPath)),
		PixDiff: allDiff.N, DimsOK: allDiff.DimsMatch, FirstDiff: firstDiffText(allDiff),
		Note: beforeAll,
	}
	explicitRow := caseRow{
		Case: "explicit", Omit: "-",
		Written: strings.Join(writtenKeys(expProvided), ","),
		Image:   "00_explicit.pgm", SHA256: sha256Hex(mustRead(expPath)),
		PixDiff: 0, DimsOK: true, FirstDiff: "-",
		Note: "显式值来自改动前冻结文件 " + defPath,
	}
	// cmd 默认场景的改动前/后对照：证明默认渲染路径未被改动影响。
	cmdScene := [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}}
	cmdNow, err := pipe.Run(cmdScene, config.Default())
	if err != nil {
		return err
	}
	cmdNowPath := filepath.Join(outDir, "00_after_HEAD_cmdscene.pgm")
	if err := cmdNow.SavePGM(cmdNowPath); err != nil {
		return err
	}
	cmdNote := "未找到改动前 cmd 场景产物，跳过比对"
	if bi, err := probe.LoadPGM(cmdBefore); err == nil {
		d := probe.Compare(bi, cmdNow)
		if d.Equal() {
			cmdNote = "与改动前 cmd 默认场景产物逐像素相同（" + sha256Hex(mustRead(cmdNowPath))[:16] + "）"
		} else {
			cmdNote = fmt.Sprintf("与改动前 cmd 默认场景产物不同：%d 像素", d.N)
		}
	}

	extras := reportExtras{
		ScaleDiffs:      scaleDiffs,
		TransformHonors: transformHonors,
		GammaMismatch:   gammaBad,
		GammaExamples:   gammaExamples,
		CmdSceneNote:    cmdNote,
	}

	if err := writeReport(outDir, results, explicitRow, all, extras); err != nil {
		return err
	}
	fmt.Printf("产物目录: %s\n", outDir)
	return nil
}

// --- 加载与 JSON 组装 ---

// readDefaults 读取改动前冻结的 key<TAB>value 文件。
func readDefaults(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("%s: 无法解析行 %q", path, line)
		}
		out[parts[0]] = parts[1]
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	for _, name := range config.FieldNames() {
		if _, ok := out[name]; !ok {
			return nil, fmt.Errorf("%s 缺少字段 %q", path, name)
		}
	}
	return out, nil
}

// parseValue 按字段类型把冻结文件里的字面量转成 JSON 可用的 Go 值。
func parseValue(name, s string) (any, error) {
	switch kind[name] {
	case "int":
		return strconv.Atoi(strings.TrimSpace(s))
	case "float":
		return strconv.ParseFloat(strings.TrimSpace(s), 64)
	case "string":
		t := strings.TrimSpace(s)
		if strings.HasPrefix(t, `"`) {
			return strconv.Unquote(t)
		}
		return t, nil // Snapshot/Values 里的裸字符串

	default:
		return nil, fmt.Errorf("未知字段类型: %s", name)
	}
}

// marshalExplicit 生成“除 omit 外全部显式”的 JSON 对象，值来自 docDefaults。
func marshalExplicit(docDefaults map[string]string, omit map[string]bool) ([]byte, error) {
	m := map[string]any{}
	for _, name := range config.FieldNames() {
		if omit[name] {
			continue
		}
		v, err := parseValue(name, docDefaults[name])
		if err != nil {
			return nil, fmt.Errorf("字段 %s 的冻结默认值 %q 非法: %w", name, docDefaults[name], err)
		}
		m[name] = v
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

func loadJSON(b []byte) (config.Config, map[string]bool, error) {
	return config.LoadDetailed(strings.NewReader(string(b)))
}

func writtenKeys(provided map[string]bool) []string {
	keys := make([]string, 0, len(provided))
	for k := range provided {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// --- 小工具 ---

func fieldValue(c config.Config, name string) string {
	for _, line := range strings.Split(strings.TrimRight(config.Values(c), "\n"), "\n") {
		if strings.HasPrefix(line, name+"\t") {
			v := strings.TrimPrefix(line, name+"\t")
			if kind[name] == "string" {
				return strconv.Quote(v)
			}
			return v
		}
	}
	return ""
}

// sameValue 按字段类型归一化后比较冻结默认值与运行中取值，
// 避免字符串字面量的引号差异造成假阳性。
func sameValue(name, doc, live string) (bool, error) {
	dv, err := parseValue(name, doc)
	if err != nil {
		return false, err
	}
	lv, err := parseValue(name, live)
	if err != nil {
		return false, err
	}
	switch kind[name] {
	case "int":
		return dv.(int) == lv.(int), nil
	case "float":
		return dv.(float64) == lv.(float64), nil
	case "string":
		return dv.(string) == lv.(string), nil
	}
	return false, fmt.Errorf("未知字段类型: %s", name)
}

func omittedNote(provided map[string]bool, name string) string {
	if provided[name] {
		return "警告：缺省字段被标记为显式提供"
	}
	return "缺省字段由 Default() 补齐"
}

func firstDiffText(d probe.Diff) string {
	if !d.DimsMatch {
		return "尺寸不同"
	}
	if d.N == 0 {
		return "-"
	}
	return fmt.Sprintf("%d,%d", d.FirstX, d.FirstY)
}

func mustRead(path string) []byte {
	b, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	return b
}

func writeText(path, s string) {
	if err := os.WriteFile(path, []byte(s), 0o644); err != nil {
		panic(err)
	}
}

func writeDefJSON(path string, c config.Config) {
	b, err := json.MarshalIndent(map[string]any{
		"width": c.Width, "height": c.Height, "scale": c.Scale, "rotate": c.Rotate,
		"supersample": c.Supersample, "shading": c.Shading, "gamma": c.Gamma,
		"blur": c.Blur, "output": c.Output,
	}, "", "  ")
	if err != nil {
		panic(err)
	}
	writeText(path, string(append(b, '\n')))
}
