package raster

import (
	"errors"

	"vecgo/geom"
)

// crossRule 决定扫描线恰好穿过顶点时，该顶点被计几次。
//
// 一致性条件（证明见 FillScanline 注释）：设顶点 v 落在扫描线 y 上，
// 其两条邻边为 e1、e2，则 v 被计入的次数 c(v) 必须满足
//
//	c(v) ≡ [e1 活跃] + [e2 活跃] (mod 2)
//
// 即：邻边分居扫描线两侧（通过顶点）时 c(v) 必须为奇；
// 邻边在同侧（极值顶点）时 c(v) 必须为偶。
type crossRule int

const (
	// ruleHalfOpen 半开区间 [min,max)：每条边只携带自己 y 较小的端点。
	// 顶点落在扫描线上时，它在两条邻边中恰好被计一次：
	//   通过顶点计 1 次，极值顶点计 0 或 2 次，
	// 恒满足一致性条件，与 geom.Inside 的射线计数同奇偶。
	ruleHalfOpen crossRule = iota
	// ruleClosed 闭区间 [min,max]：顶点被两条邻边各计一次（共 2 次）。
	// 极值顶点 2≡0 侥幸一致；通过顶点 2≡0 与射线法的 1 矛盾，
	// 该扫描行上顶点右侧的内外全部反转，形成豁口。
	ruleClosed
)

// edgeCrossing 计算边 (a,b) 与扫描线 y 的交点横坐标。
// ok=false 表示该边在此扫描线上不活跃（不计数）。
func edgeCrossing(a, b geom.Point, y float64, rule crossRule) (cx float64, ok bool) {
	if a.Y == b.Y {
		// 水平边与扫描线重合时交点不可数，规定其贡献 0 次；
		// 它的两个端点由非水平邻边按计数规则归属。
		return 0, false
	}
	lo, hi := a.Y, b.Y
	if lo > hi {
		lo, hi = hi, lo
	}
	active := y >= lo && y < hi // 半开区间 [min,max)
	if rule == ruleClosed {
		active = y >= lo && y <= hi // 闭区间 [min,max]（旧规则，有豁口）
	}
	if !active {
		return 0, false
	}
	t := (y - a.Y) / (b.Y - a.Y)
	return a.X + (b.X-a.X)*t, true
}

// FillScanline 用扫描线奇偶翻转填充（半开区间计数规则）。
//
// 一致性证明：对不在任何边上的采样点 p=(px,py)，扫描线算法在直线
// ℓ: y=py 上求出全部交点 {xi}，p 处的翻转次数是满足 xi>px 的交点个数；
// 从 p 向右的射线正是 ℓ 的右半支，它与多边形边界的交点集合同样是
// {xi : xi>px}。两边数的是同一个集合，奇偶性必然相同——前提是两边
// 对"边是否活跃"的判定相同，即顶点计数规则满足上面 crossRule 注释
// 中的同余条件。半开区间规则使每个线上顶点恰好被其下方邻边计一次，
// 条件恒成立，故结果与 Fill（逐点 geom.Inside 判定）逐像素一致。
func (im *Image) FillScanline(polys [][]geom.Point, supersample int) error {
	return im.fillScanline(polys, supersample, ruleHalfOpen)
}

func (im *Image) fillScanline(polys [][]geom.Point, supersample int, rule crossRule) error {
	if supersample < 1 {
		return errors.New("supersample 必须为正")
	}
	if err := checkPolys(polys); err != nil {
		return err
	}
	minx, miny, spanx, spany := polysBBox(polys)
	den := supersample * supersample
	for row := 0; row < im.H; row++ {
		for col := 0; col < im.W; col++ {
			hit := 0
			for sj := 0; sj < supersample; sj++ {
				py := miny + (float64(row)+(float64(sj)+0.5)/float64(supersample))*spany/float64(im.H)
				// 每条扫描行只与边求一次交点，行内所有采样点复用
				xs := make([][]float64, len(polys))
				for k, p := range polys {
					n := len(p)
					for i := 0; i < n; i++ {
						if cx, ok := edgeCrossing(p[i], p[(i+1)%n], py, rule); ok {
							xs[k] = append(xs[k], cx)
						}
					}
				}
				for si := 0; si < supersample; si++ {
					px := minx + (float64(col)+(float64(si)+0.5)/float64(supersample))*spanx/float64(im.W)
					for k := range polys {
						inside := false
						for _, cx := range xs[k] {
							if cx > px {
								inside = !inside // 奇偶翻转
							}
						}
						if inside {
							hit++
							break
						}
					}
				}
			}
			im.Pix[row*im.W+col] = uint8(255 * hit / den)
		}
	}
	return nil
}
