package coverage

import (
	"fmt"
	"math"
	"sort"
)

// BinResidual 是一个空间分箱的均匀性残差。矩形坐标用归一化画幅坐标
// [0,1] 表示，便于两档画幅直接对照“不均在哪一带”。
type BinResidual struct {
	IX     int     `json:"ix"`
	IY     int     `json:"iy"`
	X0     float64 `json:"x0"`
	Y0     float64 `json:"y0"`
	X1     float64 `json:"x1"`
	Y1     float64 `json:"y1"`
	Count  int     `json:"count"`
	Z      float64 `json:"z"`
	RelDev float64 `json:"relDev"` // (count-expected)/expected
}

// BinReport 是 χ² 空间均匀性检验与最热/最冷分箱定位结果。
type BinReport struct {
	BinsX    int           `json:"binsX"`
	BinsY    int           `json:"binsY"`
	N        int           `json:"n"`
	Expected float64       `json:"expectedPerBin"`
	Chi2     float64       `json:"chi2"`
	DF       int           `json:"df"`
	PValue   float64       `json:"pValue"`
	MaxAbsZ  float64       `json:"maxAbsZ"`
	Hot      []BinResidual `json:"hot"`
	Cold     []BinResidual `json:"cold"`
}

// BinAccumulator 把样本点累计到 bx×by 的归一化空间分箱中。
type BinAccumulator struct {
	bx, by int
	counts []int
	n      int
}

// NewBinAccumulator 创建分箱累加器。
func NewBinAccumulator(bx, by int) *BinAccumulator {
	if bx <= 0 || by <= 0 {
		bx, by = 10, 10
	}
	return &BinAccumulator{bx: bx, by: by, counts: make([]int, bx*by)}
}

// Add 加入画幅像素坐标 (x,y) 处的一个点。
func (b *BinAccumulator) Add(x, y, w, h float64) {
	ix := int(x / w * float64(b.bx))
	iy := int(y / h * float64(b.by))
	if ix < 0 {
		ix = 0
	} else if ix >= b.bx {
		ix = b.bx - 1
	}
	if iy < 0 {
		iy = 0
	} else if iy >= b.by {
		iy = b.by - 1
	}
	b.counts[iy*b.bx+ix]++
	b.n++
}

// Report 计算 χ² 统计量、上侧 p 值以及残差最大的若干热/冷分箱。
// topK 决定保留几个最热和最冷分箱。
func (b *BinAccumulator) Report(topK int) *BinReport {
	if topK <= 0 {
		topK = 3
	}
	bins := b.bx * b.by
	expected := float64(b.n) / float64(bins)
	rep := &BinReport{BinsX: b.bx, BinsY: b.by, N: b.n, Expected: expected, DF: bins - 1}
	res := make([]BinResidual, 0, bins)
	maxZ := 0.0
	for iy := 0; iy < b.by; iy++ {
		for ix := 0; ix < b.bx; ix++ {
			c := b.counts[iy*b.bx+ix]
			d := float64(c) - expected
			rep.Chi2 += d * d / expected
			z := d / math.Sqrt(expected)
			if z < 0 {
				if -z > maxZ {
					maxZ = -z
				}
			} else if z > maxZ {
				maxZ = z
			}
			res = append(res, BinResidual{
				IX: ix, IY: iy,
				X0:    float64(ix) / float64(b.bx),
				Y0:    float64(iy) / float64(b.by),
				X1:    float64(ix+1) / float64(b.bx),
				Y1:    float64(iy+1) / float64(b.by),
				Count: c, Z: z, RelDev: d / expected,
			})
		}
	}
	rep.MaxAbsZ = maxZ
	// χ² 上侧 p = Q(df/2, chi2/2) = 1 - P。
	rep.PValue = 1 - RegularizedGammaP(float64(rep.DF)/2, rep.Chi2/2)

	sort.Slice(res, func(i, j int) bool { return res[i].Z > res[j].Z })
	if topK > len(res) {
		topK = len(res)
	}
	rep.Hot = append([]BinResidual(nil), res[:topK]...)
	cold := append([]BinResidual(nil), res[len(res)-topK:]...)
	// 最冷的按 z 升序展示。
	sort.Slice(cold, func(i, j int) bool { return cold[i].Z < cold[j].Z })
	rep.Cold = cold
	return rep
}

// String 给出最热/最冷分箱的人类可读定位。
func (r BinResidual) String() string {
	return fmt.Sprintf("格(x:%d,y:%d) 归一化区域 x[%.2f,%.2f] y[%.2f,%.2f] 计数=%d 相对期望%+.1f%% z=%+.1f",
		r.IX, r.IY, r.X0, r.X1, r.Y0, r.Y1, r.Count, 100*r.RelDev, r.Z)
}
