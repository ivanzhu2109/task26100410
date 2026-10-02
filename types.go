// Package kitchen 提供厨房餐票进度归并能力，数据仅保存在进程内存。
package kitchen

// Status 表示整张票据的状态。
type Status int

const (
	// StatusWithdrawn 空票据（当前没有任何有效份数）。
	StatusWithdrawn Status = iota
	// StatusInProgress 非空票据，尚有份数未完成。
	StatusInProgress
	// StatusReady 非空票据，当前所有份数均已完成。
	StatusReady
)

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

// LineProgress 是某工位下某一行的进度。
type LineProgress struct {
	LineID  string
	SKU     string
	Done    int
	Pending int
}

// StationView 是某工位的进度视图。
type StationView struct {
	Station    string
	Lines      []LineProgress
	Credential Credential
}

// TicketView 是整张票据的视图。
type TicketView struct {
	TicketID string
	Version  uint64
	Status   Status
	Stations []StationView
}
