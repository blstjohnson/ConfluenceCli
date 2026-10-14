package converters

import (
	"strings"
	"testing"

	"confcli/pkg/converters/md"
	"golang.org/x/net/html"
)

func TestExportKeepsTableBoundaryBeforeResumedList(t *testing.T) {
	source := `<ol><li>Check request</li><li>Add details:</li></ol><table><tr><th>Field</th><th>Value</th></tr><tr><td>bankId</td><td>Identifier</td></tr></table><ol start="3"><li>Find receiver<ul><li>Use identifier</li></ul></li><li>Send response</li></ol>`
	markdown, err := StorageToMarkdownAdvanced(source, "")
	if err != nil {
		t.Fatal(err)
	}
	storage, err := md.ToStorage([]byte(markdown))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(markdown, "\n   | Field") {
		t.Fatalf("table not nested in its list item:\n%s", markdown)
	}
	assertResumedListOutsideTable(t, storage)
}

func assertResumedListOutsideTable(t *testing.T, storage string) {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(storage))
	if err != nil {
		t.Fatal(err)
	}
	rows, lists, outerItems := 0, 0, 0
	var walk func(*html.Node, bool)
	walk = func(n *html.Node, inTable bool) {
		inTable = inTable || n.Data == "table"
		if n.Data == "tr" {
			rows++
		}
		if n.Data == "ol" {
			lists++
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Data == "li" {
					outerItems++
				}
			}
		}
		if n.Data == "table" && n.Parent.Data != "li" {
			t.Fatalf("table must remain inside its preceding list item: %s", storage)
		}
		if n.Type == html.TextNode && (strings.Contains(n.Data, "Find receiver") || strings.Contains(n.Data, "Use identifier") || strings.Contains(n.Data, "Send response")) && inTable {
			t.Fatalf("list text became a table cell: %s", storage)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c, inTable)
		}
	}
	walk(doc, false)
	if rows != 2 || lists != 1 || outerItems != 4 || strings.Contains(storage, `<ol start="3">`) {
		t.Fatalf("expected two table rows within one continuous four-item list: %s", storage)
	}
}

func TestImportSeparatesUnindentedTableFromResumedList(t *testing.T) {
	markdown := "1. Check request\n2. Add details:\n\n| Field | Value |\n| --- | --- |\n| bankId | Identifier |\n3. Find receiver\n    - Use identifier\n4. Send response\n"
	storage, err := md.ToStorage([]byte(markdown))
	if err != nil {
		t.Fatal(err)
	}
	assertResumedListOutsideTable(t, storage)
}

func TestTablesInConditionalBulletsKeepOuterNumbering(t *testing.T) {
	markdown := "1. Prepare\n2. Conditions:\n   - Add details:\n\n| Field | Value |\n| --- | --- |\n| id | value |\n\n   - Compute:\n     - If available:\n\n| Field | Value |\n| --- | --- |\n| amount | value |\n\n   - Finish conditions\n3. Send\n4. Complete\n"
	storage, err := md.ToStorage([]byte(markdown))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := html.Parse(strings.NewReader(storage))
	if err != nil {
		t.Fatal(err)
	}
	lists, tables, outerItems := 0, 0, 0
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Data == "ol" {
			lists++
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Data == "li" {
					outerItems++
				}
			}
		}
		if n.Data == "table" {
			tables++
			if n.Parent.Data != "li" {
				t.Fatalf("conditional table escaped its item: %s", storage)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	if lists != 1 || tables != 2 || outerItems != 4 {
		t.Fatalf("conditional lists broke the outer sequence: %s", storage)
	}
}
