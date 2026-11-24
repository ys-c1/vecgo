package coverage

import (
	"math"
	"math/rand"

	"vecgo/geom"
	"vecgo/raster"
)

// FormatSpec 是一档画幅。
type FormatSpec struct {
	Name string `json:"name"`
	W    int    `json:"w"`
	H    int    `json:"h"`
}

// ExperimentConfig 控制一档画幅上的方差实测。
type ExperimentConfig struct {
	Spec      FormatSpec
	Polys     [][]geom.Point
	N         int          // 每次覆盖率估计的样本数 n
	Reps      int          // 独立重复估计个数 K
	BatchSize int          // 计算“批次实测方差”的每组估计数 r
	Seed      int64        // 固定种子，保证报告数字可复现
	TrueP     float64      // 解析已知的真实覆盖率 p（用于理论预测）
	Alpha     float64      // 显著性水平，如 0.05 表示 95% 区间
	BinsX     int          // 空间均匀性分箱列数
	BinsY     int          // 空间均匀性分箱行数
	Trial     TrialSampler // nil 表示 iid 均匀
	TopK      int          // 空间诊断保留的热/冷格数
}

// Result 是一档画幅上的全部实测与预测对照数字。
type Result struct {
	Name      string `json:"name"`
	W         int    `json:"w"`
	H         int    `json:"h"`
	Sampler   string `json:"sampler"`
	N         int    `json:"n"`
	Reps      int    `json:"reps"`
	BatchSize int    `json:"batchSize"`
	Batches   int    `json:"batches"`
	Seed      int64  `json:"seed"`

	TrueP         float64 `json:"trueP"`
	PSampleMean   float64 `json:"pSampleMean"`
	PBias         float64 `json:"pBias"`
	VariancePred  float64 `json:"variancePred"`  // p(1-p)/n
	VarianceUpper float64 `json:"varianceUpper"` // 1/(4n)

	VarianceMeasured float64 `json:"varianceMeasured"` // K 个 p̂ 的无偏样本方差
	StdMeasured      float64 `json:"stdMeasured"`
	RatioToPred      float64 `json:"ratioToPred"`
	EffectiveN       float64 `json:"effectiveN"` // p̄(1-p̄)/S²

	// 全部 K 个估计的方差 S² 对应的 χ² 预测区间（df=K-1）。
	VarPILow  float64 `json:"varPiLow"`
	VarPIHigh float64 `json:"varPiHigh"`
	InVarPI   bool    `json:"inVarPi"`
	// 实测方差是否显著超出 χ² 95% 上界 —— 按题面规则触发“分布不均”判定。
	ExcessSignificant bool `json:"excessSignificant"`

	// 单个 p̂ 的正态预测区间，及 K 个实测估计值的落入覆盖率。
	EstPILow    float64 `json:"estPiLow"`
	EstPIHigh   float64 `json:"estPiHigh"`
	EstInside   int     `json:"estInside"`
	EstCoverage float64 `json:"estCoverage"`

	// 单个批次方差（r 个估计，df=r-1）的 χ² 预测区间，
	// 及 B 个批次实测方差对该区间的覆盖率。
	BatchPILow    float64 `json:"batchPiLow"`
	BatchPIHigh   float64 `json:"batchPiHigh"`
	BatchesInside int     `json:"batchesInside"`
	BatchCoverage float64 `json:"batchCoverage"`

	Bins *BinReport `json:"bins,omitempty"`
}

// Run 在一档画幅上完成全部重复估计、方差对照、区间覆盖率统计与空间诊断。
func Run(cfg ExperimentConfig) *Result {
	if cfg.N <= 0 || cfg.Reps < 2 || cfg.BatchSize < 2 {
		panic("coverage.Run: N 必须为正，Reps/BatchSize 至少为 2")
	}
	if cfg.Alpha == 0 {
		cfg.Alpha = 0.05
	}
	trial := cfg.Trial
	if trial == nil {
		trial = &Uniform{}
	}
	rng := rand.New(rand.NewSource(cfg.Seed))
	bins := NewBinAccumulator(cfg.BinsX, cfg.BinsY)
	m := raster.NewMapping(cfg.Polys)
	wf, hf := float64(cfg.Spec.W), float64(cfg.Spec.H)

	pred := Variance(cfg.TrueP, cfg.N)
	upper := WorstVariance(cfg.N)
	sigma := math.Sqrt(pred)
	estLo, estHi := EstimatePredictionInterval(cfg.TrueP, sigma, cfg.Alpha)
	varLo, varHi := VariancePredictionInterval(pred, cfg.Reps, cfg.Alpha)
	batchLo, batchHi := VariancePredictionInterval(pred, cfg.BatchSize, cfg.Alpha)

	res := &Result{
		Name: cfg.Spec.Name, W: cfg.Spec.W, H: cfg.Spec.H,
		Sampler: trial.Name(), N: cfg.N, Reps: cfg.Reps,
		BatchSize: cfg.BatchSize, Seed: cfg.Seed, TrueP: cfg.TrueP,
		VariancePred: pred, VarianceUpper: upper,
		VarPILow: varLo, VarPIHigh: varHi,
		EstPILow: estLo, EstPIHigh: estHi,
		BatchPILow: batchLo, BatchPIHigh: batchHi,
	}

	var all Welford
	var batch Welford
	batches := 0
	batchesInside := 0

	for rep := 0; rep < cfg.Reps; rep++ {
		trial.Begin(rng, wf, hf)
		hits := 0
		for k := 0; k < cfg.N; k++ {
			x, y := trial.Point(rng)
			bins.Add(x, y, wf, hf)
			px, py := m.PixelToWorld(cfg.Spec.W, cfg.Spec.H, x, y)
			for _, p := range cfg.Polys {
				if geom.Inside(p, px, py) {
					hits++
					break
				}
			}
		}
		hat := float64(hits) / float64(cfg.N)
		all.Add(hat)
		batch.Add(hat)
		if hat >= estLo && hat <= estHi {
			res.EstInside++
		}
		if batch.N == cfg.BatchSize {
			s2 := batch.Variance()
			batches++
			if s2 >= batchLo && s2 <= batchHi {
				batchesInside++
			}
			batch = Welford{}
		}
	}

	res.Batches = batches
	res.PSampleMean = all.Mean()
	res.PBias = all.Mean() - cfg.TrueP
	res.VarianceMeasured = all.Variance()
	res.StdMeasured = math.Sqrt(res.VarianceMeasured)
	res.RatioToPred = res.VarianceMeasured / pred
	res.EffectiveN = res.PSampleMean * (1 - res.PSampleMean) / res.VarianceMeasured
	res.InVarPI = res.VarianceMeasured >= varLo && res.VarianceMeasured <= varHi
	res.ExcessSignificant = res.VarianceMeasured > varHi
	res.EstCoverage = float64(res.EstInside) / float64(cfg.Reps)
	if batches > 0 {
		res.BatchesInside = batchesInside
		res.BatchCoverage = float64(batchesInside) / float64(batches)
	}
	res.Bins = bins.Report(cfg.TopK)
	return res
}
