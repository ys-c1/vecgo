package main

import (
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strconv"

	"vecgo/compose"
)

type summary struct {
	Width                      int     `json:"width"`
	Height                     int     `json:"height"`
	Pixels                     int     `json:"pixels"`
	FractionBits               int     `json:"fraction_bits"`
	FixedPointScale            uint64  `json:"fixed_point_scale"`
	FinalBitsPerChannel        int     `json:"final_bits_per_channel"`
	StableAllPixelEqual        bool    `json:"stable_all_pixel_equal"`
	StableMaxU8Diff            int     `json:"stable_max_u8_diff"`
	NaiveMaxU8OrderDiff        int     `json:"naive_max_u8_order_diff"`
	MaxOrderErrorRealCode      float64 `json:"max_order_error_real_code"`
	WorstPixelRatioColorBound  float64 `json:"worst_pixel_ratio_color_bound"`
	WorstPixelRatioCoarseBound float64 `json:"worst_pixel_ratio_coarse_bound"`
	MaxRatioColorBound         float64 `json:"max_ratio_color_bound"`
	MaxRatioPixel              [2]int  `json:"max_ratio_pixel"`
	WorstPixel                 [2]int  `json:"worst_pixel"`
	MaxQ16RoundErrorCode       float64 `json:"max_q16_round_error_code"`
	MaxEndToEndErrorCode       float64 `json:"max_end_to_end_error_code"`
	PremultUnsortedEqual       bool    `json:"premultiply_unsorted_all_equal"`
	PremultUnsortedMaxDiff     int     `json:"premultiply_unsorted_max_u8_diff"`
	StraightSortedEqual        bool    `json:"straight_sorted_all_equal"`
	StraightSortedMaxDiff      int     `json:"straight_sorted_max_u8_diff"`
	StraightUnsortedEqual      bool    `json:"straight_unsorted_all_equal"`
	StraightUnsortedMaxDiff    int     `json:"straight_unsorted_max_u8_diff"`
}

func main() {
	outDir := flag.String("out", "artifacts/composition", "artifact output directory")
	flag.Parse()

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fatal(err)
	}

	pixels := compose.DemoPixelFragments()
	n := compose.DemoWidth * compose.DemoHeight
	bg := compose.DemoBackground

	naiveImages := make([][][3]uint8, 3)
	stableImages := make([][][3]uint8, 3)
	for i := range naiveImages {
		naiveImages[i] = make([][3]uint8, n)
		stableImages[i] = make([][3]uint8, n)
	}

	csvFile, err := os.Create(filepath.Join(*outDir, "pixel-errors.csv"))
	if err != nil {
		fatal(err)
	}
	w := csv.NewWriter(csvFile)
	header := []string{
		"x", "y", "a_R", "a_G", "a_B",
		"bound_color_code", "bound_coarse_code",
		"order_err_real_code", "ratio_color_bound", "ratio_coarse_bound",
		"order_err_u8", "ratio_u8_over_bound_plus_1",
		"stable_R", "stable_G", "stable_B",
		"q16_round_err_R_code", "q16_round_err_G_code", "q16_round_err_B_code",
		"q16_round_max_err_code", "end_to_end_max_err_code",
	}
	if err := w.Write(header); err != nil {
		fatal(err)
	}

	sum := summary{
		Width:               compose.DemoWidth,
		Height:              compose.DemoHeight,
		Pixels:              n,
		FractionBits:        compose.FractionBits,
		FixedPointScale:     compose.Scale,
		FinalBitsPerChannel: 8,
		WorstPixel:          [2]int{-1, -1},
		MaxRatioPixel:       [2]int{-1, -1},
	}
	stableEqual := true
	stableMaxDiff := 0
	premultUnsortedEqual := true
	premultUnsortedMax := 0
	straightSortedEqual := true
	straightSortedMax := 0
	straightUnsortedEqual := true
	straightUnsortedMax := 0

	for idx, fs := range pixels {
		x := idx % compose.DemoWidth
		y := idx / compose.DemoWidth

		bound := compose.OrderBound(fs)
		coarse := coarseBound(fs)

		var naiveF [3][3]float64
		var naiveU [3][3]uint8
		var stableU [3][3]uint8
		var premultUnsorted [3][3]uint8
		var straightSorted [3][3]uint8
		var straightUnsorted [3][3]uint8

		for g := 0; g < 3; g++ {
			ordered := compose.Reorder(fs, compose.DemoPermutations[g])
			naiveF[g] = compose.StraightFloat(bg, ordered)
			naiveU[g] = roundU8(naiveF[g])
			naiveImages[g][idx] = naiveU[g]

			stableU[g] = compose.CompositeQ(bg, fs, true, true)
			stableImages[g][idx] = stableU[g]

			premultUnsorted[g] = compose.CompositeQ(bg, ordered, false, true)
			straightSorted[g] = compose.CompositeQ(bg, fs, true, false)
			straightUnsorted[g] = compose.CompositeQ(bg, ordered, false, false)
		}

		orderErrReal := 0.0
		orderErrU8 := 0
		for i := 0; i < 3; i++ {
			for j := i + 1; j < 3; j++ {
				orderErrReal = maxFloat(orderErrReal, compose.MaxChannelError(naiveF[i], naiveF[j]))
				orderErrU8 = maxInt(orderErrU8, compose.MaxU8ChannelDiff(naiveU[i], naiveU[j]))

				stableMaxDiff = maxInt(stableMaxDiff, compose.MaxU8ChannelDiff(stableU[i], stableU[j]))
				premultUnsortedMax = maxInt(premultUnsortedMax, compose.MaxU8ChannelDiff(premultUnsorted[i], premultUnsorted[j]))
				straightSortedMax = maxInt(straightSortedMax, compose.MaxU8ChannelDiff(straightSorted[i], straightSorted[j]))
				straightUnsortedMax = maxInt(straightUnsortedMax, compose.MaxU8ChannelDiff(straightUnsorted[i], straightUnsorted[j]))
			}
		}
		if stableMaxDiff != 0 {
			stableEqual = false
		}
		if premultUnsortedMax != 0 {
			premultUnsortedEqual = false
		}
		if straightSortedMax != 0 {
			straightSortedEqual = false
		}
		if straightUnsortedMax != 0 {
			straightUnsortedEqual = false
		}

		ratioColor := 0.0
		if bound > 1e-12 {
			ratioColor = orderErrReal / bound
		}
		ratioCoarse := 0.0
		if coarse > 1e-12 {
			ratioCoarse = orderErrReal / coarse
		}
		if orderErrReal > sum.MaxOrderErrorRealCode {
			sum.MaxOrderErrorRealCode = orderErrReal
			sum.WorstPixelRatioColorBound = ratioColor
			sum.WorstPixelRatioCoarseBound = ratioCoarse
			sum.WorstPixel = [2]int{x, y}
		}
		if ratioColor > sum.MaxRatioColorBound {
			sum.MaxRatioColorBound = ratioColor
			sum.MaxRatioPixel = [2]int{x, y}
		}
		sum.NaiveMaxU8OrderDiff = maxInt(sum.NaiveMaxU8OrderDiff, orderErrU8)

		qValues := compose.CompositeQValues(bg, fs, true, true)
		qRef := compose.CompositeFloatQInputs(bg, fs, true, true)
		var qErrs [3]float64
		qMax := 0.0
		for c := 0; c < 3; c++ {
			qGot := float64(qValues[c]) / float64(compose.Scale) * 255
			qErrs[c] = math.Abs(qGot - qRef[c])
			qMax = maxFloat(qMax, qErrs[c])
		}
		sum.MaxQ16RoundErrorCode = maxFloat(sum.MaxQ16RoundErrorCode, qMax)

		ideal := compose.StraightFloat(bg, compose.Reorder(fs, []int{0, 1, 2}))
		stableAsFloat := [3]float64{
			float64(stableU[0][0]), float64(stableU[0][1]), float64(stableU[0][2]),
		}
		endToEnd := compose.MaxChannelError(stableAsFloat, ideal)
		sum.MaxEndToEndErrorCode = maxFloat(sum.MaxEndToEndErrorCode, endToEnd)

		row := []string{
			strconv.Itoa(x), strconv.Itoa(y),
			alphaString(fs[0].A), alphaString(fs[1].A), alphaString(fs[2].A),
			fmt.Sprintf("%.9f", bound),
			fmt.Sprintf("%.9f", coarse),
			fmt.Sprintf("%.9f", orderErrReal),
			fmt.Sprintf("%.9f", ratioColor),
			fmt.Sprintf("%.9f", ratioCoarse),
			strconv.Itoa(orderErrU8),
			fmt.Sprintf("%.9f", float64(orderErrU8)/(bound+1)),
			strconv.Itoa(int(stableU[0][0])),
			strconv.Itoa(int(stableU[0][1])),
			strconv.Itoa(int(stableU[0][2])),
			fmt.Sprintf("%.9f", qErrs[0]),
			fmt.Sprintf("%.9f", qErrs[1]),
			fmt.Sprintf("%.9f", qErrs[2]),
			fmt.Sprintf("%.9f", qMax),
			fmt.Sprintf("%.9f", endToEnd),
		}
		if err := w.Write(row); err != nil {
			fatal(err)
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		fatal(err)
	}
	if err := csvFile.Close(); err != nil {
		fatal(err)
	}

	names := make([]string, 3)
	stableNames := make([]string, 3)
	naiveNames := make([]string, 3)
	for g := 0; g < 3; g++ {
		names[g] = compose.PermutationName(compose.DemoPermutations[g])
		naiveNames[g] = fmt.Sprintf("naive-order-%d-%s.png", g+1, names[g])
		stableNames[g] = fmt.Sprintf("stable-order-%d-%s.png", g+1, names[g])
		if err := saveScaledPNG(filepath.Join(*outDir, naiveNames[g]), naiveImages[g]); err != nil {
			fatal(err)
		}
		if err := saveScaledPNG(filepath.Join(*outDir, stableNames[g]), stableImages[g]); err != nil {
			fatal(err)
		}
	}

	manifest, err := os.Create(filepath.Join(*outDir, "HASHES.txt"))
	if err != nil {
		fatal(err)
	}
	// Keep the three order-independent image hashes together at the top.
	for _, name := range stableNames {
		sum, err := sha256File(filepath.Join(*outDir, name))
		if err != nil {
			fatal(err)
		}
		if _, err := fmt.Fprintf(manifest, "%s  %s\n", sum, name); err != nil {
			fatal(err)
		}
	}
	for _, name := range naiveNames {
		sum, err := sha256File(filepath.Join(*outDir, name))
		if err != nil {
			fatal(err)
		}
		if _, err := fmt.Fprintf(manifest, "%s  %s\n", sum, name); err != nil {
			fatal(err)
		}
	}
	if err := manifest.Close(); err != nil {
		fatal(err)
	}

	sum.StableAllPixelEqual = stableEqual
	sum.StableMaxU8Diff = stableMaxDiff
	sum.PremultUnsortedEqual = premultUnsortedEqual
	sum.PremultUnsortedMaxDiff = premultUnsortedMax
	sum.StraightSortedEqual = straightSortedEqual
	sum.StraightSortedMaxDiff = straightSortedMax
	sum.StraightUnsortedEqual = straightUnsortedEqual
	sum.StraightUnsortedMaxDiff = straightUnsortedMax

	jsonBytes, err := json.MarshalIndent(sum, "", "  ")
	if err != nil {
		fatal(err)
	}
	if err := os.WriteFile(filepath.Join(*outDir, "summary.json"), append(jsonBytes, '\n'), 0o644); err != nil {
		fatal(err)
	}

	fmt.Printf("wrote %s: %d pixels; stable_equal=%v max_diff=%d; naive_max_u8=%d\n",
		*outDir, n, stableEqual, stableMaxDiff, sum.NaiveMaxU8OrderDiff)
	fmt.Printf("max real order error=%.6f code; worst-pixel color-bound ratio=%.6f; coarse ratio=%.6f at %v\n",
		sum.MaxOrderErrorRealCode, sum.WorstPixelRatioColorBound, sum.WorstPixelRatioCoarseBound, sum.WorstPixel)
	fmt.Printf("supremum error/bound ratio=%.6f at %v\n", sum.MaxRatioColorBound, sum.MaxRatioPixel)
	fmt.Printf("Q16 max rounding error=%.9f code; end-to-end max=%.6f code\n",
		sum.MaxQ16RoundErrorCode, sum.MaxEndToEndErrorCode)
}

func coarseBound(fs []compose.Fragment) float64 {
	b := 0.0
	for i := 0; i < len(fs); i++ {
		ai := float64(fs[i].A) / float64(compose.Scale)
		for j := i + 1; j < len(fs); j++ {
			aj := float64(fs[j].A) / float64(compose.Scale)
			b += 255 * ai * aj
		}
	}
	return b
}

func alphaString(a uint64) string {
	return strconv.FormatFloat(float64(a)/float64(compose.Scale), 'f', 6, 64)
}

func roundU8(v [3]float64) [3]uint8 {
	var out [3]uint8
	for c := 0; c < 3; c++ {
		r := math.Round(v[c])
		if r < 0 {
			r = 0
		}
		if r > 255 {
			r = 255
		}
		out[c] = uint8(r)
	}
	return out
}

func saveScaledPNG(path string, pixels [][3]uint8) error {
	img := image.NewRGBA(image.Rect(0, 0, compose.DemoWidth*compose.DemoScale, compose.DemoHeight*compose.DemoScale))
	for y := 0; y < compose.DemoHeight; y++ {
		for x := 0; x < compose.DemoWidth; x++ {
			rgb := pixels[y*compose.DemoWidth+x]
			col := color.RGBA{R: rgb[0], G: rgb[1], B: rgb[2], A: 255}
			for dy := 0; dy < compose.DemoScale; dy++ {
				for dx := 0; dx < compose.DemoScale; dx++ {
					img.SetRGBA(x*compose.DemoScale+dx, y*compose.DemoScale+dy, col)
				}
			}
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		return err
	}
	return f.Close()
}

func sha256File(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func maxFloat(a, b float64) float64 {
	if b > a {
		return b
	}
	return a
}

func maxInt(a, b int) int {
	if b > a {
		return b
	}
	return a
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "失败:", err)
	os.Exit(1)
}
