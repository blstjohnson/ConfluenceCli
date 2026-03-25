package transforms

import "fmt"

// Transform is the interface that all transformation steps must implement.
// Each transform operates on a TransformContext and may modify the page content.
type Transform interface {
	// Apply executes the transformation on the given context.
	Apply(ctx *TransformContext) error

	// Name returns a human-readable name for logging/debugging.
	Name() string
}

// TransformContext holds all the data a transform may read or modify.
type TransformContext struct {
	// Content before format conversion (Confluence storage/export_view format).
	PreContent string

	// Content after format conversion (e.g. Markdown).
	PostContent string

	// Page metadata
	PageID    int
	PagePath  string // file path relative to export root
	PageTitle string

	// Format indicates the output format (e.g. "markdown", "html").
	Format string

	// OutputPath is the destination file path on disk.
	OutputPath string
}

// Phase indicates which content field a transform operates on.
type Phase int

const (
	// PhasePre indicates the transform operates on PreContent (before conversion).
	PhasePre Phase = iota
	// PhasePost indicates the transform operates on PostContent (after conversion).
	PhasePost
)

// TransformError wraps an error with the transform name that caused it.
type TransformError struct {
	TransformName string
	Err           error
}

func (e *TransformError) Error() string {
	return fmt.Sprintf("transform %q: %v", e.TransformName, e.Err)
}

func (e *TransformError) Unwrap() error {
	return e.Err
}
