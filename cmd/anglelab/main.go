// Command anglelab compares self-overlapping stroke pixels with the
// angle-dependent truncation length at a two-segment corner.
//
// The corner has included angle theta.  geom.Stroke builds the two thick
// segment quads.  S(theta) is the number of integer pixel centers inside both
// quads, i.e. the double-covered/self-intersection region.  With half-width h,
// the geometric truncation length is T(theta)=h*cot(theta/2); multiplying by h
// gives the exact rhombus area A(theta)=h*T(theta)=w^2/4*cot(theta/2).
//
// The per-angle discrepancy is S-A; it is collapsed to one number as RMSE.
package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"

	"vecgo/geom"
)

const (
	angleStart = 20.0
	angleStop  = 160.0
	angleStep  = 5.0
	armLength  = 24.0
	strokeW    = 6.0
)

type angleRow struct {
	AngleDeg    float64 `json:"angle_deg"`
	SelfPixels  int     `json:"self_intersect_pixels"`
	TruncLength float64 `json:"truncation_length_px"`
	MiterLength float64 `json:"miter_length_px"`
	PredPixels  float64 `json:"predicted_pixels"`
	DiffPixels  float64 `json:"diff_pixels"`
	Crossings   int     `json:"ring_crossings"`
}

type result struct {
	Width        float64    `json:"stroke_width"`
	Angles       int        `json:"angle_count"`
	ScalarName   string     `json:"scalar_name"`
	ScalarValue  float64    `json:"scalar_value"`
	RMSE         float64    `json:"rmse_diff_pixels"`
	TotalAbsDiff float64    `json:"total_abs_diff_pixels"`
	MaxAbsDiff   float64    `json:"max_abs_diff_pixels"`
	Rows         []angleRow `json:"rows"`
}

func segmentQuad(a, b geom.Point) ([]geom.Point, error) {
	return geom.Stroke([]geom.Point{a, b}, strokeW)
}

// countCommonPixels counts pixel centers in both convex segment quads.
func countCommonPixels(q1, q2 []geom.Point) int {
	r := int(math.Ceil(armLength + strokeW + 2))
	n := 0
	for y := -r; y <= r; y++ {
		for x := -r; x <= r; x++ {
			px := float64(x) + 0.5
			py := float64(y) + 0.5
			if geom.Inside(q1, px, py) && geom.Inside(q2, px, py) {
				n++
			}
		}
	}
	return n
}

func countRingCrossings(poly []geom.Point) int {
	n := 0
	m := len(poly)
	for i := 0; i < m; i++ {
		a, b := poly[i], poly[(i+1)%m]
		for j := i + 1; j < m; j++ {
			if j == i || j == i+1 {
				continue
			}
			if (i == 0 && j == m-1) || j == (i+1)%m {
				continue
			}
			c, d := poly[j], poly[(j+1)%m]
			if segmentsCross(a, b, c, d) {
				n++
			}
		}
	}
	return n
}

func segmentsCross(a, b, c, d geom.Point) bool {
	const eps = 1e-10
	rx, ry := b.X-a.X, b.Y-a.Y
	sx, sy := d.X-c.X, d.Y-c.Y
	den := rx*sy - ry*sx
	if math.Abs(den) < eps {
		return false
	}
	qx, qy := c.X-a.X, c.Y-a.Y
	t := (qx*sy - qy*sx) / den
	u := (qx*ry - qy*rx) / den
	return t > eps && t < 1-eps && u > eps && u < 1-eps
}

func run(out string) error {
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	rows := []angleRow{}
	var sumSq, sumAbs, maxAbs float64
	for deg := angleStart; deg <= angleStop+1e-9; deg += angleStep {
		th := deg * math.Pi / 180
		a := geom.Point{X: -armLength, Y: 0}
		v := geom.Point{X: 0, Y: 0}
		b := geom.Point{X: -armLength * math.Cos(th), Y: armLength * math.Sin(th)}

		q1, err := segmentQuad(a, v)
		if err != nil {
			return err
		}
		q2, err := segmentQuad(v, b)
		if err != nil {
			return err
		}
		ring, err := geom.Stroke([]geom.Point{a, v, b}, strokeW)
		if err != nil {
			return err
		}
		s := countCommonPixels(q1, q2)
		h := strokeW / 2
		trunc := h / math.Tan(th/2)
		miter := h / math.Sin(th/2)
		pred := h * trunc
		diff := float64(s) - pred
		sumSq += diff * diff
		sumAbs += math.Abs(diff)
		maxAbs = math.Max(maxAbs, math.Abs(diff))
		rows = append(rows, angleRow{
			AngleDeg:    deg,
			SelfPixels:  s,
			TruncLength: trunc,
			MiterLength: miter,
			PredPixels:  pred,
			DiffPixels:  diff,
			Crossings:   countRingCrossings(ring),
		})
	}
	rmse := math.Sqrt(sumSq / float64(len(rows)))
	res := result{
		Width:        strokeW,
		Angles:       len(rows),
		ScalarName:   "RMSE(S_theta - h*T_theta)",
		ScalarValue:  rmse,
		RMSE:         rmse,
		TotalAbsDiff: sumAbs,
		MaxAbsDiff:   maxAbs,
		Rows:         rows,
	}

	jf, err := os.Create(filepath.Join(out, "angle_result.json"))
	if err != nil {
		return err
	}
	enc := json.NewEncoder(jf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(res); err != nil {
		jf.Close()
		return err
	}
	if err := jf.Close(); err != nil {
		return err
	}

	cf, err := os.Create(filepath.Join(out, "angle_metrics.csv"))
	if err != nil {
		return err
	}
	defer cf.Close()
	w := csv.NewWriter(cf)
	if err := w.Write([]string{"angle_deg", "self_intersect_pixels", "truncation_length_px", "miter_length_px", "predicted_pixels", "diff_pixels", "ring_crossings"}); err != nil {
		return err
	}
	for _, r := range rows {
		rec := []string{
			fmt.Sprintf("%.0f", r.AngleDeg),
			fmt.Sprintf("%d", r.SelfPixels),
			fmt.Sprintf("%.6f", r.TruncLength),
			fmt.Sprintf("%.6f", r.MiterLength),
			fmt.Sprintf("%.6f", r.PredPixels),
			fmt.Sprintf("%.6f", r.DiffPixels),
			fmt.Sprintf("%d", r.Crossings),
		}
		if err := w.Write(rec); err != nil {
			return err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return err
	}
	fmt.Printf("angles=%d rmse=%.6f total_abs=%.6f max_abs=%.6f\n", len(rows), rmse, sumAbs, maxAbs)
	return nil
}

func main() {
	out := flag.String("out", "artifacts", "artifact output directory")
	flag.Parse()
	if err := run(*out); err != nil {
		fmt.Fprintln(os.Stderr, "anglelab:", err)
		os.Exit(1)
	}
}
