package md

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"sort"
	"strings"

	"github.com/yuin/goldmark/ast"
)

const codeParamsPrefix = "<!-- confcli-code-params: "

func precedingCodeParameters(block *ast.FencedCodeBlock, source []byte) (map[string]string, error) {
	previous, ok := block.PreviousSibling().(*ast.HTMLBlock)
	if !ok {
		return nil, nil
	}
	marker := strings.TrimSpace(string(previous.Lines().Value(source)))
	if !strings.HasPrefix(marker, codeParamsPrefix) {
		return nil, nil
	}
	if !strings.HasSuffix(marker, " -->") {
		return nil, fmt.Errorf("unclosed confcli code parameters")
	}
	return decodeCodeParameters([]byte(strings.TrimSuffix(strings.TrimPrefix(marker, codeParamsPrefix), " -->")))
}

func decodeCodeParameters(data []byte) (map[string]string, error) {
	var params map[string]string
	if err := json.Unmarshal(data, &params); err != nil {
		return nil, fmt.Errorf("invalid confcli code parameters: %w", err)
	}
	if params == nil {
		return nil, fmt.Errorf("confcli code parameters must be an object")
	}
	return params, nil
}

func writeCodeParameters(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, key := range keys {
		b.WriteString(`<ac:parameter ac:name="` + html.EscapeString(key) + `">` + html.EscapeString(params[key]) + `</ac:parameter>`)
	}
	return b.String()
}

var annotatedCodeCell = regexp.MustCompile(`<pre data-confcli-code-params="([^"]*)"><code>([^<]*)</code></pre>`)

// Restore code cells exported as a single HTML line so a multiline example
// cannot split a pipe table. Only our explicit annotation opts into a macro.
func restoreCodeCells(storage string) (string, error) {
	var failure error
	rewrite := func(segment string) string {
		return annotatedCodeCell.ReplaceAllStringFunc(segment, func(cell string) string {
			m := annotatedCodeCell.FindStringSubmatch(cell)
			data, err := base64.StdEncoding.DecodeString(m[1])
			if err != nil {
				failure = fmt.Errorf("invalid confcli code cell parameters: %w", err)
				return cell
			}
			params, err := decodeCodeParameters(data)
			if err != nil {
				failure = err
				return cell
			}
			body := strings.ReplaceAll(html.UnescapeString(m[2]), "]]>", "]]]]><![CDATA[>")
			return `<ac:structured-macro ac:name="code">` + writeCodeParameters(params) + `<ac:plain-text-body><![CDATA[` + body + `]]></ac:plain-text-body></ac:structured-macro>`
		})
	}
	var result strings.Builder
	for {
		start := strings.Index(storage, "<![CDATA[")
		if start < 0 {
			result.WriteString(rewrite(storage))
			break
		}
		result.WriteString(rewrite(storage[:start]))
		end := strings.Index(storage[start:], "]]>")
		if end < 0 {
			result.WriteString(storage[start:])
			break
		}
		end += start + 3
		result.WriteString(storage[start:end])
		storage = storage[end:]
	}
	return result.String(), failure
}
