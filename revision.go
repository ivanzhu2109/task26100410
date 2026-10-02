package kds

// Line 是修订中的一行餐品。
type Line struct {
	LineID string
	SKU    string
	Qty    int
}

// Revision 是一次票据修订，包含完整餐品清单。
type Revision struct {
	TicketID string
	Version  uint64
	Lines    []Line
}

// ApplyRevision 接收票据修订。只接受高于当前版本的修订；
// 较低或相同版本不改变任何状态。
func (k *Kitchen) ApplyRevision(rev Revision) {
	t, ok := k.tickets[rev.TicketID]
	if !ok {
		t = &ticket{lines: make(map[string]*lineState)}
		k.tickets[rev.TicketID] = t
	}
	if rev.Version <= t.version {
		return
	}
	t.version = rev.Version

	incoming := make(map[string]Line, len(rev.Lines))
	for _, l := range rev.Lines {
		incoming[l.LineID] = l
	}

	// 移除整行：撤销该行全部份数。
	for lineID, ls := range t.lines {
		if _, ok := incoming[lineID]; !ok {
			for _, u := range ls.units {
				u.revoked = true
			}
			delete(t.lines, lineID)
		}
	}

	for _, l := range rev.Lines {
		if l.Qty < 0 {
			l.Qty = 0
		}
		ls, ok := t.lines[l.LineID]
		if !ok {
			ls = &lineState{sku: l.SKU}
			t.lines[l.LineID] = ls
		}
		// 同一票据内行 ID 对应的 SKU 固定，忽略后续不一致的 SKU。
		cur := len(ls.units)
		switch {
		case l.Qty > cur:
			for i := 0; i < l.Qty-cur; i++ {
				k.nextUnitID++
				ls.units = append(ls.units, &unit{
					id:      k.nextUnitID,
					lineID:  l.LineID,
					station: k.skuStation[ls.sku],
				})
			}
		case l.Qty < cur:
			k.revokeUnits(ls, cur-l.Qty)
		}
	}
}

// revokeUnits 撤销 n 份：先撤销尚未完工份数中最近新增的部分，
// 仍不够时再撤销已完工份数中最近新增的部分。撤销永久生效。
func (k *Kitchen) revokeUnits(ls *lineState, n int) {
	revoked := 0
	for i := len(ls.units) - 1; i >= 0 && revoked < n; i-- {
		if !ls.units[i].done {
			ls.units[i].revoked = true
			revoked++
		}
	}
	for i := len(ls.units) - 1; i >= 0 && revoked < n; i-- {
		if !ls.units[i].revoked {
			ls.units[i].revoked = true
			revoked++
		}
	}
	kept := ls.units[:0]
	for _, u := range ls.units {
		if !u.revoked {
			kept = append(kept, u)
		}
	}
	ls.units = kept
}
