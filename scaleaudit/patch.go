package scaleaudit

import (
	"math"

	"vecgo/geom"
	"vecgo/raster"
)

// PatchCheck 是面积律的受控验证：让一个直角三角形覆盖块以探针像素的一角为
// 锚点作二维相似伸缩，其截距为 a*s、b*s。只要整条斜边仍在该像素内，
//
//	覆盖面积 A(s)=1/2·(a s)(b s)=A(1)·s²
//	覆盖率   C(s)=A(s)/像素面积 ∝ s²
//
// 因而灰度应按面积比 s² 变化，而不是边长比 s。
type PatchCheck struct {
	Col, Row     int
	SS           int
	Points       []PatchPoint
	AreaMaxErr   float64 // 与实测灰度的最大绝对差（取整灰度）
	LinearMaxErr float64
	AreaRMSE     float64
	LinearRMSE   float64
}

type PatchPoint struct {
	Scale      float64
	Hits, Den  int
	Gray       int
	ExactCov   float64 // 解析覆盖率 0.5*a*b*s²/d²
	ExactGray  float64 // 255*ExactCov
	AreaRaw    float64 // 以 s=1 实测灰度为基准的 s² 预测（未取整）
	AreaGray   int
	LinearRaw  float64
	LinearGray int
}

const (
	patchPPU = 8.0
	patchSS  = 8
	patchCol = 3
	patchRow = 3
	patchA   = 0.10 // x 截距（世界单位，s=1）
	patchB   = 0.10 // y 截距
)

func buildPatchCheck() (*PatchCheck, error) {
	// 探针像素左上角 A=(col/PPU,row/PPU)，视口 Min=0。
	ax := float64(patchCol) / patchPPU
	ay := float64(patchRow) / patchPPU
	pixelSide := 1.0 / patchPPU
	pixelArea := pixelSide * pixelSide
	den := patchSS * patchSS

	scales := Scales() // 降序 1→0.5
	pts := make([]PatchPoint, len(scales))
	var anchorGray int
	for i, s := range scales {
		tri := []geom.Point{
			{X: ax, Y: ay},
			{X: ax + patchA*s, Y: ay},
			{X: ax, Y: ay + patchB*s},
		}
		im, err := raster.New(8, 8)
		if err != nil {
			return nil, err
		}
		v := raster.View{MinX: 0, MinY: 0, PPU: patchPPU}
		hits, d, err := im.CountView([][]geom.Point{tri}, patchSS, v)
		if err != nil {
			return nil, err
		}
		if err := im.FillView([][]geom.Point{tri}, patchSS, v); err != nil {
			return nil, err
		}
		_ = d
		h := hits[patchRow*8+patchCol]
		g := int(im.Pix[patchRow*8+patchCol])
		if math.Abs(s-1) < 1e-12 {
			anchorGray = g
		}
		exactCov := 0.5 * patchA * patchB * s * s / pixelArea
		pts[i] = PatchPoint{
			Scale: s, Hits: h, Den: den, Gray: g,
			ExactCov: exactCov, ExactGray: 255 * exactCov,
		}
	}

	pc := &PatchCheck{Col: patchCol, Row: patchRow, SS: patchSS, Points: pts}
	var seA, seL float64
	for i := range pts {
		p := &pts[i]
		p.AreaRaw = AreaGray(anchorGray, p.Scale, 1)
		p.AreaGray = QuantizePred(p.AreaRaw)
		p.LinearRaw = LinearGray(anchorGray, p.Scale, 1)
		p.LinearGray = QuantizePred(p.LinearRaw)
		eA := math.Abs(float64(p.Gray - p.AreaGray))
		eL := math.Abs(float64(p.Gray - p.LinearGray))
		if eA > pc.AreaMaxErr {
			pc.AreaMaxErr = eA
		}
		if eL > pc.LinearMaxErr {
			pc.LinearMaxErr = eL
		}
		seA += (float64(p.Gray-p.AreaGray) * float64(p.Gray-p.AreaGray))
		seL += (float64(p.Gray-p.LinearGray) * float64(p.Gray-p.LinearGray))
	}
	n := float64(len(pts))
	pc.AreaRMSE = math.Sqrt(seA / n)
	pc.LinearRMSE = math.Sqrt(seL / n)
	return pc, nil
}
