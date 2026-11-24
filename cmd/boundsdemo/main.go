// Command boundsdemo 生成三类贴边样例（负坐标 / 超画幅 / 恰好等于边界）在
// geom、raster、post 三个包里的实际输出，并写出处理规则表。
//
// 输出目录布局：
//
//	samples/<kind>/<pkg>.txt   每次调用的完整输出（错误文本或成功摘要）
//	samples/<kind>/<pkg>.pgm   成功时同时给出图像；拒绝时没有图像
//	samples/RULES.md           一行一条的处理规则表（本程序生成）
//
// kind 取 negative / beyond / edge。
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"vecgo/bounds"
	"vecgo/geom"
	"vecgo/post"
	"vecgo/raster"
)

const side = 16

type scenario struct {
	kind  string
	label string
	poly  []geom.Point // 同一组坐标喂给 geom 与 raster
	rect  bounds.Rect  // post 的像素区域取同一组坐标包围盒
}

func scenarios() []scenario {
	return []scenario{
		{
			kind:  "negative",
			label: "负坐标：顶点 (-4,4) 落在画幅左侧之外",
			poly: []geom.Point{
				{X: -4, Y: 4}, {X: 4, Y: 4}, {X: 4, Y: 12}, {X: -4, Y: 12},
			},
			rect: bounds.Rect{X0: -4, Y0: 4, X1: 4, Y1: 12},
		},
		{
			kind:  "beyond",
			label: "超画幅：顶点 (20,8) 超出 16×16 画幅",
			poly: []geom.Point{
				{X: 8, Y: 8}, {X: 20, Y: 8}, {X: 20, Y: 12}, {X: 8, Y: 12},
			},
			rect: bounds.Rect{X0: 8, Y0: 8, X1: 20, Y1: 12},
		},
		{
			kind:  "edge",
			label: "恰好等于边界：四角落在 (0,0)-(16,16) 的四条边上",
			poly: []geom.Point{
				{X: 0, Y: 0}, {X: 16, Y: 0}, {X: 16, Y: 16}, {X: 0, Y: 16},
			},
			rect: bounds.Rect{X0: 0, Y0: 0, X1: 16, Y1: 16},
		},
	}
}

func main() {
	out := "artifacts/samples"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		panic(err)
	}
	var table strings.Builder
	table.WriteString("| # | 情形 | 包.入口 | 输入 | 结果 | 输出 |\n")
	table.WriteString("|---|---|---|---|---|---|\n")
	row := 0
	fail := 0
	for _, sc := range scenarios() {
		dir := filepath.Join(out, sc.kind)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			panic(err)
		}
		frame := bounds.PixelFrame(side, side)

		// ---- geom：Within 要求所有顶点落在 frame 内 ----
		row++
		bpts := make([]bounds.Point, len(sc.poly))
		for i, p := range sc.poly {
			bpts[i] = bounds.Point{X: p.X, Y: p.Y}
		}
		geomOut, geomErr := geom.Within(sc.poly, frame)
		if geomErr != nil {
			writeText(dir, "geom.txt", fmt.Sprintf("geom.Within 输入 %d 个顶点\n结果：拒绝\n%s\n", len(sc.poly), geomErr.Error()))
			table.WriteString(fmt.Sprintf("| %d | %s | geom.Within | %s | 拒绝 | `%s/geom.txt` |\n",
				row, sc.label, pointList(sc.poly), sc.kind))
			fail++
		} else {
			img := rasterize(geomOut)
			path := filepath.Join(dir, "geom.pgm")
			if err := img.SavePGM(path); err != nil {
				panic(err)
			}
			writeText(dir, "geom.txt", fmt.Sprintf("geom.Within 输入 %d 个顶点\n结果：接受，%d 个顶点原样返回\n", len(sc.poly), len(geomOut)))
			table.WriteString(fmt.Sprintf("| %d | %s | geom.Within | %s | 接受（贴边算界内） | `%s/geom.txt`、`geom.pgm` |\n",
				row, sc.label, pointList(sc.poly), sc.kind))
		}
		_ = bpts

		// ---- raster：FillAt 要求顶点落在 frame 内 ----
		row++
		im, err := raster.New(side, side)
		if err != nil {
			panic(err)
		}
		rasErr := im.FillAt([][]geom.Point{sc.poly}, 2, frame)
		if rasErr != nil {
			writeText(dir, "raster.txt", fmt.Sprintf("raster.FillAt 画幅 %dx%d supersample=2\n结果：拒绝\n%s\n", side, side, rasErr.Error()))
			table.WriteString(fmt.Sprintf("| %d | %s | raster.FillAt | %s，画幅 %d×%d | 拒绝 | `%s/raster.txt` |\n",
				row, sc.label, pointList(sc.poly), side, side, sc.kind))
			fail++
		} else {
			path := filepath.Join(dir, "raster.pgm")
			if err := im.SavePGM(path); err != nil {
				panic(err)
			}
			writeText(dir, "raster.txt", fmt.Sprintf("raster.FillAt 画幅 %dx%d supersample=2\n结果：接受，点亮 %d 个像素\n", side, side, im.CountOn(1)))
			table.WriteString(fmt.Sprintf("| %d | %s | raster.FillAt | %s，画幅 %d×%d | 接受（贴边算界内） | `%s/raster.txt`、`raster.pgm` |\n",
				row, sc.label, pointList(sc.poly), side, side, sc.kind))
		}

		// ---- post：GammaRect 的像素区域判定 ----
		row++
		im2, err := raster.New(side, side)
		if err != nil {
			panic(err)
		}
		for i := range im2.Pix {
			im2.Pix[i] = 128
		}
		postErr := post.GammaRect(im2, 2.0, sc.rect)
		if postErr != nil {
			writeText(dir, "post.txt", fmt.Sprintf("post.GammaRect 画幅 %dx%d 区域 [%d,%d,%d,%d)\n结果：拒绝\n%s\n",
				side, side, sc.rect.X0, sc.rect.Y0, sc.rect.X1, sc.rect.Y1, postErr.Error()))
			table.WriteString(fmt.Sprintf("| %d | %s | post.GammaRect | 区域 [%d,%d,%d,%d)，画幅 %d×%d | 拒绝 | `%s/post.txt` |\n",
				row, sc.label, sc.rect.X0, sc.rect.Y0, sc.rect.X1, sc.rect.Y1, side, side, sc.kind))
			fail++
		} else {
			path := filepath.Join(dir, "post.pgm")
			if err := im2.SavePGM(path); err != nil {
				panic(err)
			}
			writeText(dir, "post.txt", fmt.Sprintf("post.GammaRect 画幅 %dx%d 区域 [%d,%d,%d,%d)\n结果：接受，区域贴满画幅四条边\n",
				side, side, sc.rect.X0, sc.rect.Y0, sc.rect.X1, sc.rect.Y1))
			table.WriteString(fmt.Sprintf("| %d | %s | post.GammaRect | 区域 [%d,%d,%d,%d)，画幅 %d×%d | 接受（贴边算界内） | `%s/post.txt`、`post.pgm` |\n",
				row, sc.label, sc.rect.X0, sc.rect.Y0, sc.rect.X1, sc.rect.Y1, side, side, sc.kind))
		}
	}

	header := "# 坐标越界处理规则表（由 cmd/boundsdemo 生成）\n\n" +
		"判定只有一个来源：`bounds` 包。坐标域四条边算界内（贴边接受）；负坐标、超画幅、\n" +
		"非有限数、区域倒置/退化一律拒绝，错误里带包名、入口名、点下标与坐标。\n" +
		"下表一行一条，对应 `samples/` 下三个目录的实际输出。\n\n" +
		"## 判定总表\n\n" +
		"| 输入情形 | bounds.Classify 结论 | 三包统一处理 |\n" +
		"|---|---|---|\n" +
		"| 坐标严格落在域内 | Inside | 接受 |\n" +
		"| 坐标恰好等于某条边 | OnEdge | 接受（贴边算界内） |\n" +
		"| 坐标小于下界（负方向） | Negative | 拒绝，报点下标与坐标 |\n" +
		"| 坐标大于上界（超画幅） | Beyond | 拒绝，报点下标与坐标 |\n" +
		"| 坐标是 NaN 或 ±Inf | NonFinite | 拒绝，报点下标与坐标 |\n" +
		"| 区域倒置或为空 | Inverted | 拒绝，报区域与原因 |\n" +
		"| 包围盒零宽或零高 | Degenerate | 拒绝，报包围盒宽高 |\n\n" +
		"## 改前 / 改后对照\n\n" +
		"| 情形 | 包 | 改前 | 改后 |\n" +
		"|---|---|---|---|\n" +
		"| 顶点越出画幅（负坐标或超画幅） | geom | 无画幅概念；`Clip` 对整体在外静默返回空切片 | `Within` 拒绝，报 `geom.Within: poly[i]` 与坐标 |\n" +
		"| 顶点越出画幅 | raster | 无画幅概念；整帧包围盒决定采样域 | `FillAt` 拒绝，报 `raster.FillAt: polys[i][j]` 与坐标 |\n" +
		"| 区域越出画幅 | post | 无区域入口，无从越界 | `GammaRect` 拒绝，报越界那个角与坐标 |\n" +
		"| 顶点恰好贴边 | geom/raster/post | 各包隐含语义不一 | 三包一致：贴边接受 |\n" +
		"| 坐标是 NaN/±Inf | geom | 静默传播，下游产出空图或异常 | 拒绝，报点下标与坐标 |\n" +
		"| 坐标是 NaN/±Inf | raster | 静默接受，产出全 0 像素 | 拒绝，报点下标与坐标 |\n" +
		"| 包围盒零宽/零高 | raster | 静默把跨度 0 改成 1，把线段拉伸成整幅图 | 拒绝，报包围盒宽高 |\n" +
		"| 模糊核邻域越界 | post | 循环内各自 `ny >= 0 && ny < H` 判定 | 统一走 `bounds.ContainsPixel`，语义不变 |\n\n" +
		"## 逐条样例\n\n"
	if err := os.WriteFile(filepath.Join(out, "RULES.md"), []byte(header+table.String()), 0o644); err != nil {
		panic(err)
	}
	fmt.Printf("样例写入 %s（%d 条，其中拒绝 %d 条）\n", out, row, fail)
}

// rasterize 把一个多边形画进 16×16 的画布，用于成功路径的可视化。
// 坐标域是整幅画幅；这里只处理贴边样例（顶点在 [0,16] 内）。
func rasterize(poly []geom.Point) *raster.Image {
	im, err := raster.New(side, side)
	if err != nil {
		panic(err)
	}
	if err := im.FillAt([][]geom.Point{poly}, 2, bounds.PixelFrame(side, side)); err != nil {
		panic(err)
	}
	return im
}

func writeText(dir, name, body string) {
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		panic(err)
	}
}

// pointList 用紧凑形式列出顶点，便于在规则表里逐条核对。
func pointList(pts []geom.Point) string {
	parts := make([]string, len(pts))
	for i, p := range pts {
		parts[i] = fmt.Sprintf("(%g,%g)", p.X, p.Y)
	}
	return strings.Join(parts, " ")
}
