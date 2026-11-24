package junction

import (
	"fmt"
	"math"

	"vecgo/geom"
)

// SectorPolygon 构造第 i 个扇形的多边形：圆心 O，两条边界射线，
// 以及半径 radius 圆弧上的 arcSteps 个逼近点。θ>π 的优角扇形同样成立。
// 坐标系与 vecgo/geom 一致（y 轴方向由调用方决定，角度按标准数学逆时针）。
func SectorPolygon(j Junction, i int, center geom.Point, radius float64, arcSteps int) ([]geom.Point, error) {
	if i < 0 || i > 2 {
		return nil, fmt.Errorf("扇区下标越界: %d", i)
	}
	if radius <= 0 {
		return nil, fmt.Errorf("半径必须为正: %g", radius)
	}
	if arcSteps < 2 {
		arcSteps = 2
	}
	// 由三个夹角恢复第 i 个扇区的起止方向：以 0 号边为起点，周向累加。
	start := 0.0
	switch i {
	case 1:
		start = j.Angles[0]
	case 2:
		start = j.Angles[0] + j.Angles[1]
	}
	span := j.Angles[i]
	pts := make([]geom.Point, 0, arcSteps+2)
	pts = append(pts, center)
	for k := 0; k <= arcSteps; k++ {
		a := start + span*float64(k)/float64(arcSteps)
		pts = append(pts, geom.Point{
			X: center.X + radius*math.Cos(a),
			Y: center.Y + radius*math.Sin(a),
		})
	}
	return pts, nil
}

// PixelSquare 返回中心在 (cx,cy)、边长 1 的像素正方形（逆时针）。
func PixelSquare(cx, cy float64) []geom.Point {
	return []geom.Point{
		{X: cx - 0.5, Y: cy - 0.5},
		{X: cx + 0.5, Y: cy - 0.5},
		{X: cx + 0.5, Y: cy + 0.5},
		{X: cx - 0.5, Y: cy + 0.5},
	}
}

// PixelSectorAreas 用 Sutherland–Hodgman 把三个扇形分别与指定像素正方形求交，
// 返回每个扇形在该像素内的真实面积。这是独立于 A=½r²θ 公式的第二条计算路径，
// 用来核验“接点像素覆盖率可以写成三个扇形面积”。
func PixelSectorAreas(j Junction, cx, cy, sectorRadius float64) (Areas, error) {
	sq := PixelSquare(cx, cy)
	var out Areas
	for i := 0; i < 3; i++ {
		// 圆弧取足够密，把弦逼近误差压到 1e-6 以下。
		sec, err := SectorPolygon(j, i, geom.Point{X: 0, Y: 0}, sectorRadius, 96)
		if err != nil {
			return Areas{}, err
		}
		clipped, err := geom.Clip(sec, sq)
		if err != nil {
			return Areas{}, err
		}
		out[i] = geom.Area(clipped)
	}
	return out, nil
}

// ConservationAtPixel 核对一个像素上的面积守恒：
// 三个扇形在该像素内的面积之和必须等于像素面积 1
// （三个扇形恰好把平面切分，含接点所在像素）。
// 返回三扇区面积、总和与 |sum-1|。
func ConservationAtPixel(j Junction, cx, cy float64) (Areas, float64, error) {
	a, err := PixelSectorAreas(j, cx, cy, 64)
	if err != nil {
		return Areas{}, 0, err
	}
	s := SumAreas(a)
	return a, math.Abs(s - 1), nil
}
