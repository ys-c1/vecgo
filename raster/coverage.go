package raster

import (
	"container/heap"
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"

	"vecgo/geom"
)

// TopPixelCount 是覆盖率统计默认列出的最高覆盖像素数。
const TopPixelCount = 10

// PixelCoverage 描述一个像素的覆盖情况。
//
// Count 是写入该像素的图形数：只要某个图形的任意一个子采样点落在该
// 像素内，这个图形就把该像素记一次。同一个图形无论贡献了几个子采样点
// 都只记一次；不同图形各记一次。因此 Count > 1 即表示该像素被反复
// 填充，这正是接缝处斑驳的来源。
type PixelCoverage struct {
	X, Y   int
	Count  int
	Shapes []int // 覆盖该像素的图形下标，升序
	idx    int   // 行优先下标，仅用于稳定排序
}

// Bucket 是覆盖次数分桶表的一行。
//
// Interval 形如 "1"、"2"、"3+"；"3+" 表示 3 次及以上。Max < 0 表示
// 该档没有上界。Pixels 是落入该档的像素数，Writes 是这些像素的覆盖
// 次数之和（便于与总次数逐行核对）。
type Bucket struct {
	Interval string
	Min, Max int
	Pixels   int
	Writes   uint64
}

// CoverageStats 记录一次 Fill 的覆盖率统计。
//
// 统计只观察填充过程，不改变写入图像的任何像素值，因此启用统计前后
// 导出的 PGM 逐字节相同。
//
// 计数口径：某图形只要有任意一个子采样点落在像素内，就把该像素记
// 一次；同一图形的多个子采样点不重复计。因此 Counts[i] 是覆盖该像素
// 的图形个数，也就是该像素被不同图形反复填充的次数。注意 Fill 本身
// 对每个像素只做一次最终赋值，这里的“写入次数”量化的是多图形叠加，
// 而不是内存写操作次数。
//
// Counts、Hist、TotalWrites 等字段由 FillStats 填充，零值无需构造。
type CoverageStats struct {
	W, H        int
	Supersample int
	Polygons    int
	BBox        [4]float64 // minx, miny, maxx, maxy（世界坐标）

	Counts   []uint32  // 每个像素被多少个图形写入，行优先，长度 W*H
	Hist     []uint64  // Hist[k] = 恰好被 k 个图形覆盖的像素数，长度 Polygons+1
	Vertices []int     // 每个图形的顶点数
	Descs    []string  // 每个图形的一句话描述（类型与顶点，世界坐标）
	Areas    []float64 // 每个图形的面积（像素单位）
	Touched  []int     // 每个图形触及的像素数（有任一子采样点落入即算）

	TotalWrites   uint64  // 所有像素覆盖次数之和 = Σ Counts
	CoveredPixels int     // Count > 0 的像素数
	AreaSum       float64 // 各图形面积之和（像素单位，重叠部分重复计）
	WorldAreaSum  float64 // 各图形面积之和（世界单位）

	PixelPerArea float64 // 世界面积到像素面积的换算系数

	Top []PixelCoverage // 覆盖次数最高的 TopPixelCount 个像素

	minx, miny, spanx, spany float64 // 与 Fill 完全一致的映射参数
}

// begin 由 fill 在开始扫描像素前调用，重置并分配统计缓冲。
func (s *CoverageStats) begin(w, h, supersample int, polys [][]geom.Point, minx, miny, spanx, spany float64) {
	s.W, s.H = w, h
	s.Supersample = supersample
	s.Polygons = len(polys)
	s.BBox = [4]float64{minx, miny, minx + spanx, miny + spany}
	s.minx, s.miny, s.spanx, s.spany = minx, miny, spanx, spany

	s.Counts = make([]uint32, w*h)
	// Hist 至少要有 0 与 1 两档；Top 的图形明细个数也不超过图形数，
	// 因此单图形场景同样得到完整的核对数据。
	if n := len(polys) + 1; n >= 2 {
		s.Hist = make([]uint64, n)
	} else {
		s.Hist = make([]uint64, 2)
	}
	s.Vertices = make([]int, len(polys))
	s.Descs = make([]string, len(polys))
	s.Areas = make([]float64, len(polys))
	s.Touched = make([]int, len(polys))
	for i, p := range polys {
		s.Vertices[i] = len(p)
		s.Descs[i] = shapeDesc(p)
	}

	s.TotalWrites = 0
	s.CoveredPixels = 0
	s.AreaSum = 0
	s.WorldAreaSum = 0
	s.Top = nil

	// 世界面积映射到像素面积：x、y 两个方向分别按画幅与包围盒的
	// 比值缩放。与 Fill 的采样映射完全一致。
	sx := float64(w) / spanx
	sy := float64(h) / spany
	s.PixelPerArea = sx * sy
	for i, p := range polys {
		world := polyArea(p)
		s.WorldAreaSum += world
		s.Areas[i] = world * s.PixelPerArea
		s.AreaSum += s.Areas[i]
	}
}

// record 记录一个像素的覆盖次数；seen 是该像素命中过的图形标记。
func (s *CoverageStats) record(idx int, seen []bool) {
	n := 0
	for k, v := range seen {
		if v {
			n++
			if k < len(s.Touched) {
				s.Touched[k]++
			}
		}
	}
	s.Counts[idx] = uint32(n)
	if n == 0 {
		return
	}
	s.CoveredPixels++
	s.TotalWrites += uint64(n)
	if n < len(s.Hist) {
		s.Hist[n]++
	}
}

// finish 在像素扫描结束后挑出覆盖次数最高的像素，并补上图形明细。
func (s *CoverageStats) finish(polys [][]geom.Point) {
	s.selectTop(TopPixelCount)
	for i := range s.Top {
		s.Top[i].Shapes = s.shapesAt(polys, s.Top[i].X, s.Top[i].Y)
	}
}

// Diff 返回覆盖总次数与面积和的差（总次数 - 面积和）。
//
// 两者单位一致：面积和以像素为单位，且重叠部分重复计。对于恰好铺满
// 且互不重叠的图形，该差值为 0；为正说明有像素被多个图形重复填充，
// 其量级就是重复填充带来的额外写入次数（叠加边界离散误差）。
func (s *CoverageStats) Diff() float64 {
	return cleanZero(float64(s.TotalWrites) - s.AreaSum)
}

// cleanZero 把浮点噪声（|x|<1e-9）归零。面积由世界坐标换算而来，
// 恰好铺满时会有 ±1e-13 的残差，不应显示成 -0.000，更不该被当成
// 真实重复写入。
func cleanZero(x float64) float64 {
	if math.Abs(x) < 1e-9 {
		return 0
	}
	return x
}

// RelativeDiff 返回差值相对面积和的百分比；面积和为 0 时返回 0。
func (s *CoverageStats) RelativeDiff() float64 {
	if s.AreaSum == 0 {
		return 0
	}
	return 100 * s.Diff() / s.AreaSum
}

// Buckets 返回三档分桶：1 次、2 次、3 次及以上。
//
// 未覆盖像素（0 次）不计入任何一档；它们由 CoveredPixels 与画幅
// 像素数之差单独给出。
func (s *CoverageStats) Buckets() []Bucket {
	one := int(s.histAt(1))
	two := int(s.histAt(2))
	var threePixels int
	var threeWrites uint64
	for k := 3; k < len(s.Hist); k++ {
		threePixels += int(s.Hist[k])
		threeWrites += uint64(k) * s.Hist[k]
	}
	return []Bucket{
		{Interval: "1", Min: 1, Max: 1, Pixels: one, Writes: uint64(one)},
		{Interval: "2", Min: 2, Max: 2, Pixels: two, Writes: 2 * uint64(two)},
		{Interval: "3+", Min: 3, Max: -1, Pixels: threePixels, Writes: threeWrites},
	}
}

func (s *CoverageStats) histAt(k int) uint64 {
	if k < 0 || k >= len(s.Hist) {
		return 0
	}
	return s.Hist[k]
}

// WriteBucketCSV 把分桶表导出为 CSV，恰好两列：次数区间、像素数。
//
// 三行依次为 1、2、3+。各行像素数之和等于 CoveredPixels；把每行的
// 次数区间代表值乘以像素数后求和，并与 Buckets 的 Writes 列核对，
// 即可逐行复核。
func (s *CoverageStats) WriteBucketCSV(w io.Writer) error {
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{"次数区间", "像素数"}); err != nil {
		return err
	}
	for _, b := range s.Buckets() {
		if err := cw.Write([]string{b.Interval, strconv.Itoa(b.Pixels)}); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// Report 把统计摘要写到 w，包括总次数、面积和及其具体差值、分桶表和
// 覆盖次数最高的 10 个像素。
func (s *CoverageStats) Report(w io.Writer) error {
	fmt.Fprintln(w, "填充覆盖率统计")
	fmt.Fprintf(w, "  画幅 %d×%d，图形 %d 个，超采样 %d×%d\n",
		s.W, s.H, s.Polygons, s.Supersample, s.Supersample)
	fmt.Fprintf(w, "  覆盖总次数 %d\n", s.TotalWrites)
	fmt.Fprintf(w, "  面积和（像素单位，重叠重复计）%.3f\n", s.AreaSum)
	fmt.Fprintf(w, "  差值（覆盖总次数 - 面积和）%+.3f\n", s.Diff())
	fmt.Fprintf(w, "  相对差值 %+.4f%%\n", s.RelativeDiff())
	fmt.Fprintf(w, "  有覆盖像素 %d / %d，未覆盖 %d\n",
		s.CoveredPixels, s.W*s.H, s.W*s.H-s.CoveredPixels)
	fmt.Fprintln(w, "  计数口径：图形有任一子采样点落在像素内即记该像素一次，同一图形不重复计")

	fmt.Fprintln(w, "分桶统计:")
	fmt.Fprintf(w, "  %-8s %s\n", "次数区间", "像素数")
	for _, b := range s.Buckets() {
		fmt.Fprintf(w, "  %-10s %d\n", b.Interval, b.Pixels)
	}
	fmt.Fprintf(w, "  分桶像素数之和 %d（应等于有覆盖像素 %d）\n",
		s.bucketPixels(), s.CoveredPixels)

	fmt.Fprintf(w, "图形一览（下标、顶点数、像素面积、触及像素数；顶点为填充坐标，已应用缩放/旋转）:\n")
	for i := range s.Areas {
		fmt.Fprintf(w, "  #%d：%d 顶点，面积 %.3f，触及像素 %d，边界缺口 %+.3f\n      %s\n",
			i, s.Vertices[i], s.Areas[i], s.Touched[i],
			cleanZero(float64(s.Touched[i])-s.Areas[i]), s.Descs[i])
	}
	fmt.Fprintf(w, "  差值分解：Σ(触及像素 - 面积) = %+.3f，应等于上面的差值 %+.3f\n",
		s.TouchGap(), s.Diff())

	fmt.Fprintf(w, "覆盖次数最高的 %d 个像素（图形明细见上方一览）:\n", len(s.Top))
	for i, p := range s.Top {
		fmt.Fprintf(w, "  %2d. (x=%d,y=%d) 次数=%d ", i+1, p.X, p.Y, p.Count)
		switch len(p.Shapes) {
		case 0:
			fmt.Fprintln(w, "无图形覆盖")
		case 1:
			fmt.Fprintf(w, "仅图形 %s 覆盖，无叠加\n", indices(p.Shapes))
		default:
			fmt.Fprintf(w, "由 %d 个图形叠加：%s\n", len(p.Shapes), indices(p.Shapes))
		}
	}

	if issues := s.Verify(); len(issues) == 0 {
		fmt.Fprintln(w, "账目核对：通过（Counts/分桶/面积/差值分解等七项等式全部成立）")
	} else {
		fmt.Fprintln(w, "账目核对：不通过")
		for _, issue := range issues {
			fmt.Fprintln(w, "  -", issue)
		}
	}
	return nil
}

// indices 把图形下标格式化为 "#0、#1、#2"。
func indices(shapes []int) string {
	out := ""
	for i, k := range shapes {
		if i > 0 {
			out += "、"
		}
		out += "#" + strconv.Itoa(k)
	}
	return out
}

func (s *CoverageStats) bucketPixels() int {
	n := 0
	for _, b := range s.Buckets() {
		n += b.Pixels
	}
	return n
}

// TouchGap 返回 Σ(图形触及像素数 − 图形像素面积)，即差值按图形分解后
// 的和。它恒等于 Diff()：覆盖总次数 = Σ 触及像素数，像素面积和 =
// Σ 图形面积。每项即该图形边界上半覆盖像素带来的缺口。
func (s *CoverageStats) TouchGap() float64 {
	gap := 0.0
	for i := range s.Areas {
		gap += float64(s.Touched[i]) - s.Areas[i]
	}
	return cleanZero(gap)
}

// Verify 逐行核对统计账目，返回所有不成立的等式。
//
// 核对项：
//  1. Σ Counts == TotalWrites；
//  2. 各分桶像素数之和 == CoveredPixels，且 CoveredPixels <= W*H；
//  3. 各分桶的 Writes 之和 == TotalWrites；
//  4. Hist 各项像素数之和 == CoveredPixels；
//  5. Top 中每个像素的 Count 与 Counts 表一致，图形明细个数等于 Count。
//
// 这些等式与 WriteBucketCSV 导出的两列一一对应，供逐行复核。
func (s *CoverageStats) Verify() []string {
	var issues []string

	var sumCounts uint64
	for _, c := range s.Counts {
		sumCounts += uint64(c)
	}
	if sumCounts != s.TotalWrites {
		issues = append(issues, fmt.Sprintf(
			"Σ Counts=%d 与 TotalWrites=%d 不符", sumCounts, s.TotalWrites))
	}

	var bucketPixels int
	var bucketWrites uint64
	for _, b := range s.Buckets() {
		bucketPixels += b.Pixels
		bucketWrites += b.Writes
	}
	if bucketPixels != s.CoveredPixels {
		issues = append(issues, fmt.Sprintf(
			"分桶像素数之和=%d 与 CoveredPixels=%d 不符", bucketPixels, s.CoveredPixels))
	}
	if bucketWrites != s.TotalWrites {
		issues = append(issues, fmt.Sprintf(
			"分桶 Writes 之和=%d 与 TotalWrites=%d 不符", bucketWrites, s.TotalWrites))
	}
	if s.CoveredPixels > s.W*s.H {
		issues = append(issues, fmt.Sprintf(
			"CoveredPixels=%d 超过画幅像素数 %d", s.CoveredPixels, s.W*s.H))
	}
	var areaSum float64
	for _, a := range s.Areas {
		areaSum += a
	}
	if math.Abs(areaSum-s.AreaSum) > 1e-9 {
		issues = append(issues, fmt.Sprintf(
			"逐图形面积之和=%.6f 与 AreaSum=%.6f 不符", areaSum, s.AreaSum))
	}
	if gap := s.TouchGap(); math.Abs(gap-s.Diff()) > 1e-6 {
		issues = append(issues, fmt.Sprintf(
			"差值分解 Σ(触及-面积)=%.6f 与 Diff=%.6f 不符", gap, s.Diff()))
	}

	var histPixels uint64
	for k, n := range s.Hist {
		if k >= 1 {
			histPixels += n
		}
	}
	if int(histPixels) != s.CoveredPixels {
		issues = append(issues, fmt.Sprintf(
			"Hist 像素数之和=%d 与 CoveredPixels=%d 不符", histPixels, s.CoveredPixels))
	}
	for _, p := range s.Top {
		if idx := p.Y*s.W + p.X; idx < 0 || idx >= len(s.Counts) {
			issues = append(issues, fmt.Sprintf(
				"Top 像素 (x=%d,y=%d) 越界", p.X, p.Y))
			continue
		}
		if int(s.Counts[p.Y*s.W+p.X]) != p.Count {
			issues = append(issues, fmt.Sprintf(
				"Top 像素 (x=%d,y=%d) Count=%d 与 Counts 表不符", p.X, p.Y, p.Count))
		}
		if len(p.Shapes) != p.Count {
			issues = append(issues, fmt.Sprintf(
				"Top 像素 (x=%d,y=%d) 图形明细 %d 个与覆盖次数 %d 不符",
				p.X, p.Y, len(p.Shapes), p.Count))
		}
	}
	return issues
}

// shapeDesc 用世界坐标的一句话描述图形：顶点表。顶点太多时只列前 8 个。
func shapeDesc(p []geom.Point) string {
	const maxPts = 8
	pts := p
	if len(pts) > maxPts {
		pts = pts[:maxPts]
	}
	out := "顶点=["
	for i, q := range pts {
		if i > 0 {
			out += " "
		}
		out += fmt.Sprintf("(%.2f,%.2f)", q.X, q.Y)
	}
	if len(p) > maxPts {
		out += fmt.Sprintf(" …共%d", len(p))
	}
	return out + "]"
}

// selectTop 用大小为 k 的最小堆挑出覆盖次数最高的 k 个像素。
// 排序规则：次数降序；次数相同时按行优先（y 小者优先，再 x 小者优先）。
func (s *CoverageStats) selectTop(k int) {
	h := make(topHeap, 0, k)
	for idx, c := range s.Counts {
		if c == 0 {
			continue
		}
		e := PixelCoverage{
			X:     idx % s.W,
			Y:     idx / s.W,
			Count: int(c),
			idx:   idx,
		}
		if len(h) < k {
			heap.Push(&h, e)
			continue
		}
		if betterPixel(e, h[0]) {
			heap.Pop(&h)
			heap.Push(&h, e)
		}
	}
	out := make([]PixelCoverage, len(h))
	copy(out, h)
	sort.Slice(out, func(i, j int) bool { return betterPixel(out[i], out[j]) })
	s.Top = out
}

// betterPixel 报告 a 是否应排在 b 前面。
func betterPixel(a, b PixelCoverage) bool {
	if a.Count != b.Count {
		return a.Count > b.Count
	}
	return a.idx < b.idx
}

// shapesAt 重新检查一个像素，列出覆盖它的图形下标（升序）。
// 只对 Top 中的少量像素调用，代价可以忽略。
func (s *CoverageStats) shapesAt(polys [][]geom.Point, x, y int) []int {
	seen := make([]bool, len(polys))
	for sj := 0; sj < s.Supersample; sj++ {
		for si := 0; si < s.Supersample; si++ {
			px := s.minx + (float64(x)+(float64(si)+0.5)/float64(s.Supersample))*s.spanx/float64(s.W)
			py := s.miny + (float64(y)+(float64(sj)+0.5)/float64(s.Supersample))*s.spany/float64(s.H)
			for k, p := range polys {
				if geom.Inside(p, px, py) {
					seen[k] = true
				}
			}
		}
	}
	out := make([]int, 0, len(polys))
	for k, v := range seen {
		if v {
			out = append(out, k)
		}
	}
	return out
}

// topHeap 是 selectTop 的最小堆：堆顶是当前保留集合中最差的一个。
type topHeap []PixelCoverage

func (h topHeap) Len() int { return len(h) }
func (h topHeap) Less(i, j int) bool {
	return betterPixel(h[j], h[i])
}
func (h topHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *topHeap) Push(x interface{}) {
	*h = append(*h, x.(PixelCoverage))
}
func (h *topHeap) Pop() interface{} {
	old := *h
	n := len(old)
	v := old[n-1]
	*h = old[:n-1]
	return v
}

// polyArea 返回多边形面积（世界单位，鞋带公式取绝对值）。
func polyArea(p []geom.Point) float64 {
	if len(p) < 3 {
		return 0
	}
	sum := 0.0
	for i := range p {
		j := (i + 1) % len(p)
		sum += p[i].X*p[j].Y - p[j].X*p[i].Y
	}
	return math.Abs(sum) / 2
}
