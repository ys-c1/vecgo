package main

// 把三对网格（耳切 / 最优）导出为 SVG，并高亮各自最差（面积比最小）的三角形。
// 高亮色标出覆盖率误差的主要来源。

import (
	"fmt"
	"math"
	"os"
	"strings"

	"vecgo/geom"
)

const (
	svgSize  = 460.0
	svgPad   = 16.0
	svgLabel = 34.0
)

type view struct {
	poly  []geom.Point
	tris  []geom.Triangle
	worst geom.Triangle
	title string
}

func saveComparison(base string, poly []geom.Point, r row) error {
	earTris, err := geom.EarClip(poly)
	if err != nil {
		return err
	}
	optTris, err := geom.OptimalTriangulation(poly)
	if err != nil {
		return err
	}
	ear := view{
		poly:  poly,
		tris:  earTris,
		worst: r.earWorstTri,
		title: fmt.Sprintf("耳切 EarClip  最小角 %.2f°  面积比 %.3f  覆盖率误差 %.3f%%",
			r.earMinAngle, r.earQuality, r.earCoverage*100),
	}
	opt := view{
		poly:  poly,
		tris:  optTris,
		worst: r.optWorstTri,
		title: fmt.Sprintf("最优 Optimal  最小角 %.2f°  面积比 %.3f  覆盖率误差 %.3f%%",
			r.optMinAngle, r.optQuality, r.optCoverage*100),
	}
	if err := writeSVG(base+"-ear.svg", ear, "#b03a2e"); err != nil {
		return err
	}
	if err := writeSVG(base+"-opt.svg", opt, "#1e6b3c"); err != nil {
		return err
	}
	if err := saveWorstPNG(base+"-ear.png", poly, earTris, r.earWorstTri, colEar); err != nil {
		return err
	}
	if err := saveWorstPNG(base+"-opt.png", poly, optTris, r.optWorstTri, colOpt); err != nil {
		return err
	}
	// 最差三角形特写（统一比例尺），直接回应“改进前后最差三角形各存一张照”。
	if err := saveWorstCloseupPNG(base+"-ear-closeup.png", r.earWorstTri, colEar); err != nil {
		return err
	}
	return saveWorstCloseupPNG(base+"-opt-closeup.png", r.optWorstTri, colOpt)
}

func writeSVG(path string, v view, accent string) error {
	tf := newTransform(v.poly)

	var b strings.Builder
	b.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	fmt.Fprintf(&b, "<svg xmlns=\"http://www.w3.org/2000/svg\" width=\"%.0f\" height=\"%.0f\" viewBox=\"0 0 %.0f %.0f\">\n",
		svgSize, svgSize, svgSize, svgSize)
	fmt.Fprintf(&b, "<rect width=\"100%%\" height=\"100%%\" fill=\"#fbfaf7\"/>\n")
	fmt.Fprintf(&b, "<text x=\"%.0f\" y=\"%.0f\" font-family=\"sans-serif\" font-size=\"12\" fill=\"#333\">%s</text>\n",
		svgPad, svgLabel-12, xmlEscape(v.title))

	for _, t := range v.tris {
		a, bb, c := tf.apply(t[0]), tf.apply(t[1]), tf.apply(t[2])
		fmt.Fprintf(&b, "<path d=\"M%.2f %.2f L%.2f %.2f L%.2f %.2f Z\" fill=\"none\" stroke=\"#8a8f96\" stroke-width=\"0.6\"/>\n",
			a.X, a.Y, bb.X, bb.Y, c.X, c.Y)
	}

	// 最差三角形半透明填充 + 粗描边
	wa, wb, wc := tf.apply(v.worst[0]), tf.apply(v.worst[1]), tf.apply(v.worst[2])
	fmt.Fprintf(&b, "<path d=\"M%.2f %.2f L%.2f %.2f L%.2f %.2f Z\" fill=\"%s\" fill-opacity=\"0.28\" stroke=\"%s\" stroke-width=\"2.4\"/>\n",
		wa.X, wa.Y, wb.X, wb.Y, wc.X, wc.Y, accent, accent)

	// 多边形轮廓
	fmt.Fprintf(&b, "<path d=\"")
	for i, q := range v.poly {
		p := tf.apply(q)
		if i == 0 {
			fmt.Fprintf(&b, "M%.2f %.2f ", p.X, p.Y)
		} else {
			fmt.Fprintf(&b, "L%.2f %.2f ", p.X, p.Y)
		}
	}
	fmt.Fprintf(&b, "Z\" fill=\"none\" stroke=\"#2c3e50\" stroke-width=\"1.5\"/>\n")
	b.WriteString("</svg>\n")

	return os.WriteFile(path, []byte(b.String()), 0o644)
}

type transform struct {
	scale            float64
	minx, miny, maxy float64
	dx, dy           float64
}

func newTransform(poly []geom.Point) transform {
	minx, miny := math.Inf(1), math.Inf(1)
	maxx, maxy := math.Inf(-1), math.Inf(-1)
	for _, p := range poly {
		minx = math.Min(minx, p.X)
		miny = math.Min(miny, p.Y)
		maxx = math.Max(maxx, p.X)
		maxy = math.Max(maxy, p.Y)
	}
	spanX := math.Max(maxx-minx, 1e-12)
	spanY := math.Max(maxy-miny, 1e-12)
	availW := svgSize - 2*svgPad
	availH := svgSize - svgLabel - svgPad
	s := math.Min(availW/spanX, availH/spanY)
	// 在可用区域内居中
	dx := svgPad + (availW-spanX*s)/2
	dy := svgLabel + (availH-spanY*s)/2
	return transform{scale: s, minx: minx, miny: miny, maxy: maxy, dx: dx, dy: dy}
}

// apply 等比缩放并把 y 轴翻转为屏幕坐标（SVG 的 y 向下）。
func (t transform) apply(p geom.Point) geom.Point {
	return geom.Point{
		X: t.dx + (p.X-t.minx)*t.scale,
		Y: t.dy + (t.maxy-p.Y)*t.scale,
	}
}

func xmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return r.Replace(s)
}
