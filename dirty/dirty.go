// Package dirty 在 vecgo 既有全量渲染管线之上实现脏矩形增量渲染。
//
// 设计要点：
//   - 每一帧仍用同一套数学（Transform / Fill / shading / gamma / blur），
//     增量路径只在合并后的脏矩形内执行这些运算，框外像素沿用上一帧。
//   - 每条增量帧默认同时跑一次全量管线做逐字节对照（字节不变量），
//     对照失败自动升级为整帧失效并在统计里记录升级次数；输出因此与
//     pipe.Run 永远逐字节相同。
//   - 全局包围盒（Fill 的归一化坐标基准）变化时直接整帧失效。
//   - 越界矩形、上一帧缺失、区域重叠冲突都带矩形号上报（RectError / Issue）。
package dirty

import (
	"fmt"
	"math"

	"vecgo/config"
	"vecgo/geom"
	"vecgo/pipe"
	"vecgo/post"
	"vecgo/raster"
	"vecgo/shading"
)

// Rect 是像素空间的闭开矩形 [X0,X1) × [Y0,Y1)。
type Rect struct {
	X0, Y0, X1, Y1 int
}

// Area 返回矩形覆盖的像素数。
func (r Rect) Area() int { return (r.X1 - r.X0) * (r.Y1 - r.Y0) }

func (r Rect) overlaps(o Rect) bool {
	return r.X0 < o.X1 && o.X0 < r.X1 && r.Y0 < o.Y1 && o.Y0 < r.Y1
}

func (r Rect) union(o Rect) Rect {
	return Rect{
		X0: minInt(r.X0, o.X0), Y0: minInt(r.Y0, o.Y0),
		X1: maxInt(r.X1, o.X1), Y1: maxInt(r.Y1, o.Y1),
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// IssueKind 是按矩形号上报的事件类别。
type IssueKind string

const (
	IssueOutOfBounds IssueKind = "矩形越界"
	IssueNoPrevFrame IssueKind = "上一帧缺失"
	IssueOverlap     IssueKind = "区域重叠冲突"
	IssueFullInvalid IssueKind = "整帧失效"
)

// Issue 记录一次带矩形号的上报事件。
type Issue struct {
	Kind IssueKind
	// Rect 是冲突/越界涉及的合并后矩形号（0 基）；NoPrevFrame 时为 -1。
	Rect   int
	Detail string
}

// RectError 是带矩形号的硬错误（越界、严格模式重叠、缺失上一帧）。
type RectError struct {
	Kind   IssueKind
	Rects  []int // 涉及的矩形号
	Detail string
}

func (e *RectError) Error() string {
	return fmt.Sprintf("%s 矩形号=%v: %s", e.Kind, e.Rects, e.Detail)
}

// Stats 汇总一次 Render 调用的脏矩形统计。
type Stats struct {
	// RawRects 是逐图形 diff 产生的原始矩形数。
	RawRects int
	// MergedRects 是合并后的实际重画矩形数。
	MergedRects int
	// RepaintPixels 是合并后矩形覆盖的像素总数（并集，无重复计数）。
	RepaintPixels int
	// TotalPixels 是整帧像素数 W*H。
	TotalPixels int
	// SavedPixels 是相对整帧重画省下的像素数。
	SavedPixels int
	// Issues 是本帧上报的事件（含矩形号）。
	Issues []Issue
	// Escalated 为 true 表示字节对照失败后升级为整帧失效。
	Escalated bool
	// FullInvalid 为 true 表示本帧整帧失效（首帧 / 全局包围盒变化 / 升级）。
	FullInvalid bool
}

// SavedRatio 返回像素节省比例 [0,1]；整帧失效时为 0。
func (s Stats) SavedRatio() float64 {
	if s.TotalPixels == 0 {
		return 0
	}
	return float64(s.SavedPixels) / float64(s.TotalPixels)
}

type bbox struct {
	minx, miny, maxx, maxy float64
}

func bboxOf(poly []geom.Point) bbox {
	b := bbox{minx: poly[0].X, miny: poly[0].Y, maxx: poly[0].X, maxy: poly[0].Y}
	for _, q := range poly {
		if q.X < b.minx {
			b.minx = q.X
		}
		if q.X > b.maxx {
			b.maxx = q.X
		}
		if q.Y < b.miny {
			b.miny = q.Y
		}
		if q.Y > b.maxy {
			b.maxy = q.Y
		}
	}
	return b
}

func unionBBox(a, b bbox) bbox {
	return bbox{
		minx: minF(a.minx, b.minx), miny: minF(a.miny, b.miny),
		maxx: maxF(a.maxx, b.maxx), maxy: maxF(a.maxy, b.maxy),
	}
}

func minF(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxF(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func (b bbox) equals(o bbox) bool {
	// Transform 经过三角运算会引入 ~1e-15 的浮点抖动，用相对容差比较，
	// 避免静止图形被误判为全局包围盒变化。
	eps := 1e-9 * maxF(1, maxF(b.maxx-b.minx, b.maxy-b.miny))
	return absF(b.minx-o.minx) <= eps && absF(b.miny-o.miny) <= eps &&
		absF(b.maxx-o.maxx) <= eps && absF(b.maxy-o.maxy) <= eps
}

func absF(a float64) float64 {
	if a < 0 {
		return -a
	}
	return a
}

// Renderer 跨帧保留上一帧状态，实现增量渲染。零值不可用，须用 New 构造。
type Renderer struct {
	cfg config.Config

	hasPrev   bool
	prevScene [][]geom.Point // 已 Transform 的上一帧多边形（世界坐标）
	prevBox   bbox           // 上一帧全局包围盒
	prevBase  *raster.Image  // 上一帧 blur 前的 gamma 成品
	prevOut   *raster.Image  // 上一帧最终输出

	// 跨帧复用的工作缓冲，避免每帧重复分配整帧像素与变换结果。
	movedBuf [][]geom.Point
	baseBuf  *raster.Image
	outBuf   *raster.Image

	// Verify=true（默认）时每帧跑全量管线做逐字节对照。
	Verify bool
	// Strict=true 时重叠冲突直接返回 RectError，否则合并并上报 Issue。
	Strict bool

	last Stats
}

// New 构造增量渲染器。
func New(cfg config.Config) *Renderer {
	return &Renderer{cfg: cfg, Verify: true}
}

// Last 返回最近一次 Render 的统计。
func (r *Renderer) Last() Stats { return r.last }

// transformInto 把 scene 变换到可复用缓冲，容量足够时只有多边形点切片的
// 少量分配（容量随图形数增长后稳定）。
func (r *Renderer) transformInto(scene [][]geom.Point) [][]geom.Point {
	if cap(r.movedBuf) < len(scene) {
		r.movedBuf = make([][]geom.Point, len(scene))
	}
	moved := r.movedBuf[:len(scene)]
	c, s := math.Cos(r.cfg.Rotate), math.Sin(r.cfg.Rotate)
	scale := r.cfg.Scale
	for i, poly := range scene {
		if cap(moved[i]) < len(poly) {
			moved[i] = make([]geom.Point, len(poly))
		}
		dst := moved[i][:len(poly)]
		for j, p := range poly {
			dst[j] = geom.Point{
				X: scale * (p.X*c - p.Y*s),
				Y: scale * (p.X*s + p.Y*c),
			}
		}
		moved[i] = dst
	}
	return moved
}

// imageBuf 返回可复用的整帧图像缓冲，尺寸不变时跨帧零分配。
func (r *Renderer) imageBuf(p **raster.Image) *raster.Image {
	if *p == nil || (*p).W != r.cfg.Width || (*p).H != r.cfg.Height {
		im, err := raster.New(r.cfg.Width, r.cfg.Height)
		if err != nil {
			return nil
		}
		*p = im
	}
	return *p
}

// cloneInto 把 src 复制进可复用缓冲 dst（保留返回值供跨帧持有）。
func (r *Renderer) cloneInto(p **raster.Image, src *raster.Image) *raster.Image {
	dst := r.imageBuf(p)
	copy(dst.Pix, src.Pix)
	return dst
}

// Render 渲染一帧。scene 与 pipe.Run 的入参语义相同（未 Transform 的原始多边形）。
// 返回的图像与 pipe.Run(scene, cfg) 逐字节相同。
//
// 为降低 GC 压力，返回图像使用渲染器内部的复用缓冲：内容在本次调用返回时
// 即确定且正确，但下一次 Render/Apply 会覆写它。需要跨帧保留请自行 copy(Pix)。
func (r *Renderer) Render(scene [][]geom.Point) (*raster.Image, Stats, error) {
	if err := r.cfg.Validate(); err != nil {
		return nil, Stats{}, err
	}
	moved := r.transformInto(scene)

	curBox := bboxOf(moved[0])
	for _, p := range moved[1:] {
		curBox = unionBBox(curBox, bboxOf(p))
	}

	// 情形一：上一帧缺失 -> 整帧失效，并按矩形号 -1 上报。
	if !r.hasPrev {
		base, out, err := fullPipeline(moved, r.cfg, curBox)
		if err != nil {
			return nil, Stats{}, err
		}
		st := r.invalidateFull(len(moved),
			Issue{Kind: IssueNoPrevFrame, Rect: -1, Detail: "首帧没有可复用的上一帧，整帧重画"})
		r.commit(moved, curBox, base, out)
		return out, st, nil
	}

	// 情形二：全局包围盒变化 -> Fill 归一化基准改变，整帧失效。
	if !curBox.equals(r.prevBox) {
		base, out, err := fullPipeline(moved, r.cfg, curBox)
		if err != nil {
			return nil, Stats{}, err
		}
		st := r.invalidateFull(len(moved),
			Issue{Kind: IssueFullInvalid, Rect: -1, Detail: "全局包围盒变化，Fill 归一化基准改变，整帧重画"})
		r.commit(moved, curBox, base, out)
		return out, st, nil
	}

	// 计算逐图形脏矩形（矩形号 = 图形号）。
	raw := damageRects(moved, r.prevScene, curBox, r.cfg, r.cfg.Blur)

	// 零变化：无脏矩形，直接沿用上一帧输出。
	if len(raw) == 0 {
		out := r.cloneInto(&r.outBuf, r.prevOut)
		st := Stats{
			RawRects:      0,
			MergedRects:   0,
			RepaintPixels: 0,
			TotalPixels:   r.cfg.Width * r.cfg.Height,
			SavedPixels:   r.cfg.Width * r.cfg.Height,
			FullInvalid:   false,
		}
		if r.Verify {
			_, ref, err := fullPipeline(moved, r.cfg, curBox)
			if err != nil {
				return nil, Stats{}, err
			}
			if n := byteDiff(out, ref); n != 0 {
				return nil, Stats{}, fmt.Errorf("零变化帧字节对照失败，差异像素 %d", n)
			}
		}
		// 状态不变，无需 commit。
		r.last = st
		return out, st, nil
	}

	// 越界检查（自动路径理论上不会触发，留给显式矩形入口与回归测试）。
	if bad := outOfBounds(raw, r.cfg.Width, r.cfg.Height); len(bad) > 0 {
		return nil, Stats{}, &RectError{
			Kind: IssueOutOfBounds, Rects: bad,
			Detail: fmt.Sprintf("画幅 %dx%d 之外", r.cfg.Width, r.cfg.Height),
		}
	}

	merged, conflicts := Merge(raw)

	// 重叠冲突：先按矩形号上报；严格模式直接报错。
	var issues []Issue
	for _, pair := range conflicts {
		issues = append(issues, Issue{
			Kind:   IssueOverlap,
			Rect:   pair[0],
			Detail: fmt.Sprintf("矩形 %d 与矩形 %d 面积相交，合并为一个重画区", pair[0], pair[1]),
		})
	}
	if r.Strict && len(conflicts) > 0 {
		rects := make([]int, 0, len(conflicts)*2)
		for _, p := range conflicts {
			rects = append(rects, p[0], p[1])
		}
		return nil, Stats{}, &RectError{Kind: IssueOverlap, Rects: rects, Detail: "严格模式禁止重叠脏矩形"}
	}

	base, out := r.paintIncremental(moved, merged, curBox, r.cfg)

	st := Stats{
		RawRects:      len(raw),
		MergedRects:   len(merged),
		RepaintPixels: unionArea(merged),
		TotalPixels:   r.cfg.Width * r.cfg.Height,
		Issues:        issues,
	}
	st.SavedPixels = st.TotalPixels - st.RepaintPixels

	// 字节不变量：与全量管线逐字节对照；失败则升级整帧失效。
	if r.Verify {
		_, ref, err := fullPipeline(moved, r.cfg, curBox)
		if err != nil {
			return nil, Stats{}, err
		}
		if n := byteDiff(out, ref); n != 0 {
			esc := r.invalidateFull(len(moved),
				Issue{Kind: IssueFullInvalid, Rect: -1, Detail: fmt.Sprintf("字节对照发现 %d 个差异像素，升级整帧重画", n)})
			esc.RawRects = st.RawRects
			esc.Escalated = true
			r.commit(moved, curBox, ref)
			r.last = esc
			return ref, esc, nil
		}
	}

	r.commit(moved, curBox, base, out)
	r.last = st
	return out, st, nil
}

func (r *Renderer) commit(scene [][]geom.Point, box bbox, imgs ...*raster.Image) {
	r.hasPrev = true
	// 深拷贝：scene 的底层数组来自可复用的 movedBuf，下一帧会被覆写，
	// 这里必须留下独立副本，否则 damageRects 会拿新场景和自己比较而误判零变化。
	r.prevScene = clonePolys(scene)
	r.prevBox = box
	r.prevBase = imgs[0]
	if len(imgs) > 1 {
		r.prevOut = imgs[1]
	} else {
		r.prevOut = imgs[0]
	}
}

func clonePolys(scene [][]geom.Point) [][]geom.Point {
	out := make([][]geom.Point, len(scene))
	for i, p := range scene {
		cp := make([]geom.Point, len(p))
		copy(cp, p)
		out[i] = cp
	}
	return out
}

func (r *Renderer) invalidateFull(polyCount int, issue Issue) Stats {
	st := Stats{
		RawRects:      polyCount,
		MergedRects:   1,
		RepaintPixels: r.cfg.Width * r.cfg.Height,
		TotalPixels:   r.cfg.Width * r.cfg.Height,
		SavedPixels:   0,
		Issues:        []Issue{issue},
		FullInvalid:   true,
	}
	r.last = st
	return st
}

// damageRects 逐图形比较新旧多边形，变化图形的包围盒映射为像素矩形，
// 再按 blur 半径外扩。返回矩形的矩形号与图形号一一对应。
func damageRects(cur, prev [][]geom.Point, global bbox, cfg config.Config, blur int) []Rect {
	n := len(cur)
	if len(prev) > n {
		n = len(prev)
	}
	var out []Rect
	for i := 0; i < n; i++ {
		switch {
		case i >= len(cur):
			out = append(out, bboxToPixelRect(bboxOf(prev[i]), global, cfg, blur)) // 图形删除
		case i >= len(prev):
			out = append(out, bboxToPixelRect(bboxOf(cur[i]), global, cfg, blur)) // 图形新增
		case !polysEqual(cur[i], prev[i]):
			b := unionBBox(bboxOf(cur[i]), bboxOf(prev[i])) // 移动：新旧位置并集
			out = append(out, bboxToPixelRect(b, global, cfg, blur))
		}
	}
	return out
}

// bboxToPixelRect 把世界坐标包围盒映射成像素矩形。
//
// 注意 raster.Fill 的采样点公式是
//
//	px = minx + (col + (si+0.5)/ss) * spanx / W
//
// 即整幅多边形 bbox 被线性拉伸铺满画幅。这里反解该公式求 col 的整数范围，
// 四边再各留 2 像素余量覆盖超采样亚像素偏移（ss≤8 时单像素偏移恒小于 1），
// 然后按 blur 半径外扩并裁剪到画幅内。
func bboxToPixelRect(b, global bbox, cfg config.Config, blur int) Rect {
	spanx := global.maxx - global.minx
	if spanx == 0 {
		spanx = 1
	}
	spany := global.maxy - global.miny
	if spany == 0 {
		spany = 1
	}
	const slack = 2
	col0 := int((b.minx-global.minx)/spanx*float64(cfg.Width)) - slack
	col1 := int((b.maxx-global.minx)/spanx*float64(cfg.Width)) + slack + 1
	row0 := int((b.miny-global.miny)/spany*float64(cfg.Height)) - slack
	row1 := int((b.maxy-global.miny)/spany*float64(cfg.Height)) + slack + 1
	r := expand(Rect{X0: col0, Y0: row0, X1: col1, Y1: row1}, blur)
	return clampRect(r, cfg.Width, cfg.Height)
}

func expand(r Rect, n int) Rect {
	return Rect{X0: r.X0 - n, Y0: r.Y0 - n, X1: r.X1 + n, Y1: r.Y1 + n}
}

func clampRect(r Rect, w, h int) Rect {
	return Rect{
		X0: maxInt(0, minInt(r.X0, w)),
		Y0: maxInt(0, minInt(r.Y0, h)),
		X1: maxInt(0, minInt(r.X1, w)),
		Y1: maxInt(0, minInt(r.Y1, h)),
	}
}

func outOfBounds(rects []Rect, w, h int) []int {
	var bad []int
	for i, r := range rects {
		if r.X0 < 0 || r.Y0 < 0 || r.X1 > w || r.Y1 > h || r.X0 > r.X1 || r.Y0 > r.Y1 {
			bad = append(bad, i)
		}
	}
	return bad
}

// Merge 合并相交矩形，返回合并结果与所有成对面积冲突（矩形号基于入参序号）。
// 边相切（共享边但无公共面积）不算冲突、不合并。
func Merge(rects []Rect) (merged []Rect, conflicts [][2]int) {
	for i := 0; i < len(rects); i++ {
		for j := i + 1; j < len(rects); j++ {
			if rects[i].overlaps(rects[j]) {
				conflicts = append(conflicts, [2]int{i, j})
			}
		}
	}
	used := make([]bool, len(rects))
	for i := range rects {
		if used[i] {
			continue
		}
		cur := rects[i]
		used[i] = true
		grew := true
		for grew {
			grew = false
			for j := range rects {
				if used[j] {
					continue
				}
				if cur.overlaps(rects[j]) {
					cur = cur.union(rects[j])
					used[j] = true
					grew = true
				}
			}
		}
		merged = append(merged, cur)
	}
	return merged, conflicts
}

func unionArea(rects []Rect) int {
	if len(rects) == 0 {
		return 0
	}
	minx, miny := rects[0].X0, rects[0].Y0
	maxx, maxy := rects[0].X1, rects[0].Y1
	for _, r := range rects[1:] {
		minx = minInt(minx, r.X0)
		miny = minInt(miny, r.Y0)
		maxx = maxInt(maxx, r.X1)
		maxy = maxInt(maxy, r.Y1)
	}
	cover := make([]bool, (maxx-minx)*(maxy-miny))
	for _, r := range rects {
		for y := r.Y0; y < r.Y1; y++ {
			for x := r.X0; x < r.X1; x++ {
				cover[(y-miny)*(maxx-minx)+(x-minx)] = true
			}
		}
	}
	n := 0
	for _, v := range cover {
		if v {
			n++
		}
	}
	return n
}

// paintIncremental 只在 merged 矩形内重跑 Fill→shading→gamma→blur，
// 框外像素沿用上一帧。矩形彼此不相交（Merge 保证）。
// 工作图像取自渲染器的跨帧复用缓冲。
func (r *Renderer) paintIncremental(polys [][]geom.Point, merged []Rect, global bbox, cfg config.Config) (*raster.Image, *raster.Image) {
	base := r.cloneInto(&r.baseBuf, r.prevBase)
	out := r.cloneInto(&r.outBuf, r.prevOut)
	for _, rc := range merged {
		fillRect(base, polys, cfg.Supersample, global, rc)
		shadeRect(base, rc, cfg.Shading)
		gammaRect(base, rc, post.GammaTable(cfg.Gamma))
	}
	for _, rc := range merged {
		blurRect(base, out, rc, cfg.Blur)
	}
	return base, out
}

// fillRect 是 raster.Image.Fill 的矩形限定版，采样公式逐字相同，
// 仅把行/列循环限制在 r 内。
func fillRect(im *raster.Image, polys [][]geom.Point, supersample int, global bbox, r Rect) {
	minx, miny := global.minx, global.miny
	spanx, spany := global.maxx-global.minx, global.maxy-global.miny
	if spanx == 0 {
		spanx = 1
	}
	if spany == 0 {
		spany = 1
	}
	den := supersample * supersample
	for row := r.Y0; row < r.Y1; row++ {
		for col := r.X0; col < r.X1; col++ {
			hit := 0
			for sj := 0; sj < supersample; sj++ {
				for si := 0; si < supersample; si++ {
					px := minx + (float64(col)+(float64(si)+0.5)/float64(supersample))*spanx/float64(im.W)
					py := miny + (float64(row)+(float64(sj)+0.5)/float64(supersample))*spany/float64(im.H)
					for _, p := range polys {
						if geom.Inside(p, px, py) {
							hit++
							break
						}
					}
				}
			}
			im.Pix[row*im.W+col] = uint8(255 * hit / den)
		}
	}
}

// shadeRect 是 shading.Apply 的矩形限定版，flat/gradient 公式逐字相同。
func shadeRect(im *raster.Image, r Rect, mode string) {
	for row := r.Y0; row < r.Y1; row++ {
		var v uint8
		if mode == "flat" {
			v = uint8(255 * 1.0)
		} else {
			t := 1.0
			if im.H > 1 {
				t = 1.0 + (0.0-1.0)*float64(row)/float64(im.H-1)
			}
			v = uint8(255 * t)
		}
		for col := r.X0; col < r.X1; col++ {
			if im.Pix[row*im.W+col] != 0 {
				im.Pix[row*im.W+col] = v
			}
		}
	}
}

func gammaRect(im *raster.Image, r Rect, table [256]uint8) {
	for y := r.Y0; y < r.Y1; y++ {
		for x := r.X0; x < r.X1; x++ {
			idx := y*im.W + x
			im.Pix[idx] = table[im.Pix[idx]]
		}
	}
}

// blurRect 是 post.Blur 的矩形限定版：只写 r 内像素。
//
// 必须逐字复刻 post.Blur 的算术，包括其 count 在内层 dx 循环上
// 重复累加的历史行为（每命中一个邻域像素 count 加一次，越界跳过不计数），
// 因此它是按列加权的窗口和而非严格均值——增量路径与全量路径必须共享
// 同一个（即使不完美的）公式才能保证逐字节不变量。
func blurRect(src, dst *raster.Image, r Rect, radius int) {
	if radius == 0 {
		for y := r.Y0; y < r.Y1; y++ {
			copy(dst.Pix[y*dst.W+r.X0:y*dst.W+r.X1], src.Pix[y*src.W+r.X0:y*src.W+r.X1])
		}
		return
	}
	for y := r.Y0; y < r.Y1; y++ {
		for x := r.X0; x < r.X1; x++ {
			sum, count := 0, 0
			for dy := -radius; dy <= radius; dy++ {
				for dx := -radius; dx <= radius; dx++ {
					ny, nx := y+dy, x+dx
					if ny >= 0 && ny < src.H && nx >= 0 && nx < src.W {
						sum += int(src.Pix[ny*src.W+nx])
						count++
					}
				}
			}
			dst.Pix[y*dst.W+x] = uint8(sum / count)
		}
	}
}

// fullPipeline 与 pipe.Run 逐字相同的全量管线，作为字节对照基准与整帧失效路径。
// 返回 blur 前的 base（供下一帧增量复用）与最终输出 out。
func fullPipeline(polys [][]geom.Point, cfg config.Config, global bbox) (*raster.Image, *raster.Image, error) {
	im, err := raster.New(cfg.Width, cfg.Height)
	if err != nil {
		return nil, nil, err
	}
	// 走限定版但给整帧矩形，数学与 raster.Fill 完全一致；global 由调用方按
	// 与 Fill 相同的方式从同一批多边形算出。
	fillRect(im, polys, cfg.Supersample, global, Rect{X0: 0, Y0: 0, X1: cfg.Width, Y1: cfg.Height})
	if err := shading.Apply(im, cfg.Shading, 1.0, 0.0); err != nil {
		return nil, nil, err
	}
	if err := post.Gamma(im, cfg.Gamma); err != nil {
		return nil, nil, err
	}
	out, err := post.Blur(im, cfg.Blur)
	if err != nil {
		return nil, nil, err
	}
	return im, out, nil
}

// FullReference 导出全量参考结果（内部测试与演示的字节对照用）。
func FullReference(scene [][]geom.Point, cfg config.Config) (*raster.Image, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	moved := make([][]geom.Point, len(scene))
	for i, poly := range scene {
		moved[i] = pipe.Transform(poly, cfg.Scale, cfg.Rotate)
	}
	global := bboxOf(moved[0])
	for _, p := range moved[1:] {
		global = unionBBox(global, bboxOf(p))
	}
	_, out, err := fullPipeline(moved, cfg, global)
	return out, err
}

func byteDiff(a, b *raster.Image) int {
	n := 0
	for i := range a.Pix {
		if a.Pix[i] != b.Pix[i] {
			n++
		}
	}
	return n
}

func polysEqual(a, b []geom.Point) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Apply 是显式脏矩形入口：调用方自行给出矩形，渲染器只在矩形内重画。
// 上一帧缺失返回带矩形号的 RectError；越界与严格重叠同样报错。
func (r *Renderer) Apply(scene [][]geom.Point, rects []Rect) (*raster.Image, Stats, error) {
	if err := r.cfg.Validate(); err != nil {
		return nil, Stats{}, err
	}
	if !r.hasPrev {
		return nil, Stats{}, &RectError{
			Kind: IssueNoPrevFrame, Rects: []int{-1},
			Detail: "没有上一帧可供增量合成，请先 Render 一帧",
		}
	}
	if bad := outOfBounds(rects, r.cfg.Width, r.cfg.Height); len(bad) > 0 {
		return nil, Stats{}, &RectError{
			Kind: IssueOutOfBounds, Rects: bad,
			Detail: fmt.Sprintf("画幅 %dx%d 之外", r.cfg.Width, r.cfg.Height),
		}
	}
	moved := make([][]geom.Point, len(scene))
	for i, poly := range scene {
		moved[i] = pipe.Transform(poly, r.cfg.Scale, r.cfg.Rotate)
	}
	curBox := bboxOf(moved[0])
	for _, p := range moved[1:] {
		curBox = unionBBox(curBox, bboxOf(p))
	}
	merged, conflicts := Merge(rects)
	var issues []Issue
	for _, pair := range conflicts {
		issues = append(issues, Issue{
			Kind:   IssueOverlap,
			Rect:   pair[0],
			Detail: fmt.Sprintf("矩形 %d 与矩形 %d 面积相交，合并为一个重画区", pair[0], pair[1]),
		})
	}
	if r.Strict && len(conflicts) > 0 {
		rects2 := make([]int, 0, len(conflicts)*2)
		for _, p := range conflicts {
			rects2 = append(rects2, p[0], p[1])
		}
		return nil, Stats{}, &RectError{Kind: IssueOverlap, Rects: rects2, Detail: "严格模式禁止重叠脏矩形"}
	}
	base, out := r.paintIncremental(moved, merged, curBox, r.cfg)
	st := Stats{
		RawRects:      len(rects),
		MergedRects:   len(merged),
		RepaintPixels: unionArea(merged),
		TotalPixels:   r.cfg.Width * r.cfg.Height,
		Issues:        issues,
	}
	st.SavedPixels = st.TotalPixels - st.RepaintPixels
	if r.Verify {
		_, ref, err := fullPipeline(moved, r.cfg, curBox)
		if err != nil {
			return nil, Stats{}, err
		}
		if n := byteDiff(out, ref); n != 0 {
			return nil, Stats{}, fmt.Errorf("Apply 字节对照失败，差异像素 %d", n)
		}
	}
	r.commit(moved, curBox, base, out)
	r.last = st
	return out, st, nil
}
