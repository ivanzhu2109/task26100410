package kitchen

import (
	"sort"
	"sync"
)

// Credential 是某次查看时为单个工位签发的完工凭据，
// 只覆盖查看时该票据在该工位尚未完成的份。
// 凭据不会因票据版本变化而整体失效。
type Credential struct {
	ticketID string
	station  string
	unitIDs  []uint64
}

// Store 是餐票进度归并库的主入口，并发安全，数据仅在进程内存。
type Store struct {
	mu         sync.Mutex
	skuStation map[string]string
	tickets    map[string]*ticket
}

// NewStore 创建库实例，skuStation 为 SKU 到工位的映射，之后不变。
func NewStore(skuStation map[string]string) *Store {
	m := make(map[string]string, len(skuStation))
	for sku, station := range skuStation {
		m[sku] = station
	}
	return &Store{skuStation: m, tickets: make(map[string]*ticket)}
}

// ApplyRevision 应用一次修订；版本不高于当前版本时不改变任何状态。
func (s *Store) ApplyRevision(rev Revision) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tickets[rev.TicketID]
	if !ok {
		t = newTicket()
		s.tickets[rev.TicketID] = t
	}
	t.apply(rev)
}

// View 返回票据分工位进度视图，并为每个工位签发完工凭据。
// 凭据只覆盖此次查看时该票据在该工位尚未完成的份。
func (s *Store) View(ticketID string) TicketView {
	s.mu.Lock()
	defer s.mu.Unlock()

	view := TicketView{TicketID: ticketID, Status: StatusWithdrawn}
	t, ok := s.tickets[ticketID]
	if !ok {
		return view
	}
	view.Version = t.version

	// station -> lineID -> 聚合计数；同时收集各工位待完成份用于签发凭据。
	type agg struct {
		sku           string
		done, pending int
	}
	byStation := make(map[string]map[string]*agg)
	credUnits := make(map[string][]uint64)
	aliveTotal, doneTotal := 0, 0

	for lineID, ln := range t.lines {
		station := s.skuStation[ln.sku]
		for _, u := range ln.units {
			if !u.alive {
				continue
			}
			aliveTotal++
			lines, ok := byStation[station]
			if !ok {
				lines = make(map[string]*agg)
				byStation[station] = lines
			}
			a, ok := lines[lineID]
			if !ok {
				a = &agg{sku: ln.sku}
				lines[lineID] = a
			}
			if u.done {
				a.done++
				doneTotal++
			} else {
				a.pending++
				credUnits[station] = append(credUnits[station], u.id)
			}
		}
	}

	stations := make([]string, 0, len(byStation))
	for station := range byStation {
		stations = append(stations, station)
	}
	sort.Strings(stations)
	for _, station := range stations {
		lines := byStation[station]
		lineIDs := make([]string, 0, len(lines))
		for lineID := range lines {
			lineIDs = append(lineIDs, lineID)
		}
		sort.Strings(lineIDs)
		sv := StationView{Station: station}
		for _, lineID := range lineIDs {
			a := lines[lineID]
			sv.Lines = append(sv.Lines, LineProgress{
				LineID:  lineID,
				SKU:     a.sku,
				Done:    a.done,
				Pending: a.pending,
			})
		}
		ids := credUnits[station]
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		sv.Credential = Credential{ticketID: ticketID, station: station, unitIDs: ids}
		view.Stations = append(view.Stations, sv)
	}

	switch {
	case aliveTotal == 0:
		view.Status = StatusWithdrawn
	case doneTotal == aliveTotal:
		view.Status = StatusReady
	default:
		view.Status = StatusInProgress
	}
	return view
}

// Submit 提交完工凭据，只完成其覆盖范围内仍然存在且尚未完成的份，
// 返回本次新完成的份数。重复提交或凭据交叠不会重复计入。
func (s *Store) Submit(cred Credential) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tickets[cred.ticketID]
	if !ok {
		return 0
	}
	completed := 0
	for _, id := range cred.unitIDs {
		u, ok := t.units[id]
		if !ok || !u.alive || u.done {
			continue
		}
		u.done = true
		completed++
	}
	return completed
}
