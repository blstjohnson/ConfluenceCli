package converters

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"confcli/pkg/markdownutil"
)

// RoundTripPage supplies the real owner context for links, images and nested
// transclusions. Included content is a snapshot inside the consumer document,
// never a synthetic page in the export tree.
type RoundTripPage struct {
	PageID                               int
	SpaceKey, Title, Storage, ExportView string
}

type transclusionMetadata struct {
	Macro      string            `json:"macro"`
	Space      string            `json:"space,omitempty"`
	Title      string            `json:"title,omitempty"`
	PageID     int               `json:"page_id,omitempty"`
	SHA256     string            `json:"sha256,omitempty"`
	Parameters map[string]string `json:"parameters,omitempty"`
}

func transclusionStart(kind string, meta transclusionMetadata) string {
	b, _ := json.Marshal(meta)
	return "<!-- confcli-" + kind + ":start " + base64.StdEncoding.EncodeToString(b) + " -->"
}

func inlineTransclusion(n *storageNode, opts RoundTripOptions) (string, error) {
	if opts.FetchPage == nil || opts.ResolvePage == nil {
		return "", fmt.Errorf("inline transclusions require a page fetcher and resolver")
	}
	ref := n.find("ri:page")
	if ref == nil {
		return "", fmt.Errorf("%s has no ri:page", n.attr("ac:name"))
	}
	title, refSpace := ref.attr("ri:content-title"), ref.attr("ri:space-key")
	space := refSpace
	if space == "" {
		space = opts.SpaceKey
	}
	id, err := strconv.Atoi(ref.attr("ri:content-id"))
	if err != nil || id <= 0 {
		id, err = opts.ResolvePage(space, title)
	}
	if err != nil {
		return "", err
	}
	if id <= 0 {
		return "", fmt.Errorf("included page %q in %q not found", title, space)
	}
	stack := append(append([]int{}, opts.transclusionStack...), opts.PageID)
	for _, ancestor := range stack {
		if ancestor == id {
			return "", fmt.Errorf("cyclic transclusion of page %d (%s)", id, title)
		}
	}
	if len(stack) >= 16 {
		return "", fmt.Errorf("transclusion depth exceeds 16 at page %d", id)
	}
	page, err := opts.FetchPage(id)
	if err != nil {
		return "", fmt.Errorf("fetch included page %d: %w", id, err)
	}
	if title == "" {
		title = page.Title
	}
	storage := page.Storage
	name := n.attr("ac:name")
	if name == "excerpt-include" {
		if n.params()["name"] != "" {
			return "", fmt.Errorf("named excerpt on page %d requires explicit support", id)
		}
		var found bool
		storage, found, err = StorageExcerptBody(storage)
		if err != nil {
			return "", err
		}
		if !found {
			return "", fmt.Errorf("included page %d has no excerpt", id)
		}
	}
	child := opts
	child.PageID, child.SpaceKey = id, page.SpaceKey
	if child.SpaceKey == "" {
		child.SpaceKey = space
	}
	child.transclusionStack = stack
	prepared, restore, err := PrepareRoundTripExport(storage, page.ExportView, child)
	if err != nil {
		return "", err
	}
	body, err := StorageToMarkdownAdvanced(prepared, opts.BaseURL)
	if err != nil {
		return "", err
	}
	body = restore(body)
	params := n.params()
	delete(params, "")
	meta := transclusionMetadata{Macro: name, Title: title, Space: refSpace, PageID: id, Parameters: params}
	return transclusionStart("transclusion", meta) + "\n\n" + strings.TrimSpace(body) + "\n\n<!-- confcli-transclusion:end -->", nil
}

type transclusionSpan struct {
	start, bodyStart, bodyEnd, end int
	kind                           string
	meta                           transclusionMetadata
	children                       []*transclusionSpan
}

var transclusionMarkerRE = regexp.MustCompile(`<!-- confcli-(transclusion|excerpt):(start(?: [A-Za-z0-9+/=]+)?|end) -->`)

func transclusionSpans(source string) ([]*transclusionSpan, error) {
	masked := markdownutil.MaskCode(source)
	var roots, stack []*transclusionSpan
	for _, loc := range transclusionMarkerRE.FindAllStringSubmatchIndex(masked, -1) {
		kind, directive := masked[loc[2]:loc[3]], masked[loc[4]:loc[5]]
		if directive == "end" {
			if len(stack) == 0 || stack[len(stack)-1].kind != kind {
				return nil, fmt.Errorf("unmatched confcli-%s end marker", kind)
			}
			top := stack[len(stack)-1]
			top.bodyEnd, top.end = loc[0], loc[1]
			stack = stack[:len(stack)-1]
			continue
		}
		payload := strings.TrimPrefix(directive, "start ")
		data, err := base64.StdEncoding.DecodeString(payload)
		if err != nil {
			return nil, fmt.Errorf("invalid confcli-%s metadata: %w", kind, err)
		}
		var meta transclusionMetadata
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&meta); err != nil {
			return nil, fmt.Errorf("invalid confcli-%s metadata: %w", kind, err)
		}
		if (kind == "excerpt" && meta.Macro != "excerpt") ||
			(kind == "transclusion" && (meta.Title == "" || (meta.Macro != "include" && meta.Macro != "excerpt-include"))) {
			return nil, fmt.Errorf("invalid confcli-%s macro or title", kind)
		}
		s := &transclusionSpan{start: loc[0], bodyStart: loc[1], kind: kind, meta: meta}
		if len(stack) == 0 {
			roots = append(roots, s)
		} else {
			top := stack[len(stack)-1]
			top.children = append(top.children, s)
		}
		stack = append(stack, s)
	}
	if len(stack) != 0 {
		return nil, fmt.Errorf("unclosed confcli-%s marker", stack[len(stack)-1].kind)
	}
	return roots, nil
}

func transclusionHash(s string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n")))))
}

// SealTransclusions runs after all export transforms and link/asset rewrites.
// The checksum prevents sync from silently discarding an edited snapshot while
// restoring a live Include macro. Own excerpt bodies remain editable.
func SealTransclusions(source string) (string, error) {
	spans, err := transclusionSpans(source)
	if err != nil {
		return "", err
	}
	var seal func(string, int, []*transclusionSpan) string
	seal = func(segment string, offset int, children []*transclusionSpan) string {
		for i := len(children) - 1; i >= 0; i-- {
			s := children[i]
			body := seal(source[s.bodyStart:s.bodyEnd], s.bodyStart, s.children)
			meta := s.meta
			if s.kind == "transclusion" {
				meta.SHA256 = transclusionHash(body)
			}
			replacement := transclusionStart(s.kind, meta) + body + source[s.bodyEnd:s.end]
			segment = segment[:s.start-offset] + replacement + segment[s.end-offset:]
		}
		return segment
	}
	return seal(source, 0, spans), nil
}

func macroParameters(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(`<ac:parameter ac:name="` + html.EscapeString(k) + `">` + html.EscapeString(params[k]) + "</ac:parameter>")
	}
	return b.String()
}

// RestoreTransclusions runs before import transforms, removing unchanged
// snapshots and restoring their original live macros after Markdown conversion.
func RestoreTransclusions(source string) (string, func(string) string, error) {
	spans, err := transclusionSpans(source)
	if err != nil {
		return "", nil, err
	}
	prefix := fmt.Sprintf("confcli-transclusion-restore-%x", sha256.Sum256([]byte(source)))
	for strings.Contains(source, prefix) {
		prefix += "x"
	}
	originals := map[string]string{}
	stash := func(xml string) string {
		token := fmt.Sprintf("<!--%s-%d-->", prefix, len(originals))
		originals[token] = xml
		return token
	}
	var restore func(string, int, []*transclusionSpan) (string, error)
	restore = func(segment string, offset int, children []*transclusionSpan) (string, error) {
		for i := len(children) - 1; i >= 0; i-- {
			s := children[i]
			meta, body := s.meta, source[s.bodyStart:s.bodyEnd]
			open := `<ac:structured-macro ac:name="` + meta.Macro + `">` + macroParameters(meta.Parameters)
			replacement := ""
			if s.kind == "transclusion" {
				if meta.SHA256 == "" || transclusionHash(body) != meta.SHA256 {
					return "", fmt.Errorf("included snapshot of %q (page %d) was edited or is unsealed; update its source page or explicitly remove the transclusion markers to materialize it", meta.Title, meta.PageID)
				}
				spaceAttr := ""
				if meta.Space != "" {
					spaceAttr = ` ri:space-key="` + html.EscapeString(meta.Space) + `"`
				}
				xml := open + `<ac:parameter ac:name=""><ac:link><ri:page ri:content-title="` + html.EscapeString(meta.Title) + `"` + spaceAttr + ` /></ac:link></ac:parameter></ac:structured-macro>`
				replacement = stash(xml)
			} else {
				inner, err := restore(body, s.bodyStart, s.children)
				if err != nil {
					return "", err
				}
				replacement = stash(open+"<ac:rich-text-body>") + "\n\n" + strings.TrimSpace(inner) + "\n\n" + stash("</ac:rich-text-body></ac:structured-macro>")
			}
			segment = segment[:s.start-offset] + replacement + segment[s.end-offset:]
		}
		return segment, nil
	}
	out, err := restore(source, 0, spans)
	if err != nil {
		return "", nil, err
	}
	return out, func(storage string) string {
		for token, xml := range originals {
			storage = strings.ReplaceAll(storage, token, xml)
		}
		return storage
	}, nil
}
