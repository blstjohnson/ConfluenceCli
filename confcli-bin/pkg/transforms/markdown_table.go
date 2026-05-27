package transforms

import (
	"regexp"
	"strings"
)

// tableSeparatorRe matches a GFM table separator row such as
// `|---|---|`, `| :--- | ---: |`, or `--- | :---:`. At least one
// dash run is required; pipes are optional at the row edges.
var tableSeparatorRe = regexp.MustCompile(
	`^\s*\|?\s*:?-{3,}:?\s*(\|\s*:?-{3,}:?\s*)+\|?\s*$`,
)

// markTableLines returns a per-line bool slice marking which lines
// belong to a GFM table (header row + separator + contiguous body
// rows). Lines inside fenced code blocks are ignored. The result
// always has the same length as lines.
//
// The header row is identified by the line *preceding* a separator
// row (the standard GFM shape). Body rows continue until a blank
// line or a line without a pipe.
func markTableLines(lines []string) []bool {
	inTable := make([]bool, len(lines))
	inFence := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if !tableSeparatorRe.MatchString(line) {
			continue
		}
		// Found a separator. The line above is the header row only
		// if it contains a pipe (GFM requires at least one column).
		if i > 0 && strings.Contains(lines[i-1], "|") {
			inTable[i-1] = true
		}
		inTable[i] = true
		// Body rows: contiguous, non-blank, contain a pipe.
		for j := i + 1; j < len(lines); j++ {
			next := lines[j]
			if strings.TrimSpace(next) == "" || !strings.Contains(next, "|") {
				break
			}
			inTable[j] = true
		}
	}
	return inTable
}
