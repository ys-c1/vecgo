package strip

import "math"

// ConnectionCounts is the exact full-grid accounting for one topology.
type ConnectionCounts struct {
	E                      int     `json:"endpoint_count"`
	LowerBound             int     `json:"lower_bound"`
	CellsPerStrip          int     `json:"cells_per_strip"`
	RealTriangles          int     `json:"real_triangles"`
	DirectVertices         int     `json:"direct_vertices"`
	DirectTriangles        int     `json:"direct_triangles"`
	DirectDegenerate       int     `json:"direct_degenerate"`
	DirectExtraPositive    int     `json:"direct_extra_positive"`
	DirectValid            bool    `json:"direct_valid"`
	OptimalVertices        int     `json:"optimal_vertices"`
	OptimalTriangles       int     `json:"optimal_triangles"`
	OptimalDegenerate      int     `json:"optimal_degenerate"`
	OptimalExtraPositive   int     `json:"optimal_extra_positive"`
	OptimalMeetsLowerBound bool    `json:"optimal_meets_lower_bound"`
	NaiveVertices          int     `json:"naive_vertices"`
	NaiveTriangles         int     `json:"naive_triangles"`
	NaiveDegenerate        int     `json:"naive_degenerate"`
	NaiveExtraPositive     int     `json:"naive_extra_positive"`
	NaiveAboveBound        bool    `json:"naive_above_bound"`
	UniqueVertices         int     `json:"unique_vertices"`
	CornerBeforeOptimal    int     `json:"corner_vertices_before_optimal"`
	CornerAfter            int     `json:"corner_vertices_after"`
	CornerReductionPct     float64 `json:"corner_reduction_pct"`
}

// TopologyAudit bundles exact counts and exhaustive raster evidence.
type TopologyAudit struct {
	Topology Topology
	Counts   ConnectionCounts
	Before   []Triangle
	After    []Triangle
	Naive    []Triangle
}

func uniquePointCount(groups ...[]Point) int {
	seen := make(map[[2]float64]struct{})
	for _, g := range groups {
		for _, p := range g {
			seen[[2]float64{p.X, p.Y}] = struct{}{}
		}
	}
	return len(seen)
}

// Audit computes exact counts without any sampling.
func Audit(t Topology) TopologyAudit {
	realA := FromStrip(t.A)
	realB := FromStrip(t.B)
	realN := len(realA) + len(realB)

	directTris := FromStrip(t.Direct)
	optimalTris := FromStrip(t.Optimal)
	naiveTris := FromStrip(t.Naive)

	directDeg := CountDegenerate(directTris)
	optimalDeg := CountDegenerate(optimalTris)
	naiveDeg := CountDegenerate(naiveTris)

	directPositive := len(directTris) - directDeg
	optimalPositive := len(optimalTris) - optimalDeg
	naivePositive := len(naiveTris) - naiveDeg

	lb := t.E - 1
	after := FilterDegenerate(optimalTris)
	cornerBefore := 3 * len(optimalTris)
	cornerAfter := 3 * len(after)
	directExtraPositive := directPositive - realN

	c := ConnectionCounts{
		E:                      t.E,
		LowerBound:             lb,
		CellsPerStrip:          t.Cells,
		RealTriangles:          realN,
		DirectVertices:         len(t.Direct),
		DirectTriangles:        len(directTris),
		DirectDegenerate:       directDeg,
		DirectExtraPositive:    directExtraPositive,
		DirectValid:            directExtraPositive == 0,
		OptimalVertices:        len(t.Optimal),
		OptimalTriangles:       len(optimalTris),
		OptimalDegenerate:      optimalDeg,
		OptimalExtraPositive:   optimalPositive - realN,
		OptimalMeetsLowerBound: optimalDeg == lb && optimalPositive == realN,
		NaiveVertices:          len(t.Naive),
		NaiveTriangles:         len(naiveTris),
		NaiveDegenerate:        naiveDeg,
		NaiveExtraPositive:     naivePositive - realN,
		NaiveAboveBound:        naiveDeg > lb && naivePositive == realN,
		UniqueVertices:         uniquePointCount(t.A, t.B),
		CornerBeforeOptimal:    cornerBefore,
		CornerAfter:            cornerAfter,
		CornerReductionPct:     100 * float64(cornerBefore-cornerAfter) / float64(cornerBefore),
	}

	return TopologyAudit{
		Topology: t,
		Counts:   c,
		Before:   optimalTris,
		After:    after,
		Naive:    naiveTris,
	}
}

// Aggregate is the grid-wide total across all three topologies.
type Aggregate struct {
	Topologies          int     `json:"topologies"`
	E                   int     `json:"endpoint_occurrences"`
	LowerBound          int     `json:"lower_bound_total"`
	RealTriangles       int     `json:"real_triangles"`
	OptimalDegenerate   int     `json:"optimal_degenerate"`
	NaiveDegenerate     int     `json:"naive_degenerate"`
	CornerBeforeOptimal int     `json:"corner_before_optimal"`
	CornerBeforeNaive   int     `json:"corner_before_naive"`
	CornerAfter         int     `json:"corner_after"`
	CornerReductionPct  float64 `json:"corner_reduction_pct"`
	NaiveReductionPct   float64 `json:"naive_corner_reduction_pct"`
	StreamBeforeOptimal int     `json:"strip_stream_slots_before"`
	StreamIdealAfter    int     `json:"strip_stream_slots_after_charge"`
	StreamReductionPct  float64 `json:"strip_stream_reduction_pct"`
}

// Summarize aggregates exact counts.
func Summarize(audits []TopologyAudit) Aggregate {
	var s Aggregate
	s.Topologies = len(audits)
	for _, a := range audits {
		c := a.Counts
		s.E += c.E
		s.LowerBound += c.LowerBound
		s.RealTriangles += c.RealTriangles
		s.OptimalDegenerate += c.OptimalDegenerate
		s.NaiveDegenerate += c.NaiveDegenerate
		s.CornerBeforeOptimal += c.CornerBeforeOptimal
		s.CornerAfter += c.CornerAfter
		s.StreamBeforeOptimal += c.OptimalVertices
		s.StreamIdealAfter += c.RealTriangles + 2
	}
	s.CornerReductionPct = 100 * float64(s.CornerBeforeOptimal-s.CornerAfter) / float64(s.CornerBeforeOptimal)
	s.CornerBeforeNaive = 3 * (s.RealTriangles + s.NaiveDegenerate)
	s.NaiveReductionPct = 100 * float64(s.CornerBeforeNaive-s.CornerAfter) / float64(s.CornerBeforeNaive)
	s.StreamReductionPct = 100 * float64(s.StreamBeforeOptimal-s.StreamIdealAfter) / float64(s.StreamBeforeOptimal)
	return s
}

// NearlyEqualFloat is a small helper for tests/report validation.
func NearlyEqualFloat(a, b float64) bool { return math.Abs(a-b) < 1e-12 }
