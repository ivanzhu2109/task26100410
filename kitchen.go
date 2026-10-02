// Package kds 提供厨房餐票进度归并能力，数据仅保存在进程内存。
package kds

// Status 表示整张票据的状态。
type Status int

const (
	StatusWithdrawn  Status = iota // 已撤回（空票据）
	StatusInProgress               // 制作中
	StatusReady                    // 已就绪（所有份数完成）
)

func (s Status) String() string {
	switch s {
	case StatusWithdrawn:
		return "Withdrawn"
	case StatusInProgress:
		return "InProgress"
	case StatusReady:
		return "Ready"
	}
	return "Unknown"
}

// unit 表示一份餐品工作单元。
type unit struct {
	id      uint64 // 全局递增，创建顺序即新增顺序
	lineID  string
	station string
	done    bool
	revoked bool // 撤销永久生效
}

// lineState 记录一行餐品的状态；行 ID 对应的 SKU 固定。
type lineState struct {
	sku   string
	units []*unit // 仅包含未撤销的单元
}

// ticket 记录一张票据。
type ticket struct {
	version uint64
	lines   map[string]*lineState
}

// Kitchen 是库的主入口，按工位归并餐票进度。
type Kitchen struct {
	skuStation map[string]string
	tickets    map[string]*ticket
	nextUnitID uint64
}

// New 创建一个 Kitchen。skuStation 为 SKU 到工位的映射，初始化后不变。
func New(skuStation map[string]string) *Kitchen {
	m := make(map[string]string, len(skuStation))
	for k, v := range skuStation {
		m[k] = v
	}
	return &Kitchen{
		skuStation: m,
		tickets:    make(map[string]*ticket),
	}
}
