package transforms

import (
	"strings"
	"testing"
)

func TestPreserveMacrosProfileOverrides(t *testing.T) {
	p, err := LoadProfile([]byte("page:\n  preserve_macros: true\npages:\n  - id: 42\n    preserve_macros: false\n"))
	if err != nil {
		t.Fatal(err)
	}
	cfg, _, _ := p.ResolvePageConfig(42, "")
	if cfg.PreserveMacros {
		t.Fatal("per-page override ignored")
	}
	cfg, _, _ = p.ResolvePageConfig(1, "")
	if !cfg.PreserveMacros {
		t.Fatal("global preservation ignored")
	}
	if err := ApplySetOverrides(p, map[string]string{"page.preserve_macros": "false"}); err != nil {
		t.Fatal(err)
	}
	if p.Page.PreserveMacros {
		t.Fatal("--set ignored")
	}
}

func TestInlineTransclusionsProfileOverrides(t *testing.T) {
	p, err := LoadProfile([]byte("page:\n  inline_transclusions: true\npages:\n  - id: 42\n    inline_transclusions: false\n"))
	if err != nil {
		t.Fatal(err)
	}
	cfg, _, _ := p.ResolvePageConfig(42, "")
	if cfg.InlineTransclusions {
		t.Fatal("per-page override ignored")
	}
	cfg, _, _ = p.ResolvePageConfig(1, "")
	if !cfg.InlineTransclusions {
		t.Fatal("global inline setting ignored")
	}
	if err := ApplySetOverrides(p, map[string]string{"page.inline_transclusions": "false"}); err != nil {
		t.Fatal(err)
	}
	if p.Page.InlineTransclusions {
		t.Fatal("--set ignored")
	}
}

func TestInvalidImportTransformAndURL(t *testing.T) {
	for _, source := range []string{
		"kind: import\ntransforms:\n  - type: unknown\n",
		"kind: import\ntransforms:\n  - type: restore_macro\n    params: {start_pattern: invalid}\n",
		"kind: import\ngit_files: {mode: url}\n",
		"kind: import\ngit_files: {mode: url, url: 'file:///C:/_git/repo'}\n",
	} {
		if _, err := LoadImportProfile([]byte(source)); err == nil {
			t.Errorf("invalid profile accepted: %s", source)
		}
	}
}

func TestURLPerExtensionInTableAndExternalSchemes(t *testing.T) {
	r := &RewriteGitFileLinks{Mode: "link", Macro: "view-git-file", Parameters: map[string]string{"path": "{path}"}, URL: "https://tfs.test/C/P/_git/R", PerExtension: map[string]string{"yaml": "url"}}
	ctx := &TransformContext{PreContent: "| File |\n| --- |\n| [File](config.yaml) |\n\n[Data](data:text/plain,file.yaml)"}
	if err := r.Apply(ctx); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ctx.PreContent, "path=%2Fconfig.yaml") || !strings.Contains(ctx.PreContent, "[Data](data:text/plain,file.yaml)") {
		t.Fatalf("mixed URL mode: %s", ctx.PreContent)
	}
}
