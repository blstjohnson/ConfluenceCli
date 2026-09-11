package converters

import (
	"reflect"
	"strings"
	"testing"
)

const gitFileStorage = `<p>
<ac:structured-macro ac:name="view-git-file" ac:schema-version="1" ac:macro-id="a"><ac:parameter ac:name="path">docs/diagrams/c2bsubs/c2b_subs_management.puml</ac:parameter><ac:parameter ac:name="renderpuml">true</ac:parameter><ac:parameter ac:name="repository-id">4</ac:parameter><ac:parameter ac:name="branch">refs/remotes/origin/develop</ac:parameter></ac:structured-macro></p>
<p><ac:structured-macro ac:name="view-git-file" ac:schema-version="1" ac:macro-id="b"><ac:parameter ac:name="path">APIReferences/QRCode-Service.yaml</ac:parameter><ac:parameter ac:name="repository-id">4</ac:parameter><ac:parameter ac:name="branch">refs/remotes/origin/develop</ac:parameter></ac:structured-macro></p>
<p><ac:structured-macro ac:name="view-git-file" ac:schema-version="1" ac:macro-id="c"><ac:parameter ac:name="path">docs/diagrams/c2bsubs/c2b_subs_payment.puml</ac:parameter><ac:parameter ac:name="renderpuml">true</ac:parameter><ac:parameter ac:name="repository-id">4</ac:parameter><ac:parameter ac:name="renderpanel">true</ac:parameter><ac:parameter ac:name="branch">refs/remotes/origin/develop</ac:parameter></ac:structured-macro></p>`

func TestExtractGitFileRefs(t *testing.T) {
	got := ExtractGitFileRefs(gitFileStorage)
	want := []GitFileRef{
		{Path: "docs/diagrams/c2bsubs/c2b_subs_management.puml", Branch: "refs/remotes/origin/develop", RepositoryID: "4"},
		{Path: "docs/diagrams/c2bsubs/c2b_subs_payment.puml", Branch: "refs/remotes/origin/develop", RepositoryID: "4"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ExtractGitFileRefs mismatch\n got: %+v\nwant: %+v", got, want)
	}
}

// Panel-less image (as the plugin emits it in export_view) followed by a
// panel-wrapped one. Only the first must be rewritten.
const gitFileExportView = `<h1>Управление подписками</h1>` +
	`<p><div><ul style="list-style-type: none; padding-left: 0"><li><img src="/download/export/git-for-confluence2001406496354065099png"></li></ul></div></p>` +
	`<h1>Осуществление платежа</h1>` +
	`<div class="git-plugin-container"><div class="git-plugin-metadata"><div class="git-plugin-left-content"><strong>c2b_subs_payment.puml</strong></div>` +
	`<aui-inline-dialog><table><tr><th>Full path</th><td><a href="ssh://tfs.example.com/Col/_git/Repodevelop/docs/diagrams/c2bsubs/c2b_subs_payment.puml">docs/diagrams/c2bsubs/c2b_subs_payment.puml</a></td></tr></table></aui-inline-dialog></div>` +
	`<div class="git-plugin-content"><div><ul style="list-style-type: none; padding-left: 0"><li><img src="/download/export/git-for-confluence17798025296591167019png"></li></ul></div></div></div>`

func TestInjectGitFileRefs(t *testing.T) {
	refs := ExtractGitFileRefs(gitFileStorage)
	out := InjectGitFileRefs(gitFileExportView, refs)

	if !strings.Contains(out, `<div class="git-plugin-container"><strong>c2b_subs_management.puml</strong><a href="docs/diagrams/c2bsubs/c2b_subs_management.puml">docs/diagrams/c2bsubs/c2b_subs_management.puml</a></div>`) {
		t.Errorf("panel-less image not replaced with link, got:\n%s", out)
	}
	if strings.Contains(out, "git-for-confluence2001406496354065099png") {
		t.Errorf("panel-less image wrapper should be removed, got:\n%s", out)
	}
	if !strings.Contains(out, "git-for-confluence17798025296591167019png") {
		t.Errorf("panel-wrapped image must be left untouched, got:\n%s", out)
	}
	if strings.Contains(out, `<a href="docs/diagrams/c2bsubs/c2b_subs_payment.puml">`) {
		t.Errorf("panel-wrapped macro must not get an injected link (panel already has one), got:\n%s", out)
	}
	if CountGitFileImages(out) != 1 {
		t.Errorf("expected exactly one git image left, got %d", CountGitFileImages(out))
	}
}

func TestInjectGitFileRefs_NoRefsOrNoImages(t *testing.T) {
	if got := InjectGitFileRefs(gitFileExportView, nil); got != gitFileExportView {
		t.Errorf("nil refs must leave HTML unchanged")
	}
	plain := `<p>no diagrams here</p>`
	if got := InjectGitFileRefs(plain, ExtractGitFileRefs(gitFileStorage)); got != plain {
		t.Errorf("HTML without git images must be unchanged")
	}
}

func TestInjectGitFileRefs_FewerRefsThanImages(t *testing.T) {
	two := `<p><div><ul><li><img src="/download/export/git-for-confluence1png"></li></ul></div></p>` +
		`<p><div><ul><li><img src="/download/export/git-for-confluence2png"></li></ul></div></p>`
	out := InjectGitFileRefs(two, []GitFileRef{{Path: "a/one.puml"}})
	if !strings.Contains(out, `<strong>one.puml</strong><a href="a/one.puml">a/one.puml</a>`) {
		t.Errorf("first image should be replaced, got:\n%s", out)
	}
	if !strings.Contains(out, "git-for-confluence2png") {
		t.Errorf("unpaired second image must be kept as-is, got:\n%s", out)
	}
}

// End to end: the injected HTML must survive export_view→markdown conversion
// as a bold link, and the empty "- " bullet that the bare wrapper used to
// leave behind must be gone.
func TestExportViewToMarkdown_GitFileLink(t *testing.T) {
	refs := ExtractGitFileRefs(gitFileStorage)
	md, err := ExportViewToMarkdown(InjectGitFileRefs(gitFileExportView, refs), "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(md, "**[c2b_subs_management.puml](docs/diagrams/c2bsubs/c2b_subs_management.puml)**") {
		t.Errorf("expected bold link for panel-less diagram, got:\n%s", md)
	}
	if !strings.Contains(md, "**[c2b_subs_payment.puml](ssh://tfs.example.com/Col/_git/Repodevelop/docs/diagrams/c2bsubs/c2b_subs_payment.puml)**") {
		t.Errorf("expected panel link preserved, got:\n%s", md)
	}
	for _, line := range strings.Split(md, "\n") {
		if strings.TrimSpace(line) == "-" {
			t.Errorf("empty bullet left behind by stripped git image:\n%s", md)
			break
		}
	}
	if strings.Contains(md, "git-for-confluence") {
		t.Errorf("rendered git image URL must not leak into markdown:\n%s", md)
	}
}
