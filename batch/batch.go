// Package batch 提供逐场景隔离的批量渲染：单个场景失败（错误或 panic）
// 只记账不中断，其余场景继续，最后汇总成功与失败清单。
package batch

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"vecgo/config"
	"vecgo/geom"
	"vecgo/pipe"
)

// 记账表中的错误码。
const (
	CodeOK     = 0 // 成功
	CodeRender = 1 // 渲染管线返回错误
	CodePanic  = 2 // 渲染过程 panic（仅隔离开启时能被记账）
	CodeSave   = 3 // 产物导出失败
)

// Scene 是一个待渲染场景。
type Scene struct {
	Name  string
	Polys [][]geom.Point
}

// Result 是记账表的一行：场景名、错误码、耗时。
type Result struct {
	Name   string
	Code   int
	Err    error
	Dur    time.Duration
	Output string // 成功时的产物路径，失败为空
}

// runOne 渲染并导出单个场景。isolate 为 true 时 recover panic 并记为
// CodePanic；为 false 时不 recover，panic 原样传播（与原行为一致）。
// 隔离只是外层的记账包装，不触碰渲染输入与像素，产物与逐个单跑逐字节相同。
func runOne(sc Scene, cfg config.Config, outPath string, isolate bool) (res Result) {
	res = Result{Name: sc.Name, Code: CodeOK, Output: outPath}
	start := time.Now()
	defer func() {
		res.Dur = time.Since(start)
		if isolate {
			if r := recover(); r != nil {
				res.Code = CodePanic
				res.Err = fmt.Errorf("panic: %v", r)
				res.Output = ""
			}
		}
	}()
	im, err := pipe.Run(sc.Polys, cfg)
	if err != nil {
		res.Code = CodeRender
		res.Err = err
		res.Output = ""
		return res
	}
	if err := im.SavePGM(outPath); err != nil {
		res.Code = CodeSave
		res.Err = err
		res.Output = ""
		return res
	}
	return res
}

// RunBatch 逐场景渲染并导出到 outDir/<场景名>.pgm。
// isolate 开启（默认）：失败跳过并记账，其余继续，返回全部结果。
// isolate 关闭：首个失败即中断批次并返回该错误，panic 直接崩溃，与原行为一致。
func RunBatch(scenes []Scene, cfg config.Config, outDir string, isolate bool) ([]Result, error) {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, err
	}
	results := make([]Result, 0, len(scenes))
	for _, sc := range scenes {
		out := filepath.Join(outDir, sc.Name+".pgm")
		res := runOne(sc, cfg, out, isolate)
		results = append(results, res)
		if !isolate && res.Code != CodeOK {
			return results, res.Err
		}
	}
	return results, nil
}

// WriteReport 打印记账表（一行一个场景：场景名、错误码、耗时），
// 随后打印成功与失败清单，并用一句话解释批次总耗时与逐场景耗时之和的差。
func WriteReport(w io.Writer, results []Result, total time.Duration) {
	fmt.Fprintln(w, "场景\t错误码\t耗时")
	var sum time.Duration
	var ok, failed []string
	for _, r := range results {
		fmt.Fprintf(w, "%s\t%d\t%s\n", r.Name, r.Code, r.Dur)
		sum += r.Dur
		if r.Code == CodeOK {
			ok = append(ok, r.Name)
		} else {
			failed = append(failed, fmt.Sprintf("%s(%v)", r.Name, r.Err))
		}
	}
	fmt.Fprintf(w, "成功(%d): %s\n", len(ok), joinOr(ok, "无"))
	fmt.Fprintf(w, "失败(%d): %s\n", len(failed), joinOr(failed, "无"))
	fmt.Fprintf(w, "批次总耗时 %s，逐场景耗时合计 %s，差值 %s 来自建目录、逐场景计时与记账汇总等批次级固定开销，与渲染本身无关。\n",
		total, sum, total-sum)
}

func joinOr(ss []string, fallback string) string {
	if len(ss) == 0 {
		return fallback
	}
	out := ss[0]
	for _, s := range ss[1:] {
		out += ", " + s
	}
	return out
}
