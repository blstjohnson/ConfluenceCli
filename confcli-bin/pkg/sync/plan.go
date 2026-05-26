package sync

// ActionKind classifies what the executor must do for a single Action.
type ActionKind string

const (
	// ActionCreate: source markdown has no matching page on the server.
	ActionCreate ActionKind = "create"
	// ActionUpdate: page exists, but its content hash differs from the source.
	ActionUpdate ActionKind = "update"
	// ActionSkip: page exists and its content hash matches — no-op.
	ActionSkip ActionKind = "skip"
	// ActionOrphan: page exists on the server with a confcli-id label, but
	// no source file maps to it. The executor decides the policy (label,
	// rename, delete) — the engine only flags it.
	ActionOrphan ActionKind = "orphan"
)

// Action describes one operation the executor will perform.
//
// Fields are populated by ActionKind:
//
//	create:  RelPath, Title, IDLabel, NewHashLabel, Storage, ParentRelPath
//	update:  RelPath, Title, IDLabel, NewHashLabel, OldHashLabel, Storage,
//	         PageID, Version, ParentRelPath
//	skip:    RelPath, Title, IDLabel, OldHashLabel, PageID
//	orphan:  PageID, Title, IDLabel  (no source file)
//
// Reason is always populated with a human-readable explanation.
type Action struct {
	Kind ActionKind

	// RelPath is the sync-root-relative path of the source markdown file
	// (forward slashes). Empty for orphans.
	RelPath string

	// ParentRelPath is the sync-root-relative path of the parent page's
	// source file (e.g. "docs/README.md"). Empty when the parent is the
	// sync root page (passed to BuildPlan as rootPageID).
	ParentRelPath string

	// Title is the page title the executor should publish under. For create
	// and update this is the source-derived title; for skip and orphan it
	// is the current server-side title.
	Title string

	// PageID is the existing Confluence page (update / skip / orphan only).
	PageID int

	// Version is the current page version (update only). The executor
	// passes this to the update API which will atomically bump it.
	Version int

	// IDLabel is the confcli-id-<sha1> label that anchors this page to its
	// source file. Always populated except for orphans whose source file
	// was renamed/deleted (where it is the label we saw on the server).
	IDLabel string

	// OldHashLabel is the confcli-hash-<sha1> label currently on the page
	// (update / skip only).
	OldHashLabel string

	// NewHashLabel is the confcli-hash-<sha1> label that should be on the
	// page after the action runs (create / update only).
	NewHashLabel string

	// Storage is the Confluence storage-format payload to publish (create
	// / update only).
	Storage string

	// Reason is a short human-readable explanation, e.g.
	// "no page with this id label" or "hash matches existing page".
	Reason string
}

// Plan is the engine's output: an ordered list of actions plus aggregate
// stats. Actions are ordered so parents precede children — the executor may
// apply them sequentially without resolving dependencies itself.
type Plan struct {
	Actions []Action
	Stats   PlanStats
}

// PlanStats aggregates Action counts by kind. The executor surfaces these
// at the end of a run ("N created, N updated, N skipped, N orphaned").
type PlanStats struct {
	Create int
	Update int
	Skip   int
	Orphan int
}

func (s *PlanStats) add(kind ActionKind) {
	switch kind {
	case ActionCreate:
		s.Create++
	case ActionUpdate:
		s.Update++
	case ActionSkip:
		s.Skip++
	case ActionOrphan:
		s.Orphan++
	}
}
