package kds

// Submit 提交工位完工凭据，只完成凭据覆盖范围内仍然存在且尚未完成的份；
// 已撤销或已完成的部分忽略。重复提交不会重复计入。
func (k *Kitchen) Submit(tok Token) {
	if len(tok.unitIDs) == 0 {
		return
	}
	t, ok := k.tickets[tok.ticketID]
	if !ok {
		return
	}
	for _, ls := range t.lines {
		for _, u := range ls.units {
			if u.done || u.revoked {
				continue
			}
			if _, ok := tok.unitIDs[u.id]; !ok {
				continue
			}
			// 凭据只能完成本工位的工作。
			if u.station != tok.station {
				continue
			}
			u.done = true
		}
	}
}
