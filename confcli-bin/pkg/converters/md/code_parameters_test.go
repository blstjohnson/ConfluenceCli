package md

import (
	"strings"
	"testing"
)

func TestCodeParametersRejectMalformedMetadataAndPreserveLiteralAnnotations(t *testing.T) {
	if _, err := ToStorage([]byte("<!-- confcli-code-params: {broken} -->\n\n```\nexample\n```")); err == nil {
		t.Fatal("malformed code metadata accepted")
	}
	if _, err := ToStorage([]byte(`<pre data-confcli-code-params="invalid!"><code>one</code></pre>`)); err == nil {
		t.Fatal("malformed cell metadata accepted")
	}
	literal := `<pre data-confcli-code-params="invalid!"><code>one</code></pre>`
	storage, err := ToStorage([]byte("```\n" + literal + "\n```"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(storage, literal) {
		t.Fatal("annotation inside code was interpreted")
	}
}
