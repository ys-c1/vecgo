package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"vecgo/config"
	"vecgo/geom"
	"vecgo/pipe"
	"vecgo/raster"
	"vecgo/scene"
)

// builtin 是历史默认场景。无 -scene 时必须与改动前逐字节一致。
func builtin() [][]geom.Point {
	return [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}}
}

func main() {
	scenePath := flag.String("scene", "", "scene file to parse (default: built-in scene)")
	outPath := flag.String("out", "", "output PGM path (overrides scene's output field)")
	partialSel := flag.Bool("partial", false, "render recovered partial result even if parsing failed")
	asJSON := flag.Bool("json", false, "print parse/render report as JSON")
	flag.Parse()

	if *scenePath == "" {
		runBuiltin(flag.Args())
		return
	}

	res := scene.Parse(*scenePath)
	if res.Err != nil && !*asJSON {
		fmt.Fprintf(os.Stderr, "解析失败: %s (%.3f ms)\n", *scenePath, res.ElapsedMS)
		for _, d := range res.Diagnostics {
			fmt.Fprintf(os.Stderr, "  %s\n", d.Error())
			if d.Snippet != "" {
				fmt.Fprintf(os.Stderr, "    原文: %q\n", d.Snippet)
			}
		}
		fmt.Fprintf(os.Stderr, "可回收: %d 个图形, config=%+v\n", len(res.Scroll), res.Config)
	}
	if res.Err != nil && !*partialSel {
		if *asJSON {
			emitJSON(res, nil, res.Err)
		}
		os.Exit(3)
	}

	polys, cfg, err := res.Map()
	if err != nil {
		fmt.Fprintln(os.Stderr, "失败:", err)
		os.Exit(4)
	}
	if *outPath != "" {
		cfg.Output = *outPath
	}
	im, err := pipe.Run(polys, cfg)
	if err != nil {
		if *asJSON {
			emitJSON(res, nil, err)
		}
		fmt.Fprintln(os.Stderr, "渲染失败:", err)
		os.Exit(1)
	}
	if err := im.SavePGM(cfg.Output); err != nil {
		if *asJSON {
			emitJSON(res, im, err)
		}
		fmt.Fprintln(os.Stderr, "导出失败:", err)
		os.Exit(2)
	}
	if *asJSON {
		emitJSON(res, im, res.Err)
	} else {
		fmt.Printf("scene=%s parse_ms=%.3f polys=%d err=%v on=%d size=%dx%d out=%s\n",
			*scenePath, res.ElapsedMS, len(polys), res.Err,
			im.CountOn(128), im.W, im.H, cfg.Output)
	}
	if res.Err != nil {
		os.Exit(3)
	}
}

// runBuiltin 保持原有命令行语义: 位置参数即输出文件。
func runBuiltin(args []string) {
	cfg := config.Default()
	im, err := pipe.Run(builtin(), cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "失败:", err)
		os.Exit(1)
	}
	if len(args) > 0 {
		cfg.Output = args[0]
	}
	if err := im.SavePGM(cfg.Output); err != nil {
		fmt.Fprintln(os.Stderr, "导出失败:", err)
		os.Exit(2)
	}
	fmt.Printf("on=%d size=%dx%d\n", im.CountOn(128), im.W, im.H)
}

type jsonDiag struct {
	Code    string `json:"code"`
	Line    int    `json:"line"`
	EndLine int    `json:"end_line"`
	Field   string `json:"field"`
	Message string `json:"message"`
	Snippet string `json:"snippet"`
}

type jsonReport struct {
	Scene       string     `json:"scene"`
	ParseMS     float64    `json:"parse_ms"`
	OK          bool       `json:"ok"`
	Error       string     `json:"error,omitempty"`
	Polys       int        `json:"polys"`
	Groups      int        `json:"groups"`
	Width       int        `json:"width"`
	Height      int        `json:"height"`
	On          int        `json:"on"`
	Output      string     `json:"output,omitempty"`
	Diagnostics []jsonDiag `json:"diagnostics,omitempty"`
}

func emitJSON(res *scene.ParseResult, im *raster.Image, runErr error) {
	r := jsonReport{
		Scene:   res.Path,
		ParseMS: res.ElapsedMS,
		OK:      res.Err == nil && runErr == nil,
		Polys:   len(res.Scroll),
		Groups:  countGroups(res.Groups),
		Width:   res.Config.Width,
		Height:  res.Config.Height,
	}
	if res.Err != nil {
		r.Error = res.Err.Error()
	} else if runErr != nil {
		r.Error = runErr.Error()
	}
	if im != nil {
		r.On = im.CountOn(128)
		r.Width, r.Height = im.W, im.H
		r.Output = res.Config.Output
	}
	for _, d := range res.Diagnostics {
		r.Diagnostics = append(r.Diagnostics, jsonDiag{
			Code: string(d.Code), Line: d.Line, EndLine: d.EndLine,
			Field: d.Field, Message: d.Message, Snippet: d.Snippet,
		})
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(r); err != nil {
		fmt.Fprintln(os.Stderr, "JSON 输出失败:", err)
	}
}

func countGroups(gs []*scene.Group) int {
	n := 0
	for _, g := range gs {
		n += 1 + countGroups(g.Children)
	}
	return n
}
