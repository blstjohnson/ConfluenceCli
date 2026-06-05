package transforms

import "testing"

func TestExpandBranchRef(t *testing.T) {
	cases := []struct {
		name   string
		branch string
		mode   string
		want   string
	}{
		{"remote expands bare", "feature/C2B", "remote", "refs/remotes/origin/feature/C2B"},
		{"origin alias", "feature/C2B", "origin", "refs/remotes/origin/feature/C2B"},
		{"local expands bare", "feature/C2B", "local", "refs/heads/feature/C2B"},
		{"short is no-op", "feature/C2B", "short", "feature/C2B"},
		{"empty mode is no-op", "feature/C2B", "", "feature/C2B"},
		{"unknown mode is no-op", "feature/C2B", "weird", "feature/C2B"},
		{"already full ref untouched", "refs/remotes/origin/x", "remote", "refs/remotes/origin/x"},
		{"empty branch untouched", "", "remote", ""},
		{"idempotent", "refs/heads/x", "local", "refs/heads/x"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ExpandBranchRef(tc.branch, tc.mode); got != tc.want {
				t.Errorf("ExpandBranchRef(%q, %q) = %q, want %q", tc.branch, tc.mode, got, tc.want)
			}
		})
	}
}

func TestLoadImportProfile_BranchRefValidation(t *testing.T) {
	base := "kind: import\nplantuml:\n  macro: view-git-file\n"

	if _, err := LoadImportProfile([]byte(base + "  branch_ref: remote\n")); err != nil {
		t.Errorf("valid branch_ref should load: %v", err)
	}
	if _, err := LoadImportProfile([]byte(base + "  branch_ref: bogus\n")); err == nil {
		t.Errorf("invalid branch_ref must be rejected")
	}

	gf := "kind: import\ngit_files:\n  macro: view-git-file\n  branch_ref: nonsense\n"
	if _, err := LoadImportProfile([]byte(gf)); err == nil {
		t.Errorf("invalid git_files.branch_ref must be rejected")
	}
}
