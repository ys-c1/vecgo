// Command pyrlab 运行金字塔层数—重建误差实验，输出 JSON 数据、SVG 曲线与
// Markdown 报告。用法：
//
//	go run ./cmd/pyrlab -out artifacts
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"vecgo/lab"
)

func main() {
	outDir := flag.String("out", "artifacts", "产物输出目录")
	repoDoc := flag.String("repo-doc", "pyramid", "包内文档目录（写 REPORT.md 与 decay.svg），置空跳过")
	flag.Parse()

	rep, err := lab.Run(nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "实验失败:", err)
		os.Exit(1)
	}

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "建目录失败:", err)
		os.Exit(2)
	}

	// JSON 原始数据
	js, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		panic(err)
	}
	jsonPath := filepath.Join(*outDir, "pyramid_lab.json")
	mustWrite(jsonPath, js)

	// SVG 曲线（artifacts 与包文档目录各一份）
	svg := lab.RenderSVG(rep)
	svgArtifact := filepath.Join(*outDir, "pyramid_decay.svg")
	mustWrite(svgArtifact, []byte(svg))

	// artifacts 内报告，优先引用 PNG 预览，否则引用同目录 SVG。
	artifactChart := "pyramid_decay.svg"
	if _, err := os.Stat(filepath.Join(*outDir, "pyramid_decay.png")); err == nil {
		artifactChart = "pyramid_decay.png"
	}
	reportArtifact := filepath.Join(*outDir, "REPORT.md")
	mustWrite(reportArtifact, []byte(lab.MarkdownReport(rep, artifactChart)))

	if *repoDoc != "" {
		if err := os.MkdirAll(*repoDoc, 0o755); err != nil {
			fmt.Fprintln(os.Stderr, "建文档目录失败:", err)
			os.Exit(2)
		}
		mustWrite(filepath.Join(*repoDoc, "decay.svg"), []byte(svg))
		repoChart := "decay.svg"
		if _, err := os.Stat(filepath.Join(*repoDoc, "decay.png")); err == nil {
			repoChart = "decay.png"
		}
		mustWrite(filepath.Join(*repoDoc, "REPORT.md"),
			[]byte(lab.MarkdownReport(rep, repoChart)))
	}

	fmt.Print(lab.FormatTables(rep))
	fmt.Printf("\nJSON    -> %s\nSVG     -> %s\nREPORT  -> %s\n", jsonPath, svgArtifact, reportArtifact)
}

func mustWrite(path string, data []byte) {
	if err := os.WriteFile(path, data, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "写 %s 失败: %v\n", path, err)
		os.Exit(2)
	}
}
