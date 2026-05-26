package md

import (
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// AlertLevel identifies which GFM alert marker introduced a blockquote.
type AlertLevel string

const (
	AlertNote      AlertLevel = "NOTE"
	AlertTip       AlertLevel = "TIP"
	AlertImportant AlertLevel = "IMPORTANT"
	AlertWarning   AlertLevel = "WARNING"
	AlertCaution   AlertLevel = "CAUTION"
)

// confluencePanelMacro maps a GFM alert level onto the Confluence panel
// macro name. Confluence ships three panel flavors (info / note /
// warning), so the five GFM levels collapse onto them.
func (l AlertLevel) confluencePanelMacro() string {
	switch l {
	case AlertWarning, AlertCaution:
		return "warning"
	case AlertNote, AlertImportant:
		return "note"
	default:
		return "info"
	}
}

// alertAttrKey marks a blockquote that has been identified as a GFM alert.
// The attribute value is the AlertLevel as a string.
const alertAttrKey = "confcli-alert-level"

// alertParagraphTransformer detects GFM alert markers ([!NOTE], [!TIP],
// [!IMPORTANT], [!WARNING], [!CAUTION]) on the first line of a paragraph
// that lives directly inside a blockquote.
//
// It runs in the block-parsing phase, before inline parsing, so that
// trimming the marker line out of the paragraph's Lines() takes effect
// before inline content is built. A matching paragraph has its first
// line stripped (or is removed entirely if the marker was its only
// content) and its parent blockquote is tagged with alertAttrKey so the
// renderer can emit the panel macro instead of a plain <blockquote>.
type alertParagraphTransformer struct{}

func (t *alertParagraphTransformer) Transform(para *ast.Paragraph, reader text.Reader, _ parser.Context) {
	bq, ok := para.Parent().(*ast.Blockquote)
	if !ok {
		return
	}
	if bq.FirstChild() != para {
		return
	}
	lines := para.Lines()
	if lines.Len() == 0 {
		return
	}
	source := reader.Source()
	first := lines.At(0)
	firstRaw := source[first.Start:first.Stop]
	// A "line" segment may span multiple source lines when goldmark
	// joins consecutive content lines into one segment. The alert
	// marker (if present) must occupy only the first source line, so
	// detect it against the bytes up to the first newline.
	markerEnd := len(firstRaw)
	if nl := strings.IndexByte(string(firstRaw), '\n'); nl >= 0 {
		markerEnd = nl + 1 // include the trailing newline in the marker span
	}
	level, ok := parseAlertMarker(strings.TrimSpace(string(firstRaw[:markerEnd])))
	if !ok {
		return
	}

	bq.SetAttributeString(alertAttrKey, []byte(level))

	trimmed := text.NewSegments()
	if markerEnd < len(firstRaw) {
		trimmed.Append(text.NewSegment(first.Start+markerEnd, first.Stop))
	}
	for i := 1; i < lines.Len(); i++ {
		trimmed.Append(lines.At(i))
	}
	if trimmed.Len() == 0 {
		bq.RemoveChild(bq, para)
		return
	}
	para.SetLines(trimmed)
}

func parseAlertMarker(line string) (AlertLevel, bool) {
	if !strings.HasPrefix(line, "[!") || !strings.HasSuffix(line, "]") {
		return "", false
	}
	name := strings.ToUpper(strings.TrimSuffix(strings.TrimPrefix(line, "[!"), "]"))
	switch AlertLevel(name) {
	case AlertNote, AlertTip, AlertImportant, AlertWarning, AlertCaution:
		return AlertLevel(name), true
	}
	return "", false
}
