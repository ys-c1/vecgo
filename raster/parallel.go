package raster

import (
	"errors"
	"fmt"
	"sync"

	"vecgo/geom"
)

// ParallelConfig 配置按行分块并行渲染。
// Blocks：画幅高度方向被切分的块数（每块渲染连续的若干行）。
// Workers：同时运行的最大 goroutine 数（并发度）。
type ParallelConfig struct {
	Blocks  int
	Workers int
}

// taskGroup 抽象一组可等待的并行任务，使并发创建路径可注入、可测试。
type taskGroup interface {
	Go(func()) error
	Wait()
}

// newTaskGroup 是实际的 taskGroup 工厂；测试可临时替换它以模拟并发创建失败。
var newTaskGroup = func(workers int) taskGroup {
	return &semaphoreGroup{sem: make(chan struct{}, workers)}
}

type semaphoreGroup struct {
	wg  sync.WaitGroup
	sem chan struct{}
}

// Go 占用一个并发名额后启动 goroutine；名额耗尽时阻塞等待，不静默丢任务。
func (g *semaphoreGroup) Go(fn func()) error {
	g.sem <- struct{}{}
	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		defer func() { <-g.sem }()
		fn()
	}()
	return nil
}

func (g *semaphoreGroup) Wait() {
	g.wg.Wait()
}

// FillParallel 按行分块并行渲染，与 Fill 产出逐字节相同的结果。
// 它不改动 Fill 的签名与错误约定：任何非法输入都返回带字段名或块号的 error，绝不静默降级。
func (im *Image) FillParallel(polys [][]geom.Point, supersample int, pc ParallelConfig) error {
	if err := im.validateFill(polys, supersample); err != nil {
		return err
	}
	if pc.Blocks <= 0 {
		return fmt.Errorf("blocks 字段必须为正: blocks=%d", pc.Blocks)
	}
	if pc.Workers <= 0 {
		return fmt.Errorf("workers 字段必须为正: workers=%d", pc.Workers)
	}

	// 只有一个块时没有可并行的工作单元，直接走串行基线，避免 goroutine 创建与调度的纯开销。
	if pc.Blocks == 1 {
		return im.Fill(polys, supersample)
	}

	// 块数不能整除画幅高度：显式报错并带上块号信息，不调整块数、不静默改串行。
	if im.H%pc.Blocks != 0 {
		return fmt.Errorf("blocks 字段=%d 不能整除 height 字段=%d: 块号 %d 的行数无法均分",
			pc.Blocks, im.H, pc.Blocks-1)
	}

	// 空场景：结果为全黑图，与 Fill 保持一致，不访问 polys[0]。
	if len(polys) == 0 {
		return nil
	}

	minx, miny, spanx, spany := sceneBounds(polys)
	rowsPerBlock := im.H / pc.Blocks
	group := newTaskGroup(pc.Workers)

	for block := 0; block < pc.Blocks; block++ {
		block := block
		startRow := block * rowsPerBlock
		endRow := startRow + rowsPerBlock
		err := group.Go(func() {
			// 每个 goroutine 只写自己行区间内的像素，行区间互不相交，无数据竞争。
			im.renderRows(polys, supersample, minx, miny, spanx, spany, startRow, endRow)
		})
		if err != nil {
			return fmt.Errorf("块号 %d: 并发创建失败 workers 字段=%d: %w", block, pc.Workers, err)
		}
	}
	group.Wait()
	return nil
}

// failGroup 是供测试使用的 taskGroup：从第 failAfter 次 Go 起返回创建失败。
type failGroup struct {
	calls     int
	failAfter int
}

func (g *failGroup) Go(func()) error {
	g.calls++
	if g.calls > g.failAfter {
		return errors.New("注入的并发创建失败")
	}
	return nil
}

func (g *failGroup) Wait() {}
