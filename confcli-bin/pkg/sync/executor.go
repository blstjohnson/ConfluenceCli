package sync

import (
	"context"
	"fmt"
	"log"

	"confcli/pkg/api"
)

// Executor applies a Plan against Confluence: creates/updates pages,
// rewrites identity and hash labels, and surfaces per-action errors
// without aborting the run.
//
// Orphan handling is out of scope here — bead ou4 (a separate feature)
// will decide whether to rename, label, or delete orphaned pages. The
// executor currently reports orphan counts and leaves orphan pages
// untouched.
type Executor struct {
	client     api.Client
	spaceKey   string
	rootPageID int
	versionMsg string
	logger     *log.Logger
}

// ExecutorOptions bundles Executor constructor arguments.
type ExecutorOptions struct {
	Client     api.Client
	SpaceKey   string
	RootPageID int
	// VersionMsg is recorded on every page update. Empty defaults to a
	// terse "confcli sync".
	VersionMsg string
	Logger     *log.Logger
}

// NewExecutor constructs an Executor. Client, SpaceKey, and RootPageID
// are required.
func NewExecutor(opts ExecutorOptions) (*Executor, error) {
	if opts.Client == nil {
		return nil, fmt.Errorf("sync.NewExecutor: Client is required")
	}
	if opts.SpaceKey == "" {
		return nil, fmt.Errorf("sync.NewExecutor: SpaceKey is required")
	}
	if opts.RootPageID == 0 {
		return nil, fmt.Errorf("sync.NewExecutor: RootPageID is required")
	}
	msg := opts.VersionMsg
	if msg == "" {
		msg = "confcli sync"
	}
	return &Executor{
		client:     opts.Client,
		spaceKey:   opts.SpaceKey,
		rootPageID: opts.RootPageID,
		versionMsg: msg,
		logger:     opts.Logger,
	}, nil
}

// Outcome is the executor's report after applying a Plan. Counts are
// post-execution: a failed create increments Errors, not Created.
type Outcome struct {
	Created  int
	Updated  int
	Skipped  int
	Orphaned int
	Errors   []ActionError
}

// ActionError pairs an Action with the error its execution produced.
type ActionError struct {
	Action Action
	Err    error
}

func (e ActionError) Error() string {
	return fmt.Sprintf("%s %q: %v", e.Action.Kind, e.Action.RelPath, e.Err)
}

// Apply runs the Plan against Confluence. Errors on individual actions
// are collected into Outcome.Errors; the loop continues so a single
// broken page cannot block the rest of the sync.
//
// Parent resolution: actions are emitted parent-before-child by the
// engine, so by the time a child runs its parent's PageID is in the
// path→id map. Children whose parent failed to create are reported as
// errors (the parent has no ID to attach to).
func (x *Executor) Apply(ctx context.Context, plan *Plan) *Outcome {
	out := &Outcome{}

	// Pre-populate the parent-resolution map with every page the engine
	// already located on the server. Without this, a child whose parent
	// is a skip-action (existing, unchanged) would fail with "parent has
	// no page id" — the skip branch wouldn't have added it.
	pageIDs := map[string]int{} // relPath → pageID
	for _, a := range plan.Actions {
		if a.RelPath != "" && a.PageID != 0 {
			pageIDs[a.RelPath] = a.PageID
		}
	}

	for _, action := range plan.Actions {
		switch action.Kind {
		case ActionSkip:
			out.Skipped++

		case ActionOrphan:
			out.Orphaned++

		case ActionCreate:
			parentID, err := x.resolveParent(action, pageIDs)
			if err != nil {
				out.Errors = append(out.Errors, ActionError{Action: action, Err: err})
				continue
			}
			page, err := x.client.CreatePage(ctx, x.spaceKey, parentID, action.Title, action.Storage, "storage")
			if err != nil {
				out.Errors = append(out.Errors, ActionError{Action: action, Err: fmt.Errorf("create: %w", err)})
				continue
			}
			pageID, ok := page.ID.Int()
			if !ok {
				out.Errors = append(out.Errors, ActionError{Action: action, Err: fmt.Errorf("create: response page id %v is not an integer", page.ID)})
				continue
			}
			pageIDs[action.RelPath] = pageID

			if err := x.applyLabels(ctx, pageID, "", action.IDLabel, "", action.NewHashLabel); err != nil {
				out.Errors = append(out.Errors, ActionError{Action: action, Err: fmt.Errorf("label: %w", err)})
				continue
			}
			out.Created++

		case ActionUpdate:
			// Updates don't need parent resolution — the page already
			// exists at its current location. Sync v1 does not reparent
			// pages on file move (move is treated as orphan + create).
			_, err := x.client.UpdatePage(ctx, action.PageID, action.Storage, x.versionMsg, "storage")
			if err != nil {
				out.Errors = append(out.Errors, ActionError{Action: action, Err: fmt.Errorf("update: %w", err)})
				continue
			}
			pageIDs[action.RelPath] = action.PageID

			if err := x.applyLabels(ctx, action.PageID, action.IDLabel, action.IDLabel, action.OldHashLabel, action.NewHashLabel); err != nil {
				out.Errors = append(out.Errors, ActionError{Action: action, Err: fmt.Errorf("label: %w", err)})
				continue
			}
			out.Updated++
		}
	}

	return out
}

// resolveParent returns the Confluence parent ID for action. Empty
// ParentRelPath means the parent is the sync root. A non-empty path
// missing from pageIDs means the parent's create action failed earlier
// in this run.
func (x *Executor) resolveParent(action Action, pageIDs map[string]int) (*int, error) {
	if action.ParentRelPath == "" {
		id := x.rootPageID
		return &id, nil
	}
	id, ok := pageIDs[action.ParentRelPath]
	if !ok {
		return nil, fmt.Errorf("parent %q has no page id (likely failed earlier in this run)", action.ParentRelPath)
	}
	return &id, nil
}

// applyLabels reconciles a page's identity and hash labels. Existing
// labels (oldID, oldHash) are removed only if they differ from the new
// values, to avoid an unnecessary remove+add cycle that bumps the page's
// last-modified time. AddLabel is idempotent server-side, so re-adding a
// matching label is harmless.
func (x *Executor) applyLabels(ctx context.Context, pageID int, oldID, newID, oldHash, newHash string) error {
	if oldID != "" && oldID != newID {
		if err := x.client.RemoveLabel(ctx, pageID, oldID); err != nil {
			return fmt.Errorf("remove old id label %q: %w", oldID, err)
		}
	}
	if newID != "" {
		if err := x.client.AddLabel(ctx, pageID, newID); err != nil {
			return fmt.Errorf("add id label %q: %w", newID, err)
		}
	}
	if oldHash != "" && oldHash != newHash {
		if err := x.client.RemoveLabel(ctx, pageID, oldHash); err != nil {
			return fmt.Errorf("remove old hash label %q: %w", oldHash, err)
		}
	}
	if newHash != "" {
		if err := x.client.AddLabel(ctx, pageID, newHash); err != nil {
			return fmt.Errorf("add hash label %q: %w", newHash, err)
		}
	}
	return nil
}
