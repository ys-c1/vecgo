// Package incr 实现增量渲染：只重画脏矩形覆盖的区域。
//
// 两条路径（全量与增量）共用同一套像素采样坐标，产物逐字节相同。
//
// 视图映射：raster.Fill 的采样坐标由整帧场景包围盒决定（等比铺满画幅）。
// 增量路径不能自己发明映射，否则与 pipe.Run 的产物对不上。因此每帧都照
// pipe.Run 的规则重算映射：映射不变时只重画脏矩形；映射一旦变化（有图形
// 移出原包围盒、图形数量增减等），整帧失效、按新映射重画。这样任意一帧的
// 增量产物都与该帧 pipe.Run 的输出逐字节相同。
package incr

import (
	"log"
	"time"

	"vecgo/config"
	"vecgo/geom"
	"vecgo/pipe"
	"vecgo/post"
	"vecgo/raster"
	"vecgo/shading"
)

// Source 是脏矩形的来源。
type Source string

const (
	// SourceDiff 自动差分（即「全量差分」）：会话比较前后两帧的完整几何，
	// 逐图形按序号配对，取各自包围盒的并集作为脏矩形。
	// 它是几何差分而非逐像素差分——逐像素差分必须先完成一次全量渲染，
	// 耗时会直接吃掉增量收益，因此不采用。
	SourceDiff Source = "diff"
	// SourceMarks 调用方标记：由调用方直接给出像素矩形。
	// 标记不完整时会回退到全量并写日志。
	SourceMarks Source = "marks"
)

// BBoxOf 把若干组点（场景坐标，需已按 cfg 变换）换算成像素矩形并取并集。
// 调用方标记脏矩形时用它最省事：把同一图形前后两帧的顶点都传进来，
// 得到的就是「移动前后都覆盖」的矩形。
func BBoxOf(m raster.Mapping, cfg config.Config, groups ...[]geom.Point) raster.Rect {
	var r raster.Rect
	for _, g := range groups {
		r = r.Union(m.PixelBBox(g, cfg.Width, cfg.Height))
	}
	return r
}

// Stats 是一帧增量的账目。
type Stats struct {
	Source        Source
	RectsIn       int // 合并前的脏矩形个数
	RectsMerged   int // 合并后的脏矩形个数（对外报告值）
	DirtyPixels   int // 合并后矩形覆盖的像素数
	TouchedPixels int // 外扩模糊半径后实际重写的像素数
	TotalPixels   int
	Elapsed       time.Duration // 本次调用耗时
	WholeFrame    bool          // 是否整帧重画（图形数量变化或整幅脏矩形）
	FellBack      bool          // 调用方标记不完整，已回退全量
	FallbackWhy   string
	RectList      []raster.Rect
}

type SessionOptions struct {
	Source      Source
	FallbackLog *log.Logger // 回退时写日志；nil 时回退仍发生，只是不落日志
}

type Session struct {
	cfg    config.Config
	source Source
	log    *log.Logger

	haveFrame   bool
	mapping     raster.Mapping
	prevMoved   [][]geom.Point
	prevPreBlur *raster.Image // 上一帧模糊前（gamma 之后）的完整图，增量重填的底座
	prevFinal   *raster.Image // 最终图像
}

func NewSession(cfg config.Config, opts SessionOptions) (*Session, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	src := opts.Source
	if src == "" {
		src = SourceDiff
	}
	if src != SourceDiff && src != SourceMarks {
		return nil, &DirtyError{Kind: ErrBadSource, W: cfg.Width, H: cfg.Height}
	}
	return &Session{cfg: cfg, source: src, log: opts.FallbackLog}, nil
}

// Mapping 返回当前帧使用的采样映射。调用方据此把场景坐标换算成像素矩形。
// 首帧之前为零值，此时像素矩形应由调用方按预期画幅自行给出。
func (s *Session) Mapping() raster.Mapping { return s.mapping }

// HasPrevious 表示会话里已有上一帧。
func (s *Session) HasPrevious() bool { return s.haveFrame }

// Frame 渲染一帧。首帧总是全量；marks 仅在 SourceMarks 下使用。
func (s *Session) Frame(scene [][]geom.Point, marks []raster.Rect) (*raster.Image, Stats, error) {
	start := time.Now()
	stats := Stats{Source: s.source, TotalPixels: s.cfg.Width * s.cfg.Height}
	moved, err := prepare(scene, s.cfg)
	if err != nil {
		return nil, stats, err
	}

	if !s.haveFrame {
		if s.source == SourceMarks && len(marks) > 0 {
			return nil, stats, &DirtyError{
				Kind: ErrNoPreviousFrame, Index: 0, Rect: marks[0],
				W: s.cfg.Width, H: s.cfg.Height,
			}
		}
		m, err := raster.SceneMapping(moved)
		if err != nil {
			return nil, stats, err
		}
		final, pre, err := fullFrame(moved, s.cfg, m)
		if err != nil {
			return nil, stats, err
		}
		s.commit(moved, pre, final, m)
		// 首帧没有可比对的上一帧，本来就是全量。
		stats.WholeFrame = true
		stats.Elapsed = time.Since(start)
		return final.Clone(), stats, nil
	}

	// 调用方标记先验形态：越界、退化、互相重叠都在这里拦下，
	// 与后续走增量还是整帧重画无关。
	if s.source == SourceMarks {
		if err := validateMarks(marks, s.cfg.Width, s.cfg.Height); err != nil {
			return nil, stats, err
		}
	}

	// 映射变了，采样坐标整体改变，脏矩形失去意义。
	newMapping, mapErr := raster.SceneMapping(moved)
	if mapErr != nil {
		return nil, stats, mapErr
	}
	if !newMapping.Same(s.mapping) {
		s.logf("整帧重画: 场景包围盒变化，采样映射改变; 调用方标记 %d 个已忽略", len(marks))
		return s.fullPass(moved, newMapping, stats, start)
	}

	need, whole := s.diffRects(moved)
	var rects []raster.Rect
	switch s.source {
	case SourceDiff:
		if whole {
			return s.fullPass(moved, s.mapping, stats, start)
		}
		rects = need
	case SourceMarks:
		stats.RectsIn = len(marks)
		if missing, ok := firstUncovered(marks, need); !ok {
			why := "调用方标记未覆盖场景变化区域"
			stats.FellBack = true
			stats.FallbackWhy = why
			s.logf("回退全量: %s; 未覆盖的变化区域=%+v; 调用方标记=%v", why, missing, marks)
			return s.fullPass(moved, s.mapping, stats, start)
		}
		rects = marks
	}

	stats.RectsIn = len(rects)
	merged := mergeRects(rects)
	stats.RectsMerged = len(merged)
	stats.RectList = merged
	fullRect := raster.Rect{X0: 0, Y0: 0, X1: s.cfg.Width, Y1: s.cfg.Height}
	for _, r := range merged {
		stats.DirtyPixels += r.Area()
		if r == fullRect {
			// 脏矩形就是整幅，等价于整帧重画；如实记账，不计入增量帧。
			stats.WholeFrame = true
		}
	}
	if len(merged) == 0 {
		// 零变化：上一帧原样返回。
		stats.Elapsed = time.Since(start)
		return s.prevFinal.Clone(), stats, nil
	}

	pre := s.prevPreBlur.Clone()
	for _, r := range merged {
		if err := pre.FillRectInto(moved, s.cfg.Supersample, s.mapping, r); err != nil {
			return nil, stats, err
		}
		if err := shading.ApplyRect(pre, s.cfg.Shading, 1.0, 0.0, r); err != nil {
			return nil, stats, err
		}
		if err := post.GammaRect(pre, s.cfg.Gamma, r); err != nil {
			return nil, stats, err
		}
	}

	var final *raster.Image
	if s.cfg.Blur == 0 {
		final = pre.Clone()
		stats.TouchedPixels = stats.DirtyPixels
	} else {
		final = s.prevFinal.Clone()
		expanded := make([]raster.Rect, len(merged))
		for i, r := range merged {
			expanded[i] = r.Expand(s.cfg.Blur, s.cfg.Width, s.cfg.Height)
			stats.TouchedPixels += expanded[i].Area()
		}
		if err := post.BlurInto(pre, final, s.cfg.Blur, expanded); err != nil {
			return nil, stats, err
		}
	}

	s.commit(moved, pre, final, s.mapping)
	stats.Elapsed = time.Since(start)
	return final.Clone(), stats, nil
}

// Incremental 与 Frame 相同，但要求会话已有上一帧；否则返回
// ErrNoPreviousFrame（带矩形号）。用于确保测的是增量路径而不是首帧全量。
func (s *Session) Incremental(scene [][]geom.Point, marks []raster.Rect) (*raster.Image, Stats, error) {
	if !s.haveFrame {
		var r raster.Rect
		if len(marks) > 0 {
			r = marks[0]
		}
		return nil, Stats{Source: s.source, TotalPixels: s.cfg.Width * s.cfg.Height},
			&DirtyError{Kind: ErrNoPreviousFrame, Index: 0, Rect: r, W: s.cfg.Width, H: s.cfg.Height}
	}
	return s.Frame(scene, marks)
}

// Full用会话锁定的映射整帧重画一次，是增量的比对基准。
// 不改动会话状态，可以在任意时刻调用。
func (s *Session) Full(scene [][]geom.Point) (*raster.Image, error) {
	moved, err := prepare(scene, s.cfg)
	if err != nil {
		return nil, err
	}
	final, _, err := fullFrame(moved, s.cfg, s.mapping)
	return final, err
}

func (s *Session) fullPass(moved [][]geom.Point, m raster.Mapping, stats Stats, start time.Time) (*raster.Image, Stats, error) {
	final, pre, err := fullFrame(moved, s.cfg, m)
	if err != nil {
		return nil, stats, err
	}
	s.commit(moved, pre, final, m)
	stats.WholeFrame = true
	stats.RectsMerged = 1
	stats.RectList = []raster.Rect{{X0: 0, Y0: 0, X1: s.cfg.Width, Y1: s.cfg.Height}}
	stats.DirtyPixels = stats.TotalPixels
	stats.TouchedPixels = stats.TotalPixels
	stats.Elapsed = time.Since(start)
	return final.Clone(), stats, nil
}

func (s *Session) commit(moved [][]geom.Point, pre, final *raster.Image, m raster.Mapping) {
	s.haveFrame = true
	s.mapping = m
	s.prevMoved = moved
	s.prevPreBlur = pre
	s.prevFinal = final
}

func (s *Session) logf(format string, args ...any) {
	if s.log != nil {
		s.log.Printf(format, args...)
	}
}

// FullFrame 用给定映射整帧渲染，返回最终图与模糊前的图。
// 当映射等于 pipe.Run 自算的映射时，最终图与 pipe.Run 逐字节一致。
func FullFrame(scene [][]geom.Point, cfg config.Config, m raster.Mapping) (*raster.Image, *raster.Image, error) {
	moved, err := prepare(scene, cfg)
	if err != nil {
		return nil, nil, err
	}
	return fullFrame(moved, cfg, m)
}

func fullFrame(moved [][]geom.Point, cfg config.Config, m raster.Mapping) (*raster.Image, *raster.Image, error) {
	im, err := raster.New(cfg.Width, cfg.Height)
	if err != nil {
		return nil, nil, err
	}
	if err := im.FillWithMapping(moved, cfg.Supersample, m); err != nil {
		return nil, nil, err
	}
	if err := shading.Apply(im, cfg.Shading, 1.0, 0.0); err != nil {
		return nil, nil, err
	}
	if err := post.Gamma(im, cfg.Gamma); err != nil {
		return nil, nil, err
	}
	pre := im.Clone()
	out, err := post.Blur(im, cfg.Blur)
	if err != nil {
		return nil, nil, err
	}
	return out, pre, nil
}

func prepare(scene [][]geom.Point, cfg config.Config) ([][]geom.Point, error) {
	if len(scene) == 0 {
		return nil, &DirtyError{Kind: ErrEmptyScene, W: cfg.Width, H: cfg.Height}
	}
	moved := make([][]geom.Point, len(scene))
	for i, poly := range scene {
		if len(poly) < 3 {
			return nil, &DirtyError{Kind: ErrBadGeometry, Index: i, W: cfg.Width, H: cfg.Height}
		}
		moved[i] = pipe.Transform(poly, cfg.Scale, cfg.Rotate)
	}
	return moved, nil
}

// diffRects 比较前后两帧的已变换几何，返回需要重画的矩形。
// 逐图形按序号配对；数量变化直接视为整帧。
func (s *Session) diffRects(moved [][]geom.Point) (rects []raster.Rect, whole bool) {
	full := raster.Rect{X0: 0, Y0: 0, X1: s.cfg.Width, Y1: s.cfg.Height}
	if len(moved) != len(s.prevMoved) {
		return []raster.Rect{full}, true
	}
	var out []raster.Rect
	for i := range moved {
		if samePoints(s.prevMoved[i], moved[i]) {
			continue
		}
		r := s.mapping.PixelBBox(s.prevMoved[i], s.cfg.Width, s.cfg.Height).
			Union(s.mapping.PixelBBox(moved[i], s.cfg.Width, s.cfg.Height))
		if r.Empty() {
			// 图形完全在画幅外，画面不受影响。
			continue
		}
		out = append(out, r)
	}
	if len(out) == 0 {
		return nil, false
	}
	all := out[0]
	for _, r := range out[1:] {
		all = all.Union(r)
	}
	if all == full {
		return []raster.Rect{full}, true
	}
	return out, false
}

func samePoints(a, b []geom.Point) bool {
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

func validateMarks(marks []raster.Rect, w, h int) error {
	for i, r := range marks {
		if !r.InBounds(w, h) {
			return &DirtyError{Kind: ErrRectOutOfBounds, Index: i, Rect: r, W: w, H: h}
		}
	}
	for i := 0; i < len(marks); i++ {
		for j := i + 1; j < len(marks); j++ {
			if marks[i].Overlaps(marks[j]) {
				return &DirtyError{
					Kind: ErrOverlapConflict, Index: j, Other: i,
					Rect: marks[j], OtherRect: marks[i], W: w, H: h,
				}
			}
		}
	}
	return nil
}

// firstUncovered 找出第一个没有被任何标记完整覆盖的变化区域。
// 全部被覆盖时返回 ok=true；否则返回缺失的区域供日志记录。
func firstUncovered(marks, need []raster.Rect) (raster.Rect, bool) {
	for _, n := range need {
		covered := false
		for _, m := range marks {
			if m.X0 <= n.X0 && m.Y0 <= n.Y0 && m.X1 >= n.X1 && m.Y1 >= n.Y1 {
				covered = true
				break
			}
		}
		if !covered {
			return n, false
		}
	}
	return raster.Rect{}, true
}

// MergeRects 供调用方把可能重叠/相邻的矩形整理成互不重叠的一份。
// 有面积重叠的矩形必须先并成一个，否则 Frame 会返回 ErrOverlapConflict；
// 仅仅共边的矩形可以合并，也可以保留，取决于是不是愿意多画一点空白。
func MergeRects(in []raster.Rect) []raster.Rect {
	out := append([]raster.Rect{}, in...)
	// 先把有面积重叠的强行并掉（合并空白在这里不可避免）。
	for {
		mergedAny := false
		for i := 0; i < len(out); i++ {
			for j := i + 1; j < len(out); j++ {
				if !out[i].Overlaps(out[j]) {
					continue
				}
				out[i] = out[i].Union(out[j])
				out = append(out[:j], out[j+1:]...)
				mergedAny = true
				j--
			}
		}
		if !mergedAny {
			break
		}
	}
	// 再尝试无代价的相邻合并。
	return mergeRects(out)
}

// mergeRects 合并相邻（含共边）的矩形。合并不得引入额外空白，
// 否则宁可保留两个矩形——多出的空白只会拖慢耗时。
func mergeRects(in []raster.Rect) []raster.Rect {
	out := append([]raster.Rect{}, in...)
	for {
		mergedAny := false
		for i := 0; i < len(out); i++ {
			for j := i + 1; j < len(out); j++ {
				if !out[i].Touches(out[j]) {
					continue
				}
				u := out[i].Union(out[j])
				if u.Area() > out[i].Area()+out[j].Area() {
					continue
				}
				out[i] = u
				out = append(out[:j], out[j+1:]...)
				mergedAny = true
				j--
			}
		}
		if !mergedAny {
			break
		}
	}
	return out
}
