// Package cliparea 提供三档分块裁剪的面积误差实验。
//
// 设计要点：浮点路径直接调用 geom.Clip / geom.Shoelace，与生产代码同一套实现；
// 另有一条逐行镜像其代数过程的精确有理路径（math/big.Rat），用来证明
// “在精确算术下裁剪分块面积之和与原面积严格相等”，从而把浮点面积差唯一地
// 限定到 float64 的取整（舍入）误差上。
package cliparea

import (
	"math/big"

	"vecgo/geom"
)

// rPoint 是精确有理点。
type rPoint struct{ X, Y *big.Rat }

func (p rPoint) sub(q rPoint) rPoint {
	return rPoint{X: new(big.Rat).Sub(p.X, q.X), Y: new(big.Rat).Sub(p.Y, q.Y)}
}

// rIntersect 与 geom.intersect 逐行对应，只是把 float64 换成 *big.Rat。
// 入参 p=prv、q=cur，输出边 p→q 与裁剪线 a→b 的交点。
func rIntersect(p, q, a, b rPoint) rPoint {
	pxqx := new(big.Rat).Sub(p.X, q.X)
	ayby := new(big.Rat).Sub(a.Y, b.Y)
	pyqy := new(big.Rat).Sub(p.Y, q.Y)
	axbx := new(big.Rat).Sub(a.X, b.X)
	den := new(big.Rat).Sub(new(big.Rat).Mul(pxqx, ayby), new(big.Rat).Mul(pyqy, axbx))

	pxax := new(big.Rat).Sub(p.X, a.X)
	pyay := new(big.Rat).Sub(p.Y, a.Y)
	num := new(big.Rat).Sub(new(big.Rat).Mul(pxax, ayby), new(big.Rat).Mul(pyay, axbx))
	t := new(big.Rat).Quo(num, den)

	qxpx := new(big.Rat).Sub(q.X, p.X)
	qypy := new(big.Rat).Sub(q.Y, p.Y)
	return rPoint{
		X: new(big.Rat).Add(p.X, new(big.Rat).Mul(qxpx, t)),
		Y: new(big.Rat).Add(p.Y, new(big.Rat).Mul(qypy, t)),
	}
}

// rClip 是 geom.Clip 的精确有理镜像（Sutherland–Hodgman）。
func rClip(poly, window []rPoint) []rPoint {
	out := append([]rPoint{}, poly...)
	for i := range window {
		a := window[i]
		b := window[(i+1)%len(window)]
		e := b.sub(a)
		inside := func(p rPoint) bool {
			r := p.sub(a)
			// e.x*(p.y-a.y) - e.y*(p.x-a.x) >= 0
			c := new(big.Rat).Sub(new(big.Rat).Mul(e.X, r.Y), new(big.Rat).Mul(e.Y, r.X))
			return c.Sign() >= 0
		}
		nxt := []rPoint{}
		for j := range out {
			cur := out[j]
			prv := out[(j-1+len(out))%len(out)]
			ci, pi := inside(cur), inside(prv)
			if ci && !pi {
				nxt = append(nxt, rIntersect(prv, cur, a, b))
			}
			if ci {
				nxt = append(nxt, cur)
			} else if pi {
				nxt = append(nxt, rIntersect(prv, cur, a, b))
			}
		}
		out = nxt
		if len(out) == 0 {
			break
		}
	}
	return out
}

// rShoelace 是 geom.Shoelace 的精确有理版本。
func rShoelace(pts []rPoint) *big.Rat {
	twice := big.NewRat(0, 1)
	n := len(pts)
	for i := 0; i < n; i++ {
		p, q := pts[i], pts[(i+1)%n]
		twice.Add(twice, new(big.Rat).Sub(new(big.Rat).Mul(p.X, q.Y), new(big.Rat).Mul(p.Y, q.X)))
	}
	return new(big.Rat).Quo(new(big.Rat).Abs(twice), big.NewRat(2, 1))
}

// toRat 把 float64 输入点无损转成有理点：binary64 本身就是有理数
// （尾数×2^指数），(*big.Rat).SetFloat64 精确还原其真实有理值，
// 不做任何十进制截断。
func toRat(pts []geom.Point) []rPoint {
	out := make([]rPoint, len(pts))
	for i, p := range pts {
		out[i] = rPoint{X: new(big.Rat).SetFloat64(p.X), Y: new(big.Rat).SetFloat64(p.Y)}
	}
	return out
}

// RatWindow 从 float64 网格窗口构造有理窗口；D/k 对整数 D、k 是有限有理值，
// 用 SetString 精确读回（形如 "5"、"2.5"），不经过任何 float64 舍入。
func RatWindow(w []geom.Point) []rPoint {
	out := make([]rPoint, len(w))
	for i, p := range w {
		x := new(big.Rat)
		if _, ok := x.SetString(ratLiteral(p.X)); !ok {
			panic("无法精确表示窗口横坐标")
		}
		y := new(big.Rat)
		if _, ok := y.SetString(ratLiteral(p.Y)); !ok {
			panic("无法精确表示窗口纵坐标")
		}
		out[i] = rPoint{X: x, Y: y}
	}
	return out
}

// ratLiteral 把本实验使用的“整数除以 2 的幂”型坐标转成精确十进制字符串。
func ratLiteral(v float64) string {
	return big.NewFloat(v).Text('f', -1)
}

// ExactArea 返回精确算术下原多边形的面积。
func ExactArea(poly []geom.Point) *big.Rat {
	return rShoelace(rClip(toRat(poly), RatWindow(domainWindow())))
}

// ExactPieceSum 返回精确算术下某一档 k×k 分块裁剪后各块面积之和。
func ExactPieceSum(poly []geom.Point, k int, domain float64) *big.Rat {
	rp := toRat(poly)
	sum := big.NewRat(0, 1)
	for _, w := range GridWindows(k, domain) {
		piece := rClip(rp, RatWindow(w))
		if len(piece) >= 3 {
			sum.Add(sum, rShoelace(piece))
		}
	}
	return sum
}

func domainWindow() []geom.Point {
	return GridWindows(1, 10)[0]
}
