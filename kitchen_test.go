package kds

import "testing"

func testKitchen() *Kitchen {
	return New(map[string]string{
		"steak": "grill",
		"fries": "fry",
	})
}

func findStation(t *testing.T, v TicketView, station string) StationView {
	t.Helper()
	for _, sv := range v.Stations {
		if sv.Station == station {
			return sv
		}
	}
	t.Fatalf("station %q not found in view", station)
	return StationView{}
}

func findLine(t *testing.T, sv StationView, lineID string) LineProgress {
	t.Helper()
	for _, lp := range sv.Lines {
		if lp.LineID == lineID {
			return lp
		}
	}
	t.Fatalf("line %q not found in station %q", lineID, sv.Station)
	return LineProgress{}
}

// 场景一：5 份 -> 凭据A；加到 8 -> 凭据B；减到 6；再加到 8。
// 任意顺序重复提交 A、B，最终恰好完成 6 份、剩余 2 份；
// 这 2 份只能由包含它们的新凭据完成。
func TestShrinkAndRegrowTokens(t *testing.T) {
	orders := [][]string{{"A", "B"}, {"B", "A"}}
	for _, order := range orders {
		t.Run(order[0]+order[1], func(t *testing.T) {
			k := testKitchen()
			k.ApplyRevision(Revision{TicketID: "t1", Version: 1, Lines: []Line{
				{LineID: "l1", SKU: "steak", Qty: 5},
			}})
			tokA := findStation(t, k.View("t1"), "grill").Token

			k.ApplyRevision(Revision{TicketID: "t1", Version: 2, Lines: []Line{
				{LineID: "l1", SKU: "steak", Qty: 8},
			}})
			tokB := findStation(t, k.View("t1"), "grill").Token

			k.ApplyRevision(Revision{TicketID: "t1", Version: 3, Lines: []Line{
				{LineID: "l1", SKU: "steak", Qty: 6},
			}})
			k.ApplyRevision(Revision{TicketID: "t1", Version: 4, Lines: []Line{
				{LineID: "l1", SKU: "steak", Qty: 8},
			}})

			tokens := map[string]Token{"A": tokA, "B": tokB}
			// 任意顺序重复提交。
			for round := 0; round < 2; round++ {
				for _, name := range order {
					k.Submit(tokens[name])
				}
			}

			sv := findStation(t, k.View("t1"), "grill")
			lp := findLine(t, sv, "l1")
			if lp.Done != 6 || lp.Open != 2 {
				t.Fatalf("after A+B: got done=%d open=%d, want done=6 open=2", lp.Done, lp.Open)
			}
			if got := k.View("t1").Status; got != StatusInProgress {
				t.Fatalf("status = %v, want InProgress", got)
			}

			// 剩余 2 份只能由包含它们的新凭据完成。
			tokC := findStation(t, k.View("t1"), "grill").Token
			k.Submit(tokC)
			sv = findStation(t, k.View("t1"), "grill")
			lp = findLine(t, sv, "l1")
			if lp.Done != 8 || lp.Open != 0 {
				t.Fatalf("after C: got done=%d open=%d, want done=8 open=0", lp.Done, lp.Open)
			}
			if got := k.View("t1").Status; got != StatusReady {
				t.Fatalf("status = %v, want Ready", got)
			}
		})
	}
}

// 场景二：一个工位的加菜/撤改不影响另一工位旧凭据对原有工作的确认。
func TestStationIsolation(t *testing.T) {
	k := testKitchen()
	k.ApplyRevision(Revision{TicketID: "t1", Version: 1, Lines: []Line{
		{LineID: "l1", SKU: "steak", Qty: 2},
		{LineID: "l2", SKU: "fries", Qty: 3},
	}})
	view := k.View("t1")
	grillTok := findStation(t, view, "grill").Token
	fryTok := findStation(t, view, "fry").Token

	// grill 工位加菜并撤改，fry 工位旧凭据仍确认原有 3 份。
	k.ApplyRevision(Revision{TicketID: "t1", Version: 2, Lines: []Line{
		{LineID: "l1", SKU: "steak", Qty: 5},
		{LineID: "l2", SKU: "fries", Qty: 3},
	}})
	k.ApplyRevision(Revision{TicketID: "t1", Version: 3, Lines: []Line{
		{LineID: "l1", SKU: "steak", Qty: 1},
		{LineID: "l2", SKU: "fries", Qty: 3},
	}})
	k.Submit(fryTok)

	view = k.View("t1")
	fry := findLine(t, findStation(t, view, "fry"), "l2")
	if fry.Done != 3 || fry.Open != 0 {
		t.Fatalf("fry: got done=%d open=%d, want done=3 open=0", fry.Done, fry.Open)
	}
	grill := findLine(t, findStation(t, view, "grill"), "l1")
	if grill.Done != 0 || grill.Open != 1 {
		t.Fatalf("grill: got done=%d open=%d, want done=0 open=1", grill.Done, grill.Open)
	}

	// grill 旧凭据不能完成 fry 的工作，也不能完成 grill 撤改后新增的份。
	k.ApplyRevision(Revision{TicketID: "t1", Version: 4, Lines: []Line{
		{LineID: "l1", SKU: "steak", Qty: 4},
		{LineID: "l2", SKU: "fries", Qty: 3},
	}})
	k.Submit(grillTok)
	view = k.View("t1")
	grill = findLine(t, findStation(t, view, "grill"), "l1")
	if grill.Done != 1 || grill.Open != 3 {
		t.Fatalf("grill: got done=%d open=%d, want done=1 open=3", grill.Done, grill.Open)
	}
	fry = findLine(t, findStation(t, view, "fry"), "l2")
	if fry.Done != 3 || fry.Open != 0 {
		t.Fatalf("fry: got done=%d open=%d, want done=3 open=0", fry.Done, fry.Open)
	}
}

// 场景三：整行移除再重加后，旧凭据不能完成重加的餐品。
func TestRemoveAndReaddLine(t *testing.T) {
	k := testKitchen()
	k.ApplyRevision(Revision{TicketID: "t1", Version: 1, Lines: []Line{
		{LineID: "l1", SKU: "steak", Qty: 3},
	}})
	oldTok := findStation(t, k.View("t1"), "grill").Token

	// 移除整行 -> 空票据已撤回。
	k.ApplyRevision(Revision{TicketID: "t1", Version: 2, Lines: nil})
	if got := k.View("t1").Status; got != StatusWithdrawn {
		t.Fatalf("status = %v, want Withdrawn", got)
	}

	// 同一行 ID 重加，全部份数是新工作。
	k.ApplyRevision(Revision{TicketID: "t1", Version: 3, Lines: []Line{
		{LineID: "l1", SKU: "steak", Qty: 3},
	}})
	k.Submit(oldTok)
	lp := findLine(t, findStation(t, k.View("t1"), "grill"), "l1")
	if lp.Done != 0 || lp.Open != 3 {
		t.Fatalf("got done=%d open=%d, want done=0 open=3", lp.Done, lp.Open)
	}
}

// 场景四：已完成份数被撤销后，恢复数量不能恢复其完成状态。
func TestRevokeDoneUnits(t *testing.T) {
	k := testKitchen()
	k.ApplyRevision(Revision{TicketID: "t1", Version: 1, Lines: []Line{
		{LineID: "l1", SKU: "steak", Qty: 5},
	}})
	k.Submit(findStation(t, k.View("t1"), "grill").Token)
	if got := k.View("t1").Status; got != StatusReady {
		t.Fatalf("status = %v, want Ready", got)
	}

	// 减到 2：没有待制作份，撤销 3 份已完工的。
	k.ApplyRevision(Revision{TicketID: "t1", Version: 2, Lines: []Line{
		{LineID: "l1", SKU: "steak", Qty: 2},
	}})
	lp := findLine(t, findStation(t, k.View("t1"), "grill"), "l1")
	if lp.Done != 2 || lp.Open != 0 {
		t.Fatalf("got done=%d open=%d, want done=2 open=0", lp.Done, lp.Open)
	}

	// 恢复数量是全新工作，不恢复完成状态。
	k.ApplyRevision(Revision{TicketID: "t1", Version: 3, Lines: []Line{
		{LineID: "l1", SKU: "steak", Qty: 5},
	}})
	lp = findLine(t, findStation(t, k.View("t1"), "grill"), "l1")
	if lp.Done != 2 || lp.Open != 3 {
		t.Fatalf("got done=%d open=%d, want done=2 open=3", lp.Done, lp.Open)
	}
	if got := k.View("t1").Status; got != StatusInProgress {
		t.Fatalf("status = %v, want InProgress", got)
	}
}

// 减量优先撤销最近新增的待制作份，保留原有进度。
func TestShrinkRevokesNewestOpenFirst(t *testing.T) {
	k := testKitchen()
	k.ApplyRevision(Revision{TicketID: "t1", Version: 1, Lines: []Line{
		{LineID: "l1", SKU: "steak", Qty: 5},
	}})
	k.Submit(findStation(t, k.View("t1"), "grill").Token) // 5 份全完成
	k.ApplyRevision(Revision{TicketID: "t1", Version: 2, Lines: []Line{
		{LineID: "l1", SKU: "steak", Qty: 8},
	}})
	// 减到 6：撤销 2 份最近新增的待制作份，已完工 5 份保留。
	k.ApplyRevision(Revision{TicketID: "t1", Version: 3, Lines: []Line{
		{LineID: "l1", SKU: "steak", Qty: 6},
	}})
	lp := findLine(t, findStation(t, k.View("t1"), "grill"), "l1")
	if lp.Done != 5 || lp.Open != 1 {
		t.Fatalf("got done=%d open=%d, want done=5 open=1", lp.Done, lp.Open)
	}
}

// 乱序或重复到达的修订：只接受更高版本。
func TestOutOfOrderRevisions(t *testing.T) {
	k := testKitchen()
	k.ApplyRevision(Revision{TicketID: "t1", Version: 3, Lines: []Line{
		{LineID: "l1", SKU: "steak", Qty: 4},
	}})
	k.ApplyRevision(Revision{TicketID: "t1", Version: 2, Lines: []Line{
		{LineID: "l1", SKU: "steak", Qty: 9},
	}})
	k.ApplyRevision(Revision{TicketID: "t1", Version: 3, Lines: []Line{
		{LineID: "l1", SKU: "steak", Qty: 9},
	}})
	k.ApplyRevision(Revision{TicketID: "t1", Version: 1, Lines: nil})
	view := k.View("t1")
	if view.Version != 3 {
		t.Fatalf("version = %d, want 3", view.Version)
	}
	lp := findLine(t, findStation(t, view, "grill"), "l1")
	if lp.Done != 0 || lp.Open != 4 {
		t.Fatalf("got done=%d open=%d, want done=0 open=4", lp.Done, lp.Open)
	}
}

// 已就绪票据新增工作后立即回到制作中。
func TestReadyBackToInProgress(t *testing.T) {
	k := testKitchen()
	k.ApplyRevision(Revision{TicketID: "t1", Version: 1, Lines: []Line{
		{LineID: "l1", SKU: "steak", Qty: 2},
	}})
	k.Submit(findStation(t, k.View("t1"), "grill").Token)
	if got := k.View("t1").Status; got != StatusReady {
		t.Fatalf("status = %v, want Ready", got)
	}
	k.ApplyRevision(Revision{TicketID: "t1", Version: 2, Lines: []Line{
		{LineID: "l1", SKU: "steak", Qty: 3},
	}})
	if got := k.View("t1").Status; got != StatusInProgress {
		t.Fatalf("status = %v, want InProgress", got)
	}
}

// 重复提交与多凭据覆盖同一批工作，不重复计入。
func TestDuplicateAndOverlappingSubmit(t *testing.T) {
	k := testKitchen()
	k.ApplyRevision(Revision{TicketID: "t1", Version: 1, Lines: []Line{
		{LineID: "l1", SKU: "steak", Qty: 3},
	}})
	tok1 := findStation(t, k.View("t1"), "grill").Token
	tok2 := findStation(t, k.View("t1"), "grill").Token
	for i := 0; i < 3; i++ {
		k.Submit(tok1)
		k.Submit(tok2)
	}
	lp := findLine(t, findStation(t, k.View("t1"), "grill"), "l1")
	if lp.Done != 3 || lp.Open != 0 {
		t.Fatalf("got done=%d open=%d, want done=3 open=0", lp.Done, lp.Open)
	}
}

// 未知票据：查看为空票据状态，提交为空操作。
func TestUnknownTicket(t *testing.T) {
	k := testKitchen()
	view := k.View("nope")
	if view.Status != StatusWithdrawn || len(view.Stations) != 0 {
		t.Fatalf("unknown ticket view = %+v", view)
	}
	k.Submit(Token{ticketID: "nope", unitIDs: map[uint64]struct{}{1: {}}})
}
