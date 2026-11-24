// Command determinism renders the same scene repeatedly inside ONE process
// (no restart between runs) and prints the sha256 of every output file so
// byte-level reproducibility can be checked across iterations and sizes.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"vecgo/config"
	"vecgo/geom"
	"vecgo/pipe"
)

func main() {
	width := flag.Int("width", 64, "image width")
	height := flag.Int("height", 64, "image height")
	sizes := flag.String("sizes", "", "comma-separated WxH list, e.g. 64x64,128x128; runs every size in this one process")
	runs := flag.Int("runs", 10, "number of in-process iterations")
	outDir := flag.String("outdir", ".", "directory for rendered PGM files")
	label := flag.String("label", "run", "filename label")
	flag.Parse()

	scene := [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}}

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "mkdir:", err)
		os.Exit(1)
	}
	fmt.Printf("pid=%d (整个进程生命周期内只启动一次；下列各档各次渲染均不重启进程)\n", os.Getpid())

	type size struct{ w, h int }
	var tiers []size
	if *sizes != "" {
		for _, spec := range strings.Split(*sizes, ",") {
			parts := strings.Split(strings.TrimSpace(spec), "x")
			if len(parts) != 2 {
				fmt.Fprintln(os.Stderr, "非法画幅规格:", spec)
				os.Exit(1)
			}
			w, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
			h, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
			if err1 != nil || err2 != nil {
				fmt.Fprintln(os.Stderr, "非法画幅规格:", spec)
				os.Exit(1)
			}
			tiers = append(tiers, size{w, h})
		}
	} else {
		tiers = []size{{*width, *height}}
	}

	for _, tier := range tiers {
		tierLabel := fmt.Sprintf("%s_%dx%d", *label, tier.w, tier.h)
		for i := 1; i <= *runs; i++ {
			cfg := config.Default() // fresh, zero-valued struct every iteration: no shared state
			cfg.Width, cfg.Height = tier.w, tier.h
			path := filepath.Join(*outDir, fmt.Sprintf("%s_%02d.pgm", tierLabel, i))
			cfg.Output = path

			im, err := pipe.Run(scene, cfg)
			if err != nil {
				fmt.Fprintln(os.Stderr, "render:", err)
				os.Exit(1)
			}
			if err := im.SavePGM(path); err != nil {
				fmt.Fprintln(os.Stderr, "save:", err)
				os.Exit(1)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				fmt.Fprintln(os.Stderr, "read:", err)
				os.Exit(1)
			}
			sum := sha256.Sum256(data)
			fmt.Printf("%s pid=%d iter=%02d size=%dx%d bytes=%d sha256=%s\n",
				tierLabel, os.Getpid(), i, tier.w, tier.h, len(data), hex.EncodeToString(sum[:]))
		}
	}
}
