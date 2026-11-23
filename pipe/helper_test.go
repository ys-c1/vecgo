package pipe

import (
	"testing"

	"vecgo/raster"
)

func newImage(t *testing.T, w, h int) (*raster.Image, error) {
	t.Helper()
	return raster.New(w, h)
}
