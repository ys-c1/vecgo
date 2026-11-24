package pipe

import (
	"testing"

	"vecgo/config"
	"vecgo/geom"
)

func benchScene() [][]geom.Point {
	return [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}}
}

func benchConfig(w, h int) config.Config {
	c := config.Default()
	c.Width, c.Height = w, h
	return c
}

func BenchmarkRunDiagOff64(b *testing.B) {
	cfg := benchConfig(64, 64)
	for i := 0; i < b.N; i++ {
		if _, err := Run(benchScene(), cfg); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRunDiagOn64(b *testing.B) {
	cfg := benchConfig(64, 64)
	for i := 0; i < b.N; i++ {
		if _, err := RunDiag(benchScene(), cfg, &Diag{}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRunDiagOff512(b *testing.B) {
	cfg := benchConfig(512, 512)
	for i := 0; i < b.N; i++ {
		if _, err := Run(benchScene(), cfg); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRunDiagOn512(b *testing.B) {
	cfg := benchConfig(512, 512)
	for i := 0; i < b.N; i++ {
		if _, err := RunDiag(benchScene(), cfg, &Diag{}); err != nil {
			b.Fatal(err)
		}
	}
}
