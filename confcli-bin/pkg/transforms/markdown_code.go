package transforms

import "confcli/pkg/markdownutil"

// MarkdownOutsideCode applies a rewrite only outside code spans and blocks.
func MarkdownOutsideCode(source string, rewrite func(string) string) string {
	return markdownutil.MarkdownOutsideCode(source, rewrite)
}

func protectMarkdownCode(source string) (string, func(string) string) {
	return markdownutil.ProtectCode(source)
}
