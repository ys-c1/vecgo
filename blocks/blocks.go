package blocks

import (
	"errors"
	"fmt"
	"math"
	"sort"
)

// Rect 是一个像素块，坐标均为半开区间 [X,X+W) × [Y,Y+H)。
type Rect struct {
	Index int `json:"index"`
	X     int `json:"x"`
	Y     int `json:"y"`
	W     int `json:"w"`
	H     int `json:"h"`
}

// Samples 返回块内像素数。
func (r Rect) Samples() int { return r.W * r.H }

// Stats 是单个块的熵、游程和 RLE 上界统计。
type Stats struct {
	Rect
	// Entropy 是按 256 个灰度符号的经验分布计算的 Shannon 熵，单位 bit/像素。
	Entropy float64 `json:"entropy"`
	// LongestRun 是行主序扫描时实际观察到的最长相等像素游程。
	LongestRun int `json:"longestRun"`
	// RunUpper 是该块内任何游程都不可能超过的绝对上限，即 W*H。
	RunUpper int `json:"runUpper"`
	// CountCap 是本仓库示范 RLE 格式中单字节 count 能表示的最大游程。
	CountCap int `json:"countCap"`
	// Runs 是未考虑 count 封顶时的实际游程数。
	Runs int `json:"runs"`
	// RLEPairs 是按 CountCap=255 切分后需要的 (value,count) 对数。
	RLEPairs int `json:"rlePairs"`
	// RLEBytes 是不含文件头的 RLE 负载字节数（每对 2 字节）。
	RLEBytes int `json:"rleBytes"`
	// WorstCaseBytes 是同样大小块的 RLE 负载最坏上界：每像素一个长度 1 游程。
	WorstCaseBytes int `json:"worstCaseBytes"`
}

// Options 控制固定分块或块级自适应分块。
type Options struct {
	// BlockSize 是固定块边长，也是自适应四叉分块的根块边长。
	BlockSize int
	// Adaptive=false 时 Plan 必须原样返回固定 BlockSize 网格。
	Adaptive bool
	// EntropyThreshold 仅在 Adaptive=true 时使用；熵严格超过阈值的块继续分裂。
	EntropyThreshold float64
	// MinSize 是自适应分裂的最小边长。
	MinSize int
}

// DefaultOptions 返回 8x8 固定块、自适应关闭的选项。
func DefaultOptions() Options {
	return Options{BlockSize: 8, Adaptive: false, EntropyThreshold: 0.75, MinSize: 4}
}

// Plan 根据 opts 生成块。Adaptive=false 时结果就是固定块网格；
// Adaptive=true 时会对高熵根块做确定性四叉分裂。
func Plan(pix []uint8, width, height int, opts Options) ([]Rect, []Stats, error) {
	if width <= 0 || height <= 0 {
		return nil, nil, errors.New("画幅宽高必须为正")
	}
	if len(pix) != width*height {
		return nil, nil, fmt.Errorf("像素长度 %d 与 %dx%d=%d 不符", len(pix), width, height, width*height)
	}
	if opts.BlockSize <= 0 {
		return nil, nil, fmt.Errorf("BlockSize 必须为正，得到 %d", opts.BlockSize)
	}
	if opts.MinSize <= 0 {
		opts.MinSize = opts.BlockSize
	}
	if opts.MinSize > opts.BlockSize {
		opts.MinSize = opts.BlockSize
	}
	if math.IsNaN(opts.EntropyThreshold) || math.IsInf(opts.EntropyThreshold, 0) {
		return nil, nil, fmt.Errorf("EntropyThreshold 非法: %v", opts.EntropyThreshold)
	}

	var rects []Rect
	idx := 0
	for y := 0; y < height; y += opts.BlockSize {
		for x := 0; x < width; x += opts.BlockSize {
			r := Rect{X: x, Y: y, W: minInt(opts.BlockSize, width-x), H: minInt(opts.BlockSize, height-y)}
			if !opts.Adaptive {
				r.Index = idx
				idx++
				rects = append(rects, r)
				continue
			}
			leaves := splitHighEntropy(pix, width, height, r, opts, &idx)
			rects = append(rects, leaves...)
		}
	}
	stats := make([]Stats, 0, len(rects))
	for _, r := range rects {
		stats = append(stats, Analyze(pix, width, r))
	}
	return rects, stats, nil
}

// FixedPlan 是 Adaptive=false 的便捷包装，也用于测试“关闭即回到固定块”。
func FixedPlan(width, height, blockSize int) ([]Rect, error) {
	if width <= 0 || height <= 0 || blockSize <= 0 {
		return nil, errors.New("width、height、blockSize 必须为正")
	}
	var out []Rect
	idx := 0
	for y := 0; y < height; y += blockSize {
		for x := 0; x < width; x += blockSize {
			out = append(out, Rect{
				Index: idx, X: x, Y: y,
				W: minInt(blockSize, width-x),
				H: minInt(blockSize, height-y),
			})
			idx++
		}
	}
	return out, nil
}

func splitHighEntropy(pix []uint8, width, height int, r Rect, opts Options, idx *int) []Rect {
	s := Analyze(pix, width, r)
	canSplit := r.W > opts.MinSize || r.H > opts.MinSize
	if s.Entropy <= opts.EntropyThreshold || !canSplit {
		r.Index = *idx
		*idx++
		return []Rect{r}
	}

	// 按中点四叉分裂；边缘非正方形根块也不会越界。
	x2 := r.X + r.W/2
	y2 := r.Y + r.H/2
	midW := x2 - r.X
	midH := y2 - r.Y
	children := []Rect{
		{X: r.X, Y: r.Y, W: midW, H: midH},
		{X: x2, Y: r.Y, W: r.W - midW, H: midH},
		{X: r.X, Y: y2, W: midW, H: r.H - midH},
		{X: x2, Y: y2, W: r.W - midW, H: r.H - midH},
	}
	var out []Rect
	for _, ch := range children {
		if ch.W <= 0 || ch.H <= 0 {
			continue
		}
		out = append(out, splitHighEntropy(pix, width, height, ch, opts, idx)...)
	}
	return out
}

// Analyze 计算一个块的熵和游程统计。
func Analyze(pix []uint8, width int, r Rect) Stats {
	n := r.Samples()
	counts := make([]int, 256)
	values := make([]uint8, 0, n)
	longest, curRun, runs, pairs := 0, 0, 0, 0
	var last uint8
	have := false

	for y := r.Y; y < r.Y+r.H; y++ {
		base := y * width
		for x := r.X; x < r.X+r.W; x++ {
			v := pix[base+x]
			counts[v]++
			values = append(values, v)
			if have && v == last {
				curRun++
			} else {
				if have {
					longest = maxInt(longest, curRun)
					runs++
					pairs += chunksForRun(curRun, 255)
				}
				last = v
				curRun = 1
				have = true
			}
		}
	}
	if have {
		longest = maxInt(longest, curRun)
		runs++
		pairs += chunksForRun(curRun, 255)
	}

	h := 0.0
	for _, c := range counts {
		if c == 0 {
			continue
		}
		p := float64(c) / float64(n)
		h -= p * math.Log2(p)
	}

	return Stats{
		Rect:           r,
		Entropy:        h,
		LongestRun:     longest,
		RunUpper:       n,
		CountCap:       255,
		Runs:           runs,
		RLEPairs:       pairs,
		RLEBytes:       2 * pairs,
		WorstCaseBytes: 2 * n,
	}
}

// EncodeRLE 编码行主序像素。每个 (value,count) 对占两字节，count 为 1..255；
// 更长游程自动拆成多个 count=255 的对。
func EncodeRLE(pix []uint8) []byte {
	if len(pix) == 0 {
		return nil
	}
	var out []byte
	last := pix[0]
	run := 1
	flush := func() {
		for run > 0 {
			c := minInt(run, 255)
			out = append(out, last, byte(c))
			run -= c
		}
	}
	for _, v := range pix[1:] {
		if v == last && run < 255 {
			run++
			continue
		}
		flush()
		last = v
		run = 1
	}
	flush()
	return out
}

// DecodeRLE 还原 EncodeRLE 的输出。
func DecodeRLE(body []byte) ([]uint8, error) {
	if len(body)%2 != 0 {
		return nil, errors.New("RLE 负载长度必须为偶数")
	}
	var out []uint8
	for i := 0; i < len(body); i += 2 {
		v, c := body[i], int(body[i+1])
		if c == 0 {
			return nil, fmt.Errorf("第 %d 个 RLE 对 count=0 非法", i/2)
		}
		for j := 0; j < c; j++ {
			out = append(out, v)
		}
	}
	return out, nil
}

// SortedEntropyDesc 是报告/测试辅助：按熵从高到低返回统计副本。
func SortedEntropyDesc(stats []Stats) []Stats {
	out := append([]Stats(nil), stats...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Entropy > out[j].Entropy })
	return out
}

func chunksForRun(run, cap int) int {
	return (run + cap - 1) / cap
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
