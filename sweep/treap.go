package sweep

import (
	"errors"
	"fmt"
)

// treap 是活动段状态结构。优先级由段号经固定 splitmix64 派生，
// 不使用随机源：同一段号在任何进程中得到同一棵树，树高与路径长度
// 都可复现，实测复杂度才有意义。
type tnode struct {
	id, prio            int
	left, right, parent *tnode
}

type treap struct {
	root  *tnode
	nodes map[int]*tnode
	size  int
}

// splitmix64 固定常量，仅用于把段号打散成 treap 优先级。
func splitmix64(z uint64) uint64 {
	z += 0x9e3779b97f4a7c15
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}

// insert 插入段 id，less 是当前扫描位置上的活动段全序。
// 返回搜索路径上的比较次数（根为 1）。
func (t *treap) insert(id int, less func(a, b int) bool) int {
	if t.nodes == nil {
		t.nodes = make(map[int]*tnode)
	}
	n := &tnode{id: id, prio: int(splitmix64(uint64(id)+1) >> 1)}
	t.nodes[id] = n
	t.size++
	if t.root == nil {
		t.root = n
		return 1
	}
	depth := 0
	var parent *tnode
	cur := t.root
	for cur != nil {
		parent = cur
		depth++
		if less(id, cur.id) {
			cur = cur.left
		} else {
			cur = cur.right
		}
	}
	n.parent = parent
	if less(id, parent.id) {
		parent.left = n
	} else {
		parent.right = n
	}
	// 按优先级修复堆性质（max-heap），旋转时维护 parent 指针。
	for n.parent != nil && n.prio > n.parent.prio {
		if n == n.parent.left {
			t.rotateRight(n.parent)
		} else {
			t.rotateLeft(n.parent)
		}
	}
	return depth
}

func (t *treap) rotateLeft(x *tnode) {
	y := x.right
	x.right = y.left
	if y.left != nil {
		y.left.parent = x
	}
	y.parent = x.parent
	if x.parent == nil {
		t.root = y
	} else if x == x.parent.left {
		x.parent.left = y
	} else {
		x.parent.right = y
	}
	y.left = x
	x.parent = y
}

func (t *treap) rotateRight(x *tnode) {
	y := x.left
	x.left = y.right
	if y.right != nil {
		y.right.parent = x
	}
	y.parent = x.parent
	if x.parent == nil {
		t.root = y
	} else if x == x.parent.right {
		x.parent.right = y
	} else {
		x.parent.left = y
	}
	y.right = x
	x.parent = y
}

// remove 按节点指针删除段 id（旋转到叶子后摘除），不依赖比较器：
// 交点事件发生时比较器采用“扫描线右侧”约定，与树此前的左侧次序
// 可能瞬时不一致，指针删除保证在任何时刻都能正确摘除。
// 返回下沉旋转次数。
func (t *treap) remove(id int) int {
	n := t.nodes[id]
	if n == nil {
		return 0
	}
	sinks := 0
	for n.left != nil || n.right != nil {
		sinks++
		if n.right == nil || (n.left != nil && n.left.prio > n.right.prio) {
			t.rotateRight(n)
		} else {
			t.rotateLeft(n)
		}
	}
	if n.parent == nil {
		t.root = nil
	} else if n == n.parent.left {
		n.parent.left = nil
	} else {
		n.parent.right = nil
	}
	delete(t.nodes, id)
	t.size--
	return sinks
}

// neighbors 返回 id 在当前树序中的前驱与后继，缺失为 -1。
func (t *treap) neighbors(id int) (pred, succ int) {
	n := t.nodes[id]
	if n == nil {
		return -1, -1
	}
	if p, ok := t.predOf(n); ok {
		pred = p.id
	} else {
		pred = -1
	}
	if s, ok := t.succOf(n); ok {
		succ = s.id
	} else {
		succ = -1
	}
	return pred, succ
}

func (t *treap) predOf(n *tnode) (*tnode, bool) {
	if n == nil {
		return nil, false
	}
	if n.left != nil {
		cur := n.left
		for cur.right != nil {
			cur = cur.right
		}
		return cur, true
	}
	for n.parent != nil && n == n.parent.left {
		n = n.parent
	}
	return n.parent, n.parent != nil
}

func (t *treap) succOf(n *tnode) (*tnode, bool) {
	if n == nil {
		return nil, false
	}
	if n.right != nil {
		cur := n.right
		for cur.left != nil {
			cur = cur.left
		}
		return cur, true
	}
	for n.parent != nil && n == n.parent.right {
		n = n.parent
	}
	return n.parent, n.parent != nil
}

// succOfMust 用于调用方保证存在后继的块内遍历。
func (t *treap) succOfMust(n *tnode) *tnode {
	s, ok := t.succOf(n)
	if !ok {
		panic("sweep: treap 后继缺失")
	}
	return s
}

// inOrder 仅用于测试与断言：导出当前树序。
func (t *treap) inOrder() []int {
	out := make([]int, 0, t.size)
	var walk func(*tnode)
	walk = func(n *tnode) {
		if n == nil {
			return
		}
		walk(n.left)
		out = append(out, n.id)
		walk(n.right)
	}
	walk(t.root)
	return out
}

// checkInvariants 仅供测试：验证 parent 指针、节点计数与中序无环。
// 不校验 BST 键序：交点批处理期间比较器采用“扫描线右侧”约定，
// 与树快照的左侧次序允许瞬时不一致（块整体重建后恢复）。
func (t *treap) checkInvariants() error {
	for id, n := range t.nodes {
		if n.left != nil && n.left.parent != n {
			return fmt.Errorf("节点 %d 左孩子的 parent 回指断裂", id)
		}
		if n.right != nil && n.right.parent != n {
			return fmt.Errorf("节点 %d 右孩子的 parent 回指断裂", id)
		}
		if n.parent == nil && n != t.root {
			return fmt.Errorf("节点 %d 的 parent 为 nil 但不是根", id)
		}
	}
	if t.root != nil && t.root.parent != nil {
		return errors.New("根节点存在 parent")
	}
	if len(t.nodes) != t.size {
		return fmt.Errorf("节点计数不一致: map=%d size=%d", len(t.nodes), t.size)
	}
	if len(t.inOrder()) != t.size {
		return errors.New("中序遍历存在环或丢节点")
	}
	return nil
}
