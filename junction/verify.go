package junction

import "math"

// Options 控制一次接点核验所用的尺度参数。
type Options struct {
	Radius     float64 // 扇形半径（像素），默认 1
	RingPixels int     // 环上离散像素数，默认 720
	Ink        float64 // 周围参考灰度 0..255，默认 InkBrightness=200
}

// DefaultOptions 返回报表使用的缺省尺度。
func DefaultOptions() Options {
	return Options{Radius: 1, RingPixels: 720, Ink: InkBrightness}
}

func (o *Options) fillDefaults() {
	d := DefaultOptions()
	if o.Radius <= 0 || math.IsNaN(o.Radius) {
		o.Radius = d.Radius
	}
	if o.RingPixels < 3 {
		o.RingPixels = d.RingPixels
	}
	if o.Ink <= 0 || o.Ink > 255 {
		o.Ink = d.Ink
	}
}

// Result 是“一个接点、单独核对”的完整记录：
// 扇形面积、反解权重、权重和偏差、除以三反例、逐像素亮度核对、
// 以及独立裁剪路径的面积守恒残差。
type Result struct {
	Junction Junction
	Radius   float64
	Areas    Areas
	Weights  Weights

	WeightSum      float64
	WeightSumError float64
	WeightSumHolds bool
	AngleSumError  float64
	AreaSumError   float64 // |ΣA - πr²|

	Thirds ThirdsGap
	Ring   RingAudit

	// 接点中心像素上，用 geom.Clip 独立算出的三扇区面积与守恒残差 |Σa-1|。
	CenterPixelAreas      Areas
	CenterConservationErr float64
}

// Audit 对单个接点执行全部核对。权重和为 1 在这一个接点上单独判定，
// 不与其他接点平均。
func Audit(j Junction, opts Options) (Result, error) {
	opts.fillDefaults()
	if err := ValidateAngles(j.Angles); err != nil {
		return Result{}, err
	}
	ar, err := SectorAreas(j.Angles, opts.Radius)
	if err != nil {
		return Result{}, err
	}
	w, err := ConservationWeights(j.Angles)
	if err != nil {
		return Result{}, err
	}
	sumErr, err := CheckWeightSum(w)
	if err != nil {
		return Result{}, err
	}
	gap, err := EqualThirdsGap(j, opts.Radius, 255) // 反例亮度统一用满墨 255 口径
	if err != nil {
		return Result{}, err
	}
	ring, err := AuditRing(j, opts.RingPixels, opts.Ink)
	if err != nil {
		return Result{}, err
	}
	clipAreas, consErr, err := ConservationAtPixel(j, 0, 0)
	if err != nil {
		return Result{}, err
	}

	res := Result{
		Junction:              j,
		Radius:                opts.Radius,
		Areas:                 ar,
		Weights:               w,
		WeightSum:             SumWeights(w),
		WeightSumError:        sumErr,
		WeightSumHolds:        sumErr <= WeightSumTol,
		AngleSumError:         math.Abs(SumAngles(j.Angles) - Tau),
		AreaSumError:          math.Abs(SumAreas(ar) - TotalArea(opts.Radius)),
		Thirds:                gap,
		Ring:                  ring,
		CenterPixelAreas:      clipAreas,
		CenterConservationErr: consErr,
	}
	return res, nil
}

// AuditAll 逐个核对给定接点，返回等长结果；每个 Result 的 WeightSumHolds
// 必须独立为 true。
func AuditAll(js []Junction, opts Options) ([]Result, error) {
	out := make([]Result, 0, len(js))
	for _, j := range js {
		r, err := Audit(j, opts)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}
