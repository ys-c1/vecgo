package composite

import "testing"

// TestDualPathMatchesExact：双路径（float 快速 + big.Int 回退）必须与
// 纯 big.Int 精确路径在大范围输入上逐像素一致。
func TestDualPathMatchesExact(t *testing.T) {
	// 选取跨越舍入边界的 alpha 与颜色组合，包括大量会触发回退的边界值。
	alphas := []uint8{0, 1, 2, 7, 31, 63, 80, 100, 127, 128, 160, 170, 200, 254, 255}
	colors := []uint8{0, 1, 2, 3, 6, 11, 16, 17, 21, 64, 127, 128, 200, 254, 255}

	layerA, _ := NewLayer(1, 1)
	layerB, _ := NewLayer(1, 1)
	layerC, _ := NewLayer(1, 1)
	checked := 0
	for _, aa := range alphas {
		for _, ab := range alphas {
			for _, ac := range alphas {
				for _, ca := range colors {
					for _, cb := range colors {
						for _, cc := range colors {
							layerA.Pix[0] = RGBA{R: ca, G: ca, B: ca, A: aa}
							layerB.Pix[0] = RGBA{R: cb, G: cb, B: cb, A: ab}
							layerC.Pix[0] = RGBA{R: cc, G: cc, B: cc, A: ac}

							fast, err := Composite([]*Layer{layerA, layerB, layerC})
							if err != nil {
								t.Fatal(err)
							}
							exact := compositePixelFoldRight([]RGBA{
								layerA.Pix[0], layerB.Pix[0], layerC.Pix[0],
							})
							if fast.Pix[0] != exact {
								t.Fatalf("双路径与精确路径不一致: 输入 A=%+v B=%+v C=%+v fast=%+v exact=%+v",
									layerA.Pix[0], layerB.Pix[0], layerC.Pix[0], fast.Pix[0], exact)
							}
							checked++
						}
					}
				}
			}
		}
	}
	t.Logf("双路径对拍像素数 = %d，全部一致", checked)
}
