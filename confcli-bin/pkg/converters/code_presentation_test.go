package converters

import (
	"strings"
	"testing"

	"confcli/pkg/converters/md"
)

func TestCollapsedCodeParametersAndTableSurviveRoundTrip(t *testing.T) {
	source := `<ol><li><p>Example</p><ac:structured-macro ac:name="code"><ac:parameter ac:name="title">Пример --&gt; &amp; проверка</ac:parameter><ac:parameter ac:name="collapse">true</ac:parameter><ac:parameter ac:name="language">json</ac:parameter><ac:plain-text-body><![CDATA[{"one": 1,
"two": "]]>", "pipe": "|"}]]></ac:plain-text-body></ac:structured-macro><p>After list example</p></li></ol><table><tr><th>Setting</th><th>Example</th></tr><tr><td>KeySource.Path</td><td><ac:structured-macro ac:name="code"><ac:parameter ac:name="title">Пример</ac:parameter><ac:parameter ac:name="collapse">true</ac:parameter><ac:plain-text-body><![CDATA[{"kid": "one",
"key": "a|b"}]]></ac:plain-text-body></ac:structured-macro></td></tr><tr><td>Next</td><td>Kept</td></tr></table><h2>Требования к журналам</h2><p>End retained</p>`
	// Split the literal CDATA terminator in the first example, as Confluence does.
	source = strings.Replace(source, `"two": "]]>",`, `"two": "]]]]><![CDATA[>",`, 1)
	prepared, restore, err := PrepareRoundTripExport(source, "", RoundTripOptions{})
	if err != nil {
		t.Fatal(err)
	}
	markdown, err := StorageToMarkdownAdvanced(prepared, "")
	if err != nil {
		t.Fatal(err)
	}
	markdown = restore(markdown)
	storage, err := md.ToStorage([]byte(markdown))
	if err != nil {
		t.Fatal(err)
	}
	for _, wanted := range []string{`<ac:parameter ac:name="collapse">true</ac:parameter>`, `<ac:parameter ac:name="title">Пример`, `<ac:parameter ac:name="language">json</ac:parameter>`, `"key": "a|b"`, `KeySource.Path`, `<td>Next</td>`, `Требования к журналам`, `End retained`} {
		if !strings.Contains(storage, wanted) {
			t.Fatalf("lost %q:\n%s\n%s", wanted, markdown, storage)
		}
	}
	if strings.Count(storage, `<ac:structured-macro ac:name="code">`) != 2 || strings.Count(storage, `<ac:parameter ac:name="collapse">true</ac:parameter>`) != 2 {
		t.Fatalf("code presentation not restored: %s", storage)
	}
	if strings.Contains(storage, "data-confcli-code-params") {
		t.Fatalf("HTML cell annotation was not consumed: %s", storage)
	}
}

func TestNestedOrderedListRetainsAlphabeticMarkers(t *testing.T) {
	source := `<ol start="5"><li><p>Decode</p></li><li><p>Check JOSE</p><ol><li><p>b64<br/>Kept</p></li><li><p>x5t#S256</p></li><li><p>crit</p></li><li><p>sigT</p></li><li><p>sigD</p></li><li><p>alg</p></li></ol></li><li><p>Continue</p></li></ol>`
	prepared, restore, err := PrepareRoundTripExport(source, "", RoundTripOptions{})
	if err != nil {
		t.Fatal(err)
	}
	markdown, err := StorageToMarkdownAdvanced(prepared, "")
	if err != nil {
		t.Fatal(err)
	}
	markdown = restore(markdown)
	storage, err := md.ToStorage([]byte(markdown))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(markdown, `<ol type="a">`) || !strings.Contains(storage, `<ol type="a">`) || !strings.Contains(storage, `start="5"`) || !strings.Contains(storage, `Continue`) {
		t.Fatalf("nested numbering lost: %s\n%s", markdown, storage)
	}
	if strings.Contains(storage, `</br>`) || !strings.Contains(storage, `<br />`) {
		t.Fatalf("invalid XHTML line break in retained list: %s", storage)
	}
}
