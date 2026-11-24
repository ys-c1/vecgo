package lab

import (
	"fmt"
	"math"
	"strings"
)

// 图表配色取自 dataviz 参考调色板（浅面），蓝/橙对已通过 CVD 校验。
const (
	colSurface = "#fcfcfb"
	colInk     = "#0b0b0b"
	colSecond  = "#52514e"
	colMuted   = "#898781"
	colGrid    = "#e1e0d9"
	colAxis    = "#c3c2b7"
	colBlue    = "#2a78d6"
	colOrange  = "#eb6834"
	colGrayDot = "#a8a69f"
)

type panel struct {
	x0, y0, x1, y1 float64
	log            bool
	lo, hi         float64
	nmax           int
}

func (p panel) w() float64 { return p.x1 - p.x0 }
func (p panel) h() float64 { return p.y1 - p.y0 }

func (p panel) x(n int) float64 {
	return p.x0 + float64(n-1)/float64(p.nmax-1)*p.w()
}

// xF 允许连续 n（曲线采样）。
func (p panel) xF(n float64) float64 {
	return p.x0 + (n-1)/float64(p.nmax-1)*p.w()
}

func (p panel) y(v float64) float64 {
	if p.log {
		a, b := math.Log10(p.lo), math.Log10(p.hi)
		t := (math.Log10(v) - a) / (b - a)
		return p.y1 - t*p.h()
	}
	return p.y1 - v/p.hi*p.h()
}

// RenderSVG 生成“误差—层数”双面板（线性 + 对数）曲线图。
func RenderSVG(r *Report) string {
	var b strings.Builder
	W, H := 920.0, 728.0
	b.WriteString(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%.0f" height="%.0f" viewBox="0 0 %.0f %.0f" font-family="system-ui,-apple-system,'Segoe UI',sans-serif">`, W, H, W, H))
	b.WriteString(fmt.Sprintf(`<rect x="0" y="0" width="%.0f" height="%.0f" fill="%s"/>`, W, H, colSurface))

	// 标题
	b.WriteString(fmt.Sprintf(`<text x="40" y="32" font-size="18" font-weight="700" fill="%s">金字塔层数 → 重建误差衰减曲线</text>`, colInk))
	b.WriteString(fmt.Sprintf(`<text x="40" y="53" font-size="12.5" fill="%s">256×256 gradient 场景 · 2×2 均值金字塔 · RMSE 单位为 0–255 灰度级 · q=%.3f, A=%.1f</text>`, colSecond, r.Model.Q, r.Model.A))

	// 图例
	legendY := 76.0
	b.WriteString(legendLine(40, legendY, colBlue))
	b.WriteString(fmt.Sprintf(`<text x="62" y="%0.1f" font-size="12" fill="%s">解出曲线 E(n)=A·q^n</text>`, legendY+4, colSecond))
	lx := 247.0
	b.WriteString(fmt.Sprintf(`<circle cx="%0.1f" cy="%0.1f" r="4" fill="%s"/>`, lx, legendY, colGrayDot))
	b.WriteString(fmt.Sprintf(`<text x="%0.1f" y="%0.1f" font-size="12" fill="%s">穷举实测（全部 n）</text>`, lx+10, legendY+4, colSecond))
	lx = 412
	b.WriteString(fmt.Sprintf(`<circle cx="%0.1f" cy="%0.1f" r="6" fill="%s" stroke="%s" stroke-width="2"/>`, lx, legendY, colOrange, colSurface))
	b.WriteString(fmt.Sprintf(`<text x="%0.1f" y="%0.1f" font-size="12" fill="%s">三档目标解出层上的实测点</text>`, lx+12, legendY+4, colSecond))

	lin := panel{x0: 76, y0: 120, x1: 880, y1: 336, log: false, lo: 0, hi: 80, nmax: r.Depth}
	logp := panel{x0: 76, y0: 410, x1: 880, y1: 646, log: true, lo: 0.4, hi: 100, nmax: r.Depth}

	b.WriteString(renderPanel(r, lin, "线性纵轴：误差随层数的衰减形态", []float64{0, 20, 40, 60, 80}, false))
	b.WriteString(renderPanel(r, logp, "对数纵轴：指数衰减近似一条直线", []float64{0.5, 1, 2, 5, 10, 20, 50, 100}, true))

	// x 轴总标题
	b.WriteString(fmt.Sprintf(`<text x="%.0f" y="696" text-anchor="middle" font-size="13" fill="%s">金字塔层数 n（1=仅最粗 1×1 层；%d=精确还原）</text>`, (logp.x0+logp.x1)/2, colSecond, r.Depth))
	b.WriteString(`</svg>`)
	return b.String()
}

func legendLine(x, y float64, color string) string {
	return fmt.Sprintf(`<line x1="%.0f" y1="%.1f" x2="%0.0f" y2="%.1f" stroke="%s" stroke-width="2.5" stroke-linecap="round"/>`, x, y, x+18, y, color)
}

func renderPanel(r *Report, p panel, title string, yticks []float64, isLog bool) string {
	var b strings.Builder
	// 面板标题
	b.WriteString(fmt.Sprintf(`<text x="%.0f" y="%.0f" font-size="13" font-weight="600" fill="%s">%s</text>`, p.x0, p.y0-14, colInk, title))

	// 网格 + y 刻度
	for _, tv := range yticks {
		yy := p.y(tv)
		b.WriteString(fmt.Sprintf(`<line x1="%.0f" y1="%.2f" x2="%.0f" y2="%.2f" stroke="%s" stroke-width="1"/>`, p.x0, yy, p.x1, yy, colGrid))
		lab := formatTick(tv)
		b.WriteString(fmt.Sprintf(`<text x="%.0f" y="%.2f" text-anchor="end" font-size="11" fill="%s" font-variant-numeric="tabular-nums">%s</text>`, p.x0-8, yy+4, colMuted, lab))
	}
	// 坐标轴
	b.WriteString(fmt.Sprintf(`<line x1="%.0f" y1="%.0f" x2="%.0f" y2="%.0f" stroke="%s" stroke-width="1.2"/>`, p.x0, p.y1, p.x1, p.y1, colAxis))
	b.WriteString(fmt.Sprintf(`<line x1="%.0f" y1="%.0f" x2="%.0f" y2="%.0f" stroke="%s" stroke-width="1.2"/>`, p.x0, p.y0, p.x0, p.y1, colAxis))

	// x 刻度 1..Depth
	for n := 1; n <= r.Depth; n++ {
		xx := p.x(n)
		b.WriteString(fmt.Sprintf(`<line x1="%.1f" y1="%.0f" x2="%.1f" y2="%.0f" stroke="%s" stroke-width="1"/>`, xx, p.y1, xx, p.y1+5, colAxis))
		b.WriteString(fmt.Sprintf(`<text x="%.1f" y="%.0f" text-anchor="middle" font-size="11" fill="%s" font-variant-numeric="tabular-nums">%d</text>`, xx, p.y1+19, colMuted, n))
	}

	// 目标 ε 虚线（先画，压在数据下）；标签基线做碰撞错开。
	type epsTag struct {
		eps   float64
		lineY float64
		baseY float64
	}
	tags := make([]epsTag, 0, len(r.Targets))
	for _, t := range r.Targets {
		if isLog && t.Epsilon < p.lo {
			continue
		}
		yy := p.y(t.Epsilon)
		b.WriteString(fmt.Sprintf(`<line x1="%.0f" y1="%.2f" x2="%.0f" y2="%.2f" stroke="%s" stroke-width="1.2" stroke-dasharray="6 4" opacity="0.55"/>`, p.x0, yy, p.x1, yy, colOrange))
		tags = append(tags, epsTag{eps: t.Epsilon, lineY: yy, baseY: yy - 6})
	}
	// 按基线排序后保证相邻标签至少 15px，避免线性轴底部重叠。
	for i := 1; i < len(tags); i++ {
		if tags[i].baseY < tags[i-1].baseY+15 {
			tags[i].baseY = tags[i-1].baseY + 15
		}
	}
	for _, tg := range tags {
		b.WriteString(fmt.Sprintf(`<text x="%.0f" y="%.2f" text-anchor="end" font-size="11" font-weight="600" fill="%s" stroke="%s" stroke-width="3" paint-order="stroke">目标 ε=%s</text>`, p.x1-6, tg.baseY, colSecond, colSurface, formatTick(tg.eps)))
	}

	// 解出曲线 A q^n（连续采样）
	var path strings.Builder
	started := false
	for nf := 1.0; nf <= float64(r.Depth)+0.001; nf += 0.03 {
		v := r.Model.Predict(nf)
		if isLog && v < p.lo {
			break
		}
		xx, yy := p.xF(nf), p.y(v)
		if !started {
			fmt.Fprintf(&path, "M%.2f %.2f", xx, yy)
			started = true
		} else {
			fmt.Fprintf(&path, " L%.2f %.2f", xx, yy)
		}
	}
	b.WriteString(fmt.Sprintf(`<path d="%s" fill="none" stroke="%s" stroke-width="2.5" stroke-linejoin="round" stroke-linecap="round"/>`, path.String(), colBlue))

	// 穷举实测点
	targetN := map[int]bool{}
	for _, t := range r.Targets {
		targetN[t.AnalyticN] = true
	}
	for _, s := range r.Sweep {
		if targetN[s.N] {
			continue // 目标点最后高亮
		}
		if isLog && s.Measured < p.lo {
			continue
		}
		yy := p.y(s.Measured)
		b.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.2f" r="3.6" fill="%s" stroke="%s" stroke-width="1.5"><title>n=%d 实测 RMSE=%.3f</title></circle>`, p.x(s.N), yy, colGrayDot, colSurface, s.N, s.Measured))
	}

	// 三档目标高亮点 + 直接标签
	for _, t := range r.Targets {
		xx := p.x(t.AnalyticN)
		yy := p.y(t.MeasuredRMSE)
		b.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.2f" r="7" fill="%s" stroke="%s" stroke-width="2.5"><title>%s: n*=%d（穷举 %d），实测 %.3f，预测 %.3f</title></circle>`,
			xx, yy, colOrange, colSurface, t.Name, t.AnalyticN, t.ExhaustiveN, t.MeasuredRMSE, t.PredictedRMSE))
		lab := fmt.Sprintf("n=%d · %.2f", t.AnalyticN, t.MeasuredRMSE)
		// 标签居中置于点上方，配表面色描边底，避免压住相邻层灰点。
		b.WriteString(fmt.Sprintf(`<text x="%.1f" y="%.1f" text-anchor="middle" font-size="12.5" font-weight="700" fill="%s" stroke="%s" stroke-width="4" paint-order="stroke">%s</text>`,
			xx, yy-15, colInk, colSurface, lab))
	}
	return b.String()
}

func formatTick(v float64) string {
	if v == math.Trunc(v) {
		return fmt.Sprintf("%d", int(v))
	}
	return fmt.Sprintf("%g", v)
}
