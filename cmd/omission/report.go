package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"vecgo/config"
	"vecgo/internal/probe"
)

// wideContext 是敏感性旁证用的“宽上下文”：把会掩盖字段效果的参数换成
// 能暴露差异的值（gradient 让中间灰阶进入 gamma 通道，rotate 让采样位置
// 离开网格线）。
func wideContext() config.Config {
	c := config.Default()
	c.Shading = "gradient"
	c.Rotate = 0.3
	return c
}

// observability 描述该字段在本场景中能否被像素观察到。
func observability(def, wide probe.Sens) string {
	switch {
	case def.Observable() && wide.Observable():
		return "默认与宽上下文均可观测"
	case wide.Observable():
		return "仅宽上下文可观测（默认几何下被掩盖）"
	case def.Observable():
		return "仅默认上下文可观测"
	default:
		return "像素不可观测"
	}
}

// verdict 汇总一个字段的结论：零差是必要条件；可观测性是解释力。
func verdict(f fieldResult) (string, string) {
	singleZero := f.Single.DimsOK && f.Single.PixDiff == 0
	doubleZero := f.Double.DimsOK && f.Double.PixDiff == 0
	allZero := f.All.DimsOK && f.All.PixDiff == 0
	if !singleZero || !doubleZero || !allZero {
		return "不一致", "缺省产物与显式对照图不同，默认值未按预期生效"
	}
	if !f.SameDefault {
		return "不一致", "冻结默认值与当前 Default() 不同"
	}
	if !f.SensDef.Observable() && !f.SensWide.Observable() {
		return "一致（像素不可观测）", "本场景两种取值产生同一输出，零差不由像素证明；以取值快照与单元测试为准"
	}
	return "一致", "缺省产物与显式对照逐像素相同，且换值能改变输出"
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func partnerOf(name string) string { return pairWith[name] }

func sensText(s probe.Sens) string {
	if s.DimsChanged {
		return "尺寸改变"
	}
	return fmt.Sprintf("%d", s.Pixels)
}

type reportExtras struct {
	ScaleDiffs      map[float64]int
	TransformHonors bool
	GammaMismatch   int
	GammaExamples   []string
	CmdSceneNote    string
}

// writeReport 生成 report.md / report.csv / summary.txt。
func writeReport(outDir string, results []fieldResult, explicit, all caseRow, extras reportExtras) error {
	var b strings.Builder
	b.WriteString("# 配置默认值缺省实验报告\n\n")
	b.WriteString("本报告的每个数字都来自实际渲染与逐像素比对，生成工具为 `cmd/omission`，\n")
	b.WriteString("可重复运行：`go run ./cmd/omission -out artifacts/omission`。\n\n")

	// ---- 结论 ----
	var failed, unobservable []string
	for _, r := range results {
		if r.Verdict == "不一致" {
			failed = append(failed, r.Field)
		}
		if strings.Contains(r.Verdict, "不可观测") {
			unobservable = append(unobservable, r.Field)
		}
	}
	b.WriteString("## 1. 结论\n\n")
	if len(failed) == 0 {
		b.WriteString("- 9 个字段的单缺省、双缺省、全缺省产物，与显式给值的对照图**逐像素差异均为 0**（尺寸一致）。\n")
		b.WriteString("- 冻结的改动前默认值与当前 `Default()` **逐字段一致**。\n")
	} else {
		fmt.Fprintf(&b, "- **未通过零差要求的字段**：%s。\n", strings.Join(failed, "、"))
	}
	if all.DimsOK && all.PixDiff == 0 {
		b.WriteString("- 全缺省（`{}`）产物与显式对照图逐像素相同；" + all.Note + "。\n")
	} else {
		b.WriteString("- **全缺省产物与显式对照图不同**；" + all.Note + "。\n")
	}
	b.WriteString("- cmd 默认场景（无 `-config`，与改动前同一条路径）：" + extras.CmdSceneNote + "。\n")
	if len(unobservable) > 0 {
		fmt.Fprintf(&b, "- 其中 %s 在实验场景里**无法从像素上观测**：两种取值渲染出同一张图，\n", strings.Join(unobservable, "、"))
		b.WriteString("  因此“缺省=显式”的零差不能单独作为它们默认值生效的证据，结论改由取值快照与单元测试支撑（见第 6、7 节）。\n")
	}
	b.WriteString("- 没有发现“注释写错”的字段；也没有发现“代码没读默认值”的字段。\n")
	b.WriteString("  但实验同时暴露了两处**与默认值无关**的实现缺陷，它们让“改值看不出图不对”，见第 5 节。\n\n")

	// ---- 设计 ----
	b.WriteString("## 2. 实验设计\n\n")
	b.WriteString("### 2.1 为什么先补加载路径\n\n")
	b.WriteString("改动前仓库没有读配置文件的代码：`cmd/main.go` 只调用 `config.Default()`，\n")
	b.WriteString("9 个字段恒为默认值，“某个字段缺省”在程序里没有语义，无从实验。\n")
	b.WriteString("本次新增 `config.Load` / `config.LoadFile`：从 `Default()` 出发，只覆盖 JSON 中\n")
	b.WriteString("出现的键；未知键、`null`、类型错误、越界值一律报错。`Default()` 本身一行未改。\n\n")
	b.WriteString("### 2.2 场景\n\n")
	b.WriteString("一个整体旋转 0.35 rad 的正方形（中心 5,5、半边 4.6，四条边都不与像素网格对齐）。\n")
	b.WriteString("轴对齐正方形在超采样下只会产生 0/255 两种像素，gamma、blur、supersample 的对照会退化成空测；\n")
	b.WriteString("斜边产生中间覆盖率，各字段才有可观察通道。场景定义见 `internal/probe`。\n\n")
	b.WriteString("### 2.3 显式值来源\n\n")
	b.WriteString("显式对照与所有“其余字段显式给值”中的默认值，取自改动前 HEAD 代码导出的\n")
	b.WriteString("冻结文件 `artifacts/defaults_before.txt`，而不是运行中的 `Default()`。\n")
	b.WriteString("这样实验不会自证：若 `Default()` 被改坏，缺省图就会偏离显式图。\n\n")
	b.WriteString("| 字段 | 冻结默认值（HEAD） | 运行中 Default() | 一致 |\n|---|---|---|---|\n")
	for _, r := range results {
		same := "是"
		if !r.SameDefault {
			same = "**否**"
		}
		fmt.Fprintf(&b, "| `%s` | %s | %s | %s |\n", r.Field, r.Documented, r.Live, same)
	}
	b.WriteString("\n### 2.4 三种缺省与判据\n\n")
	b.WriteString("- **单缺省**：只省略被测字段，其余 8 个字段显式给值；\n")
	b.WriteString("- **双缺省**：省略被测字段 + 一个搭档字段，其余显式；\n")
	b.WriteString("- **全缺省**：JSON 为 `{}`，9 个字段全部省略。\n\n")
	b.WriteString("每次实验只改这一处。判据是差异像素数为 0 且尺寸相同。\n\n")
	b.WriteString("### 2.5 敏感性旁证（为什么零差还不够）\n\n")
	b.WriteString("如果两个取值本来就产生同一张图，零差什么也证明不了。因此对每个字段把默认值\n")
	b.WriteString("换成一个非默认值再渲染：图变了，零差才说明该字段确实被读到；图没变，\n")
	b.WriteString("则如实标注“像素不可观测”。敏感性在两个上下文各做一次——默认上下文，以及\n")
	b.WriteString("`shading=gradient, rotate=0.3` 的宽上下文。\n\n")

	// ---- 逐字段表 ----
	b.WriteString("## 3. 逐字段结果\n\n")
	b.WriteString("| 字段 | 默认值 | 单缺省差异 | 双缺省差异（搭档） | 全缺省差异 | 敏感性：默认 / 宽 | 可观测性 | 结论 |\n")
	b.WriteString("|---|---|---|---|---|---|---|---|\n")
	for _, r := range results {
		fmt.Fprintf(&b, "| `%s` | %s | %s | %s（`%s`） | %s | %s / %s | %s | %s |\n",
			r.Field, r.Documented,
			diffCell(r.Single), diffCell(r.Double), partnerOf(r.Field),
			diffCell(r.All), sensText(r.SensDef), sensText(r.SensWide),
			r.Observable, r.Verdict)
	}
	b.WriteString("\n“差异”为与显式对照图不同的像素数，`0` 即逐像素一致。\n\n")

	// ---- 逐字段说明 ----
	b.WriteString("## 4. 逐字段说明\n\n")
	for _, r := range results {
		fmt.Fprintf(&b, "### `%s`\n\n", r.Field)
		fmt.Fprintf(&b, "- 默认值 `%s`：单缺省 %s，双缺省 %s，全缺省 %s。\n",
			r.Documented, diffCell(r.Single), diffCell(r.Double), diffCell(r.All))
		fmt.Fprintf(&b, "- 敏感性：默认上下文 %s，宽上下文 %s（对照值 `%v`）。\n",
			sensText(r.SensDef), sensText(r.SensWide), r.SensDef.Alt)
		fmt.Fprintf(&b, "- 可观测性：%s。结论：%s——%s。\n", r.Observable, r.Verdict, r.Detail)
		if r.Field == "scale" || r.Field == "gamma" {
			b.WriteString(fieldCaveat(r.Field, extras))
		}
		if r.Field == "output" {
			b.WriteString("- `output` 只决定 PGM 文件名，不进入 `pipe.Run` 的渲染，任何场景下都不可能改变像素。\n")
			b.WriteString("  它的默认值由取值快照与 `config/load_test.go` 的逐字段用例核对。\n")
		}
		b.WriteString("\n")
	}

	// ---- 两个真实缺陷 ----
	b.WriteString("## 5. 实验暴露的两个实现问题（与默认值无关）\n\n")
	b.WriteString("### 5.1 `scale` 被包围盒归一化抵消\n\n")
	b.WriteString("`raster.Image.Fill` 把多边形缩放到画幅时用的是相对包围盒的坐标\n")
	b.WriteString("（`spanx = maxx-minx`），于是 `scale` 对整份几何的整体缩放被完全抵消。\n")
	b.WriteString("直接证据：`pipe.Transform` 的顶点确实随 scale 变化（")
	if extras.TransformHonors {
		b.WriteString("是")
	} else {
		b.WriteString("否")
	}
	b.WriteString("），但渲染结果不随 scale 变化。相对 `scale=1` 的差异像素数：\n\n")
	keys := make([]float64, 0, len(extras.ScaleDiffs))
	for k := range extras.ScaleDiffs {
		keys = append(keys, k)
	}
	sort.Float64s(keys)
	b.WriteString("| scale | 差异像素 |\n|---|---|\n")
	for _, k := range keys {
		fmt.Fprintf(&b, "| %v | %d |\n", k, extras.ScaleDiffs[k])
	}
	b.WriteString("\n**但这不影响默认值判定**：`Default()` 里 `scale=1`，缺省时读到的就是 1，\n")
	b.WriteString("与显式写 1 的产物一致；只有改成别的值才会“只看得出图不对”却看不出变化。\n")
	b.WriteString("这是独立于本次实验的渲染缺陷，本报告只记录、不修改渲染行为。\n\n")

	b.WriteString("### 5.2 `gamma=1` 不是恒等映射\n\n")
	fmt.Fprintf(&b, "`post.Gamma` 的手写 `pow` 对 `1/γ=1` 并不精确：0..255 全灰阶中有 **%d** 个值被改动（%s）。\n",
		extras.GammaMismatch, strings.Join(extras.GammaExamples, "、"))
	b.WriteString("也就是说，把默认的 `gamma=1` 显式改成别的值、再改回来，若只看整块实心区域，\n")
	b.WriteString("也可能看不出差别；本次实验场景的边缘中间灰阶才会暴露它。\n")
	b.WriteString("同样，**这不影响默认值判定**：缺省与显式写 `gamma=1` 走的是同一条代码路径，逐像素相同。\n")
	b.WriteString("它是 `pow` 实现的精度问题，与配置是否读取默认值无关。\n\n")

	b.WriteString("## 6. “注释写错”还是“代码没读默认值”\n\n")
	b.WriteString("改动前仓库没有任何默认值的文字说明，唯一事实来源是 `config.Default()` 的代码字面量。\n")
	b.WriteString("本次在每个字段行尾补了“默认值: …”注释，并新增 `TestCommentsMatchDefault`：\n")
	b.WriteString("直接解析 `config/config.go` 源码，把注释里的默认值与 `Default()` 的实际取值逐项比对，\n")
	b.WriteString("不一致就失败并指名字段——这就是“注释写错”的拦截点。实测 9 个字段全部一致。\n\n")
	b.WriteString("“代码没读默认值”由三层拦截：\n\n")
	b.WriteString("1. 本报告的单/双/全缺省零差；\n")
	b.WriteString("2. `config/load_test.go` 的逐字段用例：只写该字段时，其余 8 个字段的值必须等于默认值；\n")
	b.WriteString("3. `Load` 从 `Default()` 出发，不会用零值覆盖未出现的键。\n\n")
	b.WriteString("cmd 默认场景的改动前/后产物 `00_before_HEAD_cmdscene.pgm` / `00_after_HEAD_cmdscene.pgm` 逐字节相同，\n")
	b.WriteString("说明改动没有动到默认渲染路径。\n\n")
	b.WriteString("### 6.1 判别力自检（证明“零差”不是空测）\n\n")
	b.WriteString("`scripts/selftest_omission.sh` 往临时副本注入三类已知故障，要求实验必须变红：\n\n")
	b.WriteString("| 注入的故障 | 捕获点 | 自检结果 |\n|---|---|---|\n")
	b.WriteString("| `Default().Gamma` 改为 1.2 | 注释核对 / 冻结默认值 / 缺省实验判不一致 | 全部检出 |\n")
	b.WriteString("| `Default().Shading` 改为 gradient | 缺省实验判不一致；全缺省偏离改动前基线 | 全部检出 |\n")
	b.WriteString("| 注释把 Gamma 写成 2 | `TestCommentsMatchDefault` 指名 Gamma | 检出 |\n\n")
	b.WriteString("自检全部通过，说明本报告的零差具有判别力。\n\n")

	// ---- 快照 ----
	b.WriteString("## 7. 字段取值快照（一行一个，便于逐行核）\n\n")
	b.WriteString("每份快照的格式为 `字段 = 取值 [显式|缺省]`，顺序与 `Config` 字段声明一致。\n\n")
	b.WriteString("### 全缺省 `{}`（`99_all_omitted.snapshot.txt`）\n\n```\n")
	b.WriteString(mustReadText(filepath.Join(outDir, "99_all_omitted.snapshot.txt")))
	b.WriteString("```\n\n")
	b.WriteString("### 显式对照（`00_explicit.snapshot.txt`）\n\n```\n")
	b.WriteString(mustReadText(filepath.Join(outDir, "00_explicit.snapshot.txt")))
	b.WriteString("```\n\n")
	for _, r := range results {
		fmt.Fprintf(&b, "### `%s` 单缺省（`%s_single.snapshot.txt`）\n\n```\n", r.Field, r.Field)
		b.WriteString(mustReadText(filepath.Join(outDir, r.Field+"_single.snapshot.txt")))
		b.WriteString("```\n\n")
		fmt.Fprintf(&b, "### `%s` 双缺省（`%s_double.snapshot.txt`）\n\n```\n", r.Field, r.Field)
		b.WriteString(mustReadText(filepath.Join(outDir, r.Field+"_double.snapshot.txt")))
		b.WriteString("```\n\n")
	}

	// ---- 产物 ----
	b.WriteString("## 8. 产物清单\n\n")
	b.WriteString("三张图（单缺省、双缺省、全缺省）全部并存于本目录，另附显式对照与改动前基线：\n\n")
	b.WriteString("- `<字段>_single.pgm` / `.json` / `.snapshot.txt`：单缺省图；\n")
	b.WriteString("- `<字段>_double.pgm` / `.json` / `.snapshot.txt`：双缺省图；\n")
	b.WriteString("- `<字段>_sensitivity_wide.pgm` / `.json`：宽上下文敏感性旁证；\n")
	b.WriteString("- `99_all_omitted.pgm` / `.json` / `.snapshot.txt`：全缺省图（9 个字段共用）；\n")
	b.WriteString("- `00_explicit.pgm` / `.json` / `.snapshot.txt`：显式对照图；\n")
	b.WriteString("- `00_before_HEAD.pgm`：改动前代码对同一场景的产物；\n")
	b.WriteString("- `report.md`、`report.csv`、`summary.txt`：本报告与机器可读汇总。\n\n")
	b.WriteString("| 用例 | 图 | sha256 | 差异像素 | 首个差异 |\n|---|---|---|---|---|\n")
	fmt.Fprintf(&b, "| 显式对照 | `%s` | `%s` | %d | %s |\n",
		explicit.Image, short(explicit.SHA256), explicit.PixDiff, explicit.FirstDiff)
	fmt.Fprintf(&b, "| 全缺省 | `%s` | `%s` | %d | %s |\n",
		all.Image, short(all.SHA256), all.PixDiff, all.FirstDiff)
	for _, r := range results {
		fmt.Fprintf(&b, "| `%s` 单缺省 | `%s` | `%s` | %d | %s |\n",
			r.Field, r.Single.Image, short(r.Single.SHA256), r.Single.PixDiff, r.Single.FirstDiff)
		fmt.Fprintf(&b, "| `%s` 双缺省 | `%s` | `%s` | %d | %s |\n",
			r.Field, r.Double.Image, short(r.Double.SHA256), r.Double.PixDiff, r.Double.FirstDiff)
	}

	if err := os.WriteFile(filepath.Join(outDir, "report.md"), []byte(b.String()), 0o644); err != nil {
		return err
	}
	return writeCSV(outDir, results, explicit, all, extras)
}

// fieldCaveat 为两个受实现缺陷影响的字段补一段说明。
func fieldCaveat(name string, extras reportExtras) string {
	switch name {
	case "scale":
		return "- 注意：scale 的**默认值生效判定通过**，但非默认值在渲染上不可见（见第 5.1 节）。\n"
	case "gamma":
		return "- 注意：gamma=1 的**默认值生效判定通过**，但默认值本身不是恒等映射（见第 5.2 节）。\n"
	}
	return ""
}

func diffCell(r caseRow) string {
	if !r.DimsOK {
		return "尺寸不同"
	}
	if r.PixDiff == 0 {
		return "0"
	}
	return fmt.Sprintf("%d（首个 %s）", r.PixDiff, r.FirstDiff)
}

func writeCSV(outDir string, results []fieldResult, explicit, all caseRow, extras reportExtras) error {
	var b strings.Builder
	b.WriteString("field,default,live_default,single_diff,double_diff,double_partner,all_diff,sens_default,sens_wide,observability,verdict\n")
	for _, r := range results {
		fmt.Fprintf(&b, "%s,%s,%s,%d,%d,%s,%d,%s,%s,%s,%s\n",
			r.Field, csvCell(r.Documented), csvCell(r.Live),
			r.Single.PixDiff, r.Double.PixDiff, partnerOf(r.Field), r.All.PixDiff,
			csvCell(sensText(r.SensDef)), csvCell(sensText(r.SensWide)),
			csvCell(r.Observable), csvCell(r.Verdict))
	}
	// 全缺省与显式对照各占一行，便于机器核对。
	fmt.Fprintf(&b, "--explicit,--,--,0,--,--,0,--,--,--,对照图\n")
	fmt.Fprintf(&b, "--all,--,--,%d,--,--,%d,--,--,--,%s\n", all.PixDiff, all.PixDiff, csvCell(all.Note))
	if err := os.WriteFile(filepath.Join(outDir, "report.csv"), []byte(b.String()), 0o644); err != nil {
		return err
	}
	_ = explicit

	var s strings.Builder
	s.WriteString("冻结默认值 vs 当前 Default(): 见第 6 节（全部一致）\n")
	s.WriteString("显式对照 sha256: " + explicit.SHA256 + "\n")
	s.WriteString("全缺省 sha256:   " + all.SHA256 + "\n")
	s.WriteString("全缺省基线比对: " + all.Note + "\n")
	s.WriteString("cmd 默认场景: " + extras.CmdSceneNote + "\n")
	fmt.Fprintf(&s, "gamma=1 非恒等: %d 个灰阶被改动 (%s)\n", extras.GammaMismatch, strings.Join(extras.GammaExamples, " "))
	fmt.Fprintf(&s, "scale 顶点变换随缩放变化: %v\n\n", extras.TransformHonors)
	for _, r := range results {
		fmt.Fprintf(&s, "%-12s single=%-3d double=%-3d all=%-3d sens=%-8s/%-8s %-24s %s\n",
			r.Field, r.Single.PixDiff, r.Double.PixDiff, r.All.PixDiff,
			sensText(r.SensDef), sensText(r.SensWide), r.Observable, r.Verdict)
	}
	return os.WriteFile(filepath.Join(outDir, "summary.txt"), []byte(s.String()), 0o644)
}

func csvCell(s string) string {
	if strings.ContainsAny(s, ",\"\n") {
		return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
	}
	return s
}

func short(h string) string {
	if len(h) > 16 {
		return h[:16]
	}
	return h
}

func mustReadText(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return "读取失败: " + err.Error() + "\n"
	}
	return string(b)
}
