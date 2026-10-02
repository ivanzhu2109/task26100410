package kitchen

// unit 是最小工作单元（一份餐品）。
type unit struct {
	id    uint64
	done  bool
	alive bool // false 表示已被撤销，永久失效
}

// line 是票据内的一行，units 按新增顺序排列。
type line struct {
	sku   string
	units []*unit
}

// ticket 是一张票据的全部状态。
type ticket struct {
	version uint64
	nextID  uint64
	lines   map[string]*line
	units   map[uint64]*unit // 全票据范围内按 unit id 索引
}

func newTicket() *ticket {
	return &ticket{
		lines: make(map[string]*line),
		units: make(map[uint64]*unit),
	}
}

// apply 将一次修订归并进票据；版本不高于当前版本时忽略。
func (t *ticket) apply(rev Revision) {
	if rev.Version <= t.version {
		return
	}
	t.version = rev.Version

	incoming := make(map[string]Line, len(rev.Lines))
	for _, l := range rev.Lines {
		incoming[l.LineID] = l
	}

	// 整行移除：撤销该行全部份数，并删除行，使以后同 ID 重加时全部是新工作。
	for id, ln := range t.lines {
		if _, ok := incoming[id]; !ok {
			for _, u := range ln.units {
				u.alive = false
			}
			delete(t.lines, id)
		}
	}

	for lineID, l := range incoming {
		ln, ok := t.lines[lineID]
		if !ok {
			ln = &line{sku: l.SKU}
			t.lines[lineID] = ln
		}
		alive := ln.aliveCount()
		switch {
		case l.Qty > alive:
			ln.addUnits(t, l.Qty-alive)
		case l.Qty < alive:
			ln.revoke(alive - l.Qty)
		}
	}
}

func (ln *line) aliveCount() int {
	n := 0
	for _, u := range ln.units {
		if u.alive {
			n++
		}
	}
	return n
}

// addUnits 在行尾追加 n 份待制作工作。
func (ln *line) addUnits(t *ticket, n int) {
	for i := 0; i < n; i++ {
		t.nextID++
		u := &unit{id: t.nextID, alive: true}
		ln.units = append(ln.units, u)
		t.units[u.id] = u
	}
}

// revoke 撤销 n 份：先撤尚未完工中最近新增的，再撤已完工中最近新增的。
func (ln *line) revoke(n int) {
	for i := len(ln.units) - 1; i >= 0 && n > 0; i-- {
		if u := ln.units[i]; u.alive && !u.done {
			u.alive = false
			n--
		}
	}
	for i := len(ln.units) - 1; i >= 0 && n > 0; i-- {
		if u := ln.units[i]; u.alive && u.done {
			u.alive = false
			n--
		}
	}
}
