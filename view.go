package kds

import "sort"

// Token 是工位完工凭据，只覆盖查看时该票据在该工位尚未完成的份。
type Token struct {
	ticketID string
	station  string
	unitIDs  map[uint64]struct{}
}

// LineProgress 是一行餐品的进度。
type LineProgress struct {
	LineID string
	SKU    string
	Done   int
	Open   int
}

// StationView 是一个工位的进度视图。
type StationView struct {
	Station string
	Lines   []LineProgress
	Token   Token
}

// TicketView 是整张票据的视图。
type TicketView struct {
	TicketID string
	Version  uint64
	Status   Status
	Stations []StationView
}

// View 返回票据分工位进度，并为每个工位生成完工凭据。
// 凭据只覆盖这次查看时，该票据在该工位确实尚未完成的那些份。
func (k *Kitchen) View(ticketID string) TicketView {
	view := TicketView{TicketID: ticketID, Status: StatusWithdrawn}
	t, ok := k.tickets[ticketID]
	if !ok {
		return view
	}
	view.Version = t.version

	byStation := make(map[string]map[string]*LineProgress) // station -> lineID -> progress
	tokenUnits := make(map[string]map[uint64]struct{})
	total, doneTotal := 0, 0

	lineIDs := make([]string, 0, len(t.lines))
	for lineID := range t.lines {
		lineIDs = append(lineIDs, lineID)
	}
	sort.Strings(lineIDs)

	for _, lineID := range lineIDs {
		ls := t.lines[lineID]
		for _, u := range ls.units {
			st := u.station
			prog, ok := byStation[st]
			if !ok {
				prog = make(map[string]*LineProgress)
				byStation[st] = prog
			}
			lp, ok := prog[lineID]
			if !ok {
				lp = &LineProgress{LineID: lineID, SKU: ls.sku}
				prog[lineID] = lp
			}
			total++
			if u.done {
				lp.Done++
				doneTotal++
			} else {
				lp.Open++
				if tokenUnits[st] == nil {
					tokenUnits[st] = make(map[uint64]struct{})
				}
				tokenUnits[st][u.id] = struct{}{}
			}
		}
	}

	stations := make([]string, 0, len(byStation))
	for st := range byStation {
		stations = append(stations, st)
	}
	sort.Strings(stations)

	for _, st := range stations {
		sv := StationView{
			Station: st,
			Token: Token{
				ticketID: ticketID,
				station:  st,
				unitIDs:  tokenUnits[st],
			},
		}
		prog := byStation[st]
		ids := make([]string, 0, len(prog))
		for lineID := range prog {
			ids = append(ids, lineID)
		}
		sort.Strings(ids)
		for _, lineID := range ids {
			sv.Lines = append(sv.Lines, *prog[lineID])
		}
		view.Stations = append(view.Stations, sv)
	}

	if total > 0 {
		if doneTotal == total {
			view.Status = StatusReady
		} else {
			view.Status = StatusInProgress
		}
	}
	return view
}
