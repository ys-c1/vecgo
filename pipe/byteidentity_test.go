package pipe

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"

	"vecgo/config"
	"vecgo/geom"
)

// 与 cmd/main.go 相同的默认场景。
func identityScene() [][]geom.Point {
	return [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}}
}

func hashSavedPGM(t *testing.T, cfg config.Config) string {
	t.Helper()
	im, err := Run(identityScene(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "identity.pgm")
	if err := im.SavePGM(p); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(b)
	return hex(sum[:])
}

func hex(b []byte) string {
	const d = "0123456789abcdef"
	out := make([]byte, 0, len(b)*2)
	for _, x := range b {
		out = append(out, d[x>>4], d[x&0xf])
	}
	return string(out)
}

// 默认（单光源/flat、gamma=1、blur=0）路径必须与改动前 fad87b8 录制的
// PGM 完整字节 sha256 相同。该常量也可用 `go run ./cmd /tmp/x.pgm` 复核，
// stdout 仍为 on=4096 size=64x64。
func TestDefaultArtifactByteIdentical(t *testing.T) {
	cases := []struct {
		name string
		cfg  config.Config
		want string
	}{
		{
			"flat64",
			config.Default(),
			"fbda3e5665174433272beab4f25172bc03466e3f8700bcf6007b32c3636f2dc3",
		},
		{
			"gradient64",
			func() config.Config { c := config.Default(); c.Shading = "gradient"; return c }(),
			"b31af655f7ee15cb9cc4133a6d4e95ad69b8b9c0fd1e82d9728278189b4224a7",
		},
		{
			"gradient256",
			func() config.Config {
				c := config.Default()
				c.Shading = "gradient"
				c.Width, c.Height = 256, 256
				return c
			}(),
			"005f598269f05555e7b788edc07e6bec4375a9e2a1b1046cf95886c692a742a7",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hashSavedPGM(t, tc.cfg); got != tc.want {
				t.Fatalf("默认产物字节改变\n got %s\nwant %s", got, tc.want)
			}
		})
	}
}
