package kitchen

import "testing"

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
// 任意顺序重复提交 A、B，最终恰好完成 6 份、剩余 2 份，
// 且这 2 份只能由包含它们的新凭据完成。
func TestCredentialCoversOnlyWorkAtViewTime(t *testing.T) {
	orders := map[string][]string{
		"A then B":            {"A", "B"},
		"B then A":            {"B", "A"},
		"interleaved repeats": {"A", "B", "A", "B", "B", "A"},
	}
	for name, order := range orders {
		t.Run(name, func(t *testing.T) {
			s := NewStore(map[string]string{"S1": "grill"})
			s.ApplyRevision(Revision{TicketID: "T1", Version: 1, Lines: []Line{{LineID: "L1", SKU: "S1", Qty: 5}}})
			credA := findStation(t, s.View("T1"), "grill").Credential

			s.ApplyRevision(Revision{TicketID: "T1", Version: 2, Lines: []Line{{LineID: "L1", SKU: "S1", Qty: 8}}})
			credB := findStation(t, s.View("T1"), "grill").Credential

			s.ApplyRevision(Revision{TicketID: "T1", Version: 3, Lines: []Line{{LineID: "L1", SKU: "S1", Qty: 6}}})
			s.ApplyRevision(Revision{TicketID: "T1", Version: 4, Lines: []Line{{LineID: "L1", SKU: "S1", Qty: 8}}})

			total := 0
			for _, which := range order {
				if which == "A" {
					total += s.Submit(credA)
				} else {
					total += s.Submit(credB)
				}
			}
			if total != 6 {
				t.Fatalf("completed = %d, want 6", total)
			}

			// 再重复提交旧凭据，不能再完成任何份。
			for i := 0; i < 3; i++ {
				if n := s.Submit(credA); n != 0 {
					t.Fatalf("resubmit A completed %d, want 0", n)
				}
				if n := s.Submit(credB); n != 0 {
					t.Fatalf("resubmit B completed %d, want 0", n)
				}
			}

			v := s.View("T1")
			lp := findLine(t, findStation(t, v, "grill"), "L1")
			if lp.Done != 6 || lp.Pending != 2 {
				t.Fatalf("done=%d pending=%d, want 6/2", lp.Done, lp.Pending)
			}
			if v.Status != StatusInProgress {
				t.Fatalf("status = %v, want InProgress", v.Status)
			}

			// 剩余 2 份只能由包含它们的新凭据完成。
			credC := findStation(t, s.View("T1"), "grill").Credential
			if n := s.Submit(credC); n != 2 {
				t.Fatalf("new credential completed %d, want 2", n)
			}
			if v := s.View("T1"); v.Status != StatusReady {
				t.Fatalf("status = %v, want Ready", v.Status)
			}
		})
	}
}

// 场景二：一个工位的修改不影响另一工位旧凭据对原有工作的确认。
func TestStationIsolation(t *testing.T) {
	s := NewStore(map[string]string{"S1": "grill", "S2": "fry"})
	s.ApplyRevision(Revision{TicketID: "T1", Version: 1, Lines: []Line{
		{LineID: "L1", SKU: "S1", Qty: 2},
		{LineID: "L2", SKU: "S2", Qty: 3},
	}})
	grillCred := findStation(t, s.View("T1"), "grill").Credential

	// fry 工位加菜、减量，不影响 grill 的旧凭据。
	s.ApplyRevision(Revision{TicketID: "T1", Version: 2, Lines: []Line{
		{LineID: "L1", SKU: "S1", Qty: 2},
		{LineID: "L2", SKU: "S2", Qty: 5},
	}})
	s.ApplyRevision(Revision{TicketID: "T1", Version: 3, Lines: []Line{
		{LineID: "L1", SKU: "S1", Qty: 2},
		{LineID: "L2", SKU: "S2", Qty: 1},
	}})
	if n := s.Submit(grillCred); n != 2 {
		t.Fatalf("grill credential completed %d, want 2", n)
	}

	// grill 的凭据不得完成 fry 的工作。
	fry := findStation(t, s.View("T1"), "fry")
	if lp := findLine(t, fry, "L2"); lp.Done != 0 || lp.Pending != 1 {
		t.Fatalf("fry L2 done=%d pending=%d, want 0/1", lp.Done, lp.Pending)
	}
	grill := findStation(t, s.View("T1"), "grill")
	if lp := findLine(t, grill, "L1"); lp.Done != 2 || lp.Pending != 0 {
		t.Fatalf("grill L1 done=%d pending=%d, want 2/0", lp.Done, lp.Pending)
	}
}

// 场景三：整行移除再重加后，旧凭据不能完成重加的餐品。
func TestRemoveAndReaddLineInvalidatesOldWork(t *testing.T) {
	s := NewStore(map[string]string{"S1": "grill"})
	s.ApplyRevision(Revision{TicketID: "T1", Version: 1, Lines: []Line{{LineID: "L1", SKU: "S1", Qty: 3}}})
	oldCred := findStation(t, s.View("T1"), "grill").Credential
	if n := s.Submit(oldCred); n != 3 {
		t.Fatalf("completed %d, want 3", n)
	}

	// 整行移除：票据变空，状态为已撤回。
	s.ApplyRevision(Revision{TicketID: "T1", Version: 2, Lines: nil})
	if v := s.View("T1"); v.Status != StatusWithdrawn {
		t.Fatalf("status = %v, want Withdrawn", v.Status)
	}

	// 同一行 ID 重加：全部是新工作，旧凭据不能作用于它。
	s.ApplyRevision(Revision{TicketID: "T1", Version: 3, Lines: []Line{{LineID: "L1", SKU: "S1", Qty: 2}}})
	if n := s.Submit(oldCred); n != 0 {
		t.Fatalf("old credential completed %d on re-added line, want 0", n)
	}
	v := s.View("T1")
	lp := findLine(t, findStation(t, v, "grill"), "L1")
	if lp.Done != 0 || lp.Pending != 2 {
		t.Fatalf("done=%d pending=%d, want 0/2", lp.Done, lp.Pending)
	}
	if v.Status != StatusInProgress {
		t.Fatalf("status = %v, want InProgress", v.Status)
	}
}

// 场景四：已完成份数被撤销后，恢复数量不能恢复其完成状态。
func TestRevokedDoneUnitsStayRevoked(t *testing.T) {
	s := NewStore(map[string]string{"S1": "grill"})
	s.ApplyRevision(Revision{TicketID: "T1", Version: 1, Lines: []Line{{LineID: "L1", SKU: "S1", Qty: 3}}})
	cred := findStation(t, s.View("T1"), "grill").Credential
	if n := s.Submit(cred); n != 3 {
		t.Fatalf("completed %d, want 3", n)
	}
	if v := s.View("T1"); v.Status != StatusReady {
		t.Fatalf("status = %v, want Ready", v.Status)
	}

	// 减到 1：撤销 2 份已完工；再加回 3：是 2 份全新的待制作工作。
	s.ApplyRevision(Revision{TicketID: "T1", Version: 2, Lines: []Line{{LineID: "L1", SKU: "S1", Qty: 1}}})
	s.ApplyRevision(Revision{TicketID: "T1", Version: 3, Lines: []Line{{LineID: "L1", SKU: "S1", Qty: 3}}})
	v := s.View("T1")
	lp := findLine(t, findStation(t, v, "grill"), "L1")
	if lp.Done != 1 || lp.Pending != 2 {
		t.Fatalf("done=%d pending=%d, want 1/2", lp.Done, lp.Pending)
	}
	if v.Status != StatusInProgress {
		t.Fatalf("status = %v, want InProgress", v.Status)
	}

	// 旧凭据不能恢复被撤销份的完成状态，也不能完成新工作。
	if n := s.Submit(cred); n != 0 {
		t.Fatalf("old credential completed %d, want 0", n)
	}
}

// 减量先撤未完工中最近新增的，再撤已完工中最近新增的。
func TestReductionRevokesPendingFirst(t *testing.T) {
	s := NewStore(map[string]string{"S1": "grill"})
	s.ApplyRevision(Revision{TicketID: "T1", Version: 1, Lines: []Line{{LineID: "L1", SKU: "S1", Qty: 2}}})
	credFirst := findStation(t, s.View("T1"), "grill").Credential
	if n := s.Submit(credFirst); n != 2 {
		t.Fatalf("completed %d, want 2", n)
	}

	// 加 2 份（待制作），再减 2 份：应撤掉未完工的 2 份，保留已完工的 2 份。
	s.ApplyRevision(Revision{TicketID: "T1", Version: 2, Lines: []Line{{LineID: "L1", SKU: "S1", Qty: 4}}})
	s.ApplyRevision(Revision{TicketID: "T1", Version: 3, Lines: []Line{{LineID: "L1", SKU: "S1", Qty: 2}}})
	v := s.View("T1")
	lp := findLine(t, findStation(t, v, "grill"), "L1")
	if lp.Done != 2 || lp.Pending != 0 {
		t.Fatalf("done=%d pending=%d, want 2/0", lp.Done, lp.Pending)
	}
	if v.Status != StatusReady {
		t.Fatalf("status = %v, want Ready", v.Status)
	}
}

// 乱序与重复修订：只接受更高版本。
func TestOutOfOrderAndDuplicateRevisions(t *testing.T) {
	s := NewStore(map[string]string{"S1": "grill"})
	s.ApplyRevision(Revision{TicketID: "T1", Version: 2, Lines: []Line{{LineID: "L1", SKU: "S1", Qty: 4}}})
	s.ApplyRevision(Revision{TicketID: "T1", Version: 1, Lines: []Line{{LineID: "L1", SKU: "S1", Qty: 9}}})
	s.ApplyRevision(Revision{TicketID: "T1", Version: 2, Lines: []Line{{LineID: "L1", SKU: "S1", Qty: 1}}})

	v := s.View("T1")
	if v.Version != 2 {
		t.Fatalf("version = %d, want 2", v.Version)
	}
	lp := findLine(t, findStation(t, v, "grill"), "L1")
	if lp.Done != 0 || lp.Pending != 4 {
		t.Fatalf("done=%d pending=%d, want 0/4", lp.Done, lp.Pending)
	}
}

// 票据状态：空票据已撤回；就绪票据新增工作后回到制作中。
func TestStatusTransitions(t *testing.T) {
	s := NewStore(map[string]string{"S1": "grill"})

	if v := s.View("unknown"); v.Status != StatusWithdrawn {
		t.Fatalf("unknown ticket status = %v, want Withdrawn", v.Status)
	}

	s.ApplyRevision(Revision{TicketID: "T1", Version: 1, Lines: []Line{{LineID: "L1", SKU: "S1", Qty: 1}}})
	if v := s.View("T1"); v.Status != StatusInProgress {
		t.Fatalf("status = %v, want InProgress", v.Status)
	}

	cred := findStation(t, s.View("T1"), "grill").Credential
	s.Submit(cred)
	if v := s.View("T1"); v.Status != StatusReady {
		t.Fatalf("status = %v, want Ready", v.Status)
	}

	s.ApplyRevision(Revision{TicketID: "T1", Version: 2, Lines: []Line{{LineID: "L1", SKU: "S1", Qty: 2}}})
	if v := s.View("T1"); v.Status != StatusInProgress {
		t.Fatalf("status = %v, want InProgress", v.Status)
	}
}
