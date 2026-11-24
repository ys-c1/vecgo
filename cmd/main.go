package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"

	"vecgo/config"
	"vecgo/geom"
	"vecgo/pipe"
	"vecgo/raster"
)

// scenePoint 是场景 JSON 中单个顶点的解码类型。除普通数字外，还接受
// "Inf"、"-Inf"、"NaN" 三个字符串 token，用于在场景文件中显式构造坏坐标。
type scenePoint struct {
	X, Y float64
}

func (p *scenePoint) UnmarshalJSON(data []byte) error {
	var raw struct {
		X json.RawMessage `json:"x"`
		Y json.RawMessage `json:"y"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	x, err := parseCoord(raw.X)
	if err != nil {
		return fmt.Errorf("x 坐标: %w", err)
	}
	y, err := parseCoord(raw.Y)
	if err != nil {
		return fmt.Errorf("y 坐标: %w", err)
	}
	p.X, p.Y = x, y
	return nil
}

func parseCoord(b []byte) (float64, error) {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		switch s {
		case "Inf":
			return math.Inf(1), nil
		case "-Inf":
			return math.Inf(-1), nil
		case "NaN":
			return math.NaN(), nil
		default:
			f, err := strconv.ParseFloat(s, 64)
			if err != nil {
				return 0, err
			}
			return f, nil
		}
	}
	var f float64
	if err := json.Unmarshal(b, &f); err != nil {
		return 0, err
	}
	return f, nil
}

func loadScene(path string) ([][]geom.Point, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var raw [][]scenePoint
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&raw); err != nil {
		return nil, err
	}
	scene := make([][]geom.Point, len(raw))
	for i, poly := range raw {
		scene[i] = make([]geom.Point, len(poly))
		for j, q := range poly {
			scene[i][j] = geom.Point{X: q.X, Y: q.Y}
		}
	}
	return scene, nil
}

// writeSkipped 将被跳过图形的编号按升序、一行一个写出：指定路径则写文件，
// 否则写到标准输出。文件内容为纯编号，脚本可直接逐行读取。
func writeSkipped(skipped raster.SkippedList, path string) error {
	var w io.Writer = os.Stdout
	if path != "" {
		f, err := os.Create(path)
		if err != nil {
			return err
		}
		defer f.Close()
		w = f
	}
	for _, s := range skipped {
		if _, err := fmt.Fprintln(w, s.Index); err != nil {
			return err
		}
	}
	return nil
}

func fail(code int, format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(code)
}

func main() {
	isolate := flag.Bool("isolate", false, "开启失败隔离：坏图形只跳过自身，其余照常绘制")
	scenePath := flag.String("scene", "", "场景 JSON 文件路径；缺省使用内置演示场景")
	out := flag.String("out", "out.pgm", "输出 PGM 路径")
	skipList := flag.String("skip-list", "", "跳过编号清单输出路径（纯编号，升序，一行一个）；缺省打印到标准输出")
	flag.Parse()

	cfg := config.Default()
	cfg.Output = *out

	var scene [][]geom.Point
	if *scenePath != "" {
		var err error
		scene, err = loadScene(*scenePath)
		if err != nil {
			fail(1, "场景读取失败: %v", err)
		}
	} else {
		scene = [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}}
	}

	if *isolate {
		im, skipped, err := pipe.RunIsolated(scene, cfg)
		if err != nil {
			fail(1, "失败: %v", err)
		}
		if err := im.SavePGM(cfg.Output); err != nil {
			fail(2, "导出失败: %v", err)
		}
		for _, s := range skipped {
			fmt.Fprintf(os.Stderr, "跳过图形 #%d: %s\n", s.Index, s.Reason)
		}
		if err := writeSkipped(skipped, *skipList); err != nil {
			fail(2, "跳过清单写出失败: %v", err)
		}
		fmt.Fprintf(os.Stderr, "isolate=on size=%dx%d skipped=%d out=%s\n",
			im.W, im.H, len(skipped), cfg.Output)
		return
	}

	im, err := pipe.Run(scene, cfg)
	if err != nil {
		fail(1, "失败: %v", err)
	}
	if err := im.SavePGM(cfg.Output); err != nil {
		fail(2, "导出失败: %v", err)
	}
	fmt.Fprintf(os.Stderr, "isolate=off on=%d size=%dx%d out=%s\n",
		im.CountOn(128), im.W, im.H, cfg.Output)
}
