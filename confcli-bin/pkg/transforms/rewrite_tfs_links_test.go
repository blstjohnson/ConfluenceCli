package transforms

import "testing"

func TestRewriteTFSLinksBasic(t *testing.T) {
	r := NewRewriteTFSLinks("https://tfs.ekassir.com", "", "")

	ctx := &TransformContext{
		PostContent: `See [config](https://tfs.ekassir.com/Collection/Project/_git/Repo/path/to/config.yaml)`,
	}
	if err := r.Apply(ctx); err != nil {
		t.Fatal(err)
	}

	expected := `See [config](path/to/config.yaml)`
	if ctx.PostContent != expected {
		t.Errorf("expected %q, got %q", expected, ctx.PostContent)
	}
}

func TestRewriteTFSLinksQueryPath(t *testing.T) {
	r := NewRewriteTFSLinks("https://tfs.ekassir.com", "", "")

	ctx := &TransformContext{
		PostContent: `[file](https://tfs.ekassir.com/Col/Proj/_git/Repo?path=/src/main.go&version=GBmain)`,
	}
	if err := r.Apply(ctx); err != nil {
		t.Fatal(err)
	}

	expected := `[file](src/main.go)`
	if ctx.PostContent != expected {
		t.Errorf("expected %q, got %q", expected, ctx.PostContent)
	}
}

func TestRewriteTFSLinksWithLocalRepo(t *testing.T) {
	r := NewRewriteTFSLinks("https://tfs.ekassir.com", "repo", "")

	ctx := &TransformContext{
		PostContent: `[file](https://tfs.ekassir.com/Col/Proj/_git/Repo/src/main.go)`,
	}
	if err := r.Apply(ctx); err != nil {
		t.Fatal(err)
	}

	expected := `[file](repo/src/main.go)`
	if ctx.PostContent != expected {
		t.Errorf("expected %q, got %q", expected, ctx.PostContent)
	}
}

func TestRewriteTFSLinksSkipsNonTFS(t *testing.T) {
	r := NewRewriteTFSLinks("https://tfs.ekassir.com", "", "")

	content := `[link](https://github.com/org/repo)`
	ctx := &TransformContext{PostContent: content}
	if err := r.Apply(ctx); err != nil {
		t.Fatal(err)
	}

	if ctx.PostContent != content {
		t.Errorf("should not modify non-TFS links")
	}
}

func TestRewriteTFSLinksSkipsWorkItems(t *testing.T) {
	r := NewRewriteTFSLinks("https://tfs.ekassir.com", "", "")

	content := `[item](https://tfs.ekassir.com/Col/Proj/_workitems/edit/123)`
	ctx := &TransformContext{PostContent: content}
	if err := r.Apply(ctx); err != nil {
		t.Fatal(err)
	}

	if ctx.PostContent != content {
		t.Errorf("should not modify work item links")
	}
}

func TestRewriteTFSLinksName(t *testing.T) {
	r := NewRewriteTFSLinks("", "", "")
	if r.Name() != "rewrite/tfs-links" {
		t.Errorf("expected 'rewrite/tfs-links', got %q", r.Name())
	}
}
