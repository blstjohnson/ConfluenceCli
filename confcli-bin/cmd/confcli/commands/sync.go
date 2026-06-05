package commands

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"confcli/pkg/clients"
	"confcli/pkg/sync"
	"confcli/pkg/transforms"
)

// NewSyncCmd builds the `confcli sync` command — a single-run, opinionated
// uploader that pushes a local markdown tree to a Confluence page tree.
func NewSyncCmd() *cobra.Command {
	var (
		profileName string
		fromDir     string
		spaceKey    string
		rootID      int
		dryRun      bool
	)

	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Sync a local markdown tree to a Confluence page tree",
		Long: `Sync the markdown files under --from into the Confluence space --space, rooted at --root.

The import profile controls folder→page mapping (folder_page marker, flatten/skip globs, per-path overrides). Files carrying matching confcli-id labels are updated when their content hash changes and skipped when it does not. Pages on the server that no source file maps to are reported as orphans (handled by a separate command in a future release).

With --dry-run the plan is computed and printed but no API writes are performed. The global --read-only flag implies --dry-run.

Example:
  confcli sync --profile docs --from ./docs --space ENG --root 12345 --dry-run`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if profileName == "" {
				return errors.New("--profile is required")
			}
			if fromDir == "" {
				return errors.New("--from is required")
			}
			if spaceKey == "" {
				return errors.New("--space is required")
			}
			if rootID == 0 {
				return errors.New("--root is required")
			}

			profile, err := transforms.ResolveImportProfile(profileName)
			if err != nil {
				return fmt.Errorf("load import profile: %w", err)
			}

			absFrom, err := filepath.Abs(fromDir)
			if err != nil {
				return fmt.Errorf("resolve --from: %w", err)
			}
			info, err := os.Stat(absFrom)
			if err != nil {
				return fmt.Errorf("stat --from %q: %w", absFrom, err)
			}
			if !info.IsDir() {
				return fmt.Errorf("--from %q is not a directory", absFrom)
			}
			fsys := os.DirFS(absFrom)

			client, err := clients.NewClientFromViper()
			if err != nil {
				return fmt.Errorf("create API client: %w", err)
			}

			if viper.GetBool("read_only") && !dryRun {
				fmt.Fprintln(cmd.ErrOrStderr(), "sync: --read-only is set; forcing --dry-run")
				dryRun = true
			}

			stderr := log.New(cmd.ErrOrStderr(), "", 0)

			pathMap, err := sync.BuildPathMap(profile, fsys)
			if err != nil {
				return fmt.Errorf("build path map: %w", err)
			}

			plantumlRewriter, err := buildPlantUMLRewriter(profile, absFrom, stderr)
			if err != nil {
				return fmt.Errorf("plantuml setup: %w", err)
			}

			gitFilesRewriter, err := buildGitFilesRewriter(profile, absFrom, fsys, stderr)
			if err != nil {
				return fmt.Errorf("git_files setup: %w", err)
			}

			// Resolve a repo-root filesystem so image/attachment references
			// that escape --from but stay inside the repo (../../diagrams/x.svg)
			// can still be read and uploaded. Best-effort: a non-git tree just
			// restricts resolution to --from.
			repoFS, repoRel := resolveImageRepoFS(absFrom, stderr)

			engine, err := sync.New(sync.Options{
				Profile: profile,
				Locator: sync.NewLocator(client, stderr),
				Lister:  sync.NewAPILister(client, stderr),
				Convert: sync.NewMarkdownConverter(pathMap, plantumlRewriter, gitFilesRewriter, stderr),
				Logger:  stderr,
				RepoFS:  repoFS,
				RepoRel: repoRel,
			})
			if err != nil {
				return fmt.Errorf("init engine: %w", err)
			}

			ctx := context.Background()
			plan, err := engine.BuildPlan(ctx, fsys, spaceKey, rootID)
			if err != nil {
				return fmt.Errorf("build plan: %w", err)
			}

			outcome := outcomeFromStats(plan.Stats)
			if !dryRun {
				exec, err := sync.NewExecutor(sync.ExecutorOptions{
					Client:     client,
					SpaceKey:   spaceKey,
					RootPageID: rootID,
					Logger:     stderr,
				})
				if err != nil {
					return fmt.Errorf("init executor: %w", err)
				}
				outcome = exec.Apply(ctx, plan)
			}

			printSyncReport(cmd.OutOrStdout(), plan, outcome, dryRun, viper.GetBool("debug"))

			if len(outcome.Errors) > 0 {
				return fmt.Errorf("sync completed with %d error(s)", len(outcome.Errors))
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&profileName, "profile", "", "Import profile name or path (required)")
	cmd.Flags().StringVar(&fromDir, "from", "", "Local directory to sync from (required)")
	cmd.Flags().StringVar(&spaceKey, "space", "", "Confluence space key (required)")
	cmd.Flags().IntVar(&rootID, "root", 0, "Confluence root page ID (required)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Compute and print the plan without applying it")

	return cmd
}

// buildPlantUMLRewriter assembles a RewritePlantUMLLinks from the
// import profile's plantuml section. Macro and Parameters must be set
// explicitly; branch and repo root are auto-detected from .git unless
// overridden in the profile.
//
// Returns (nil, nil) — no rewriter — when Macro or Parameters is
// empty, since the rewriter is opt-in.
func buildPlantUMLRewriter(profile *transforms.ImportProfile, absFrom string, logger *log.Logger) (*transforms.RewritePlantUMLLinks, error) {
	cfg := profile.PlantUML
	if cfg.Macro == "" || len(cfg.Parameters) == 0 {
		return nil, nil
	}
	branch, syncRel, err := resolveGitContext(absFrom, cfg.Branch, cfg.RepoRoot, "plantuml")
	if err != nil {
		return nil, err
	}
	branch = transforms.ExpandBranchRef(branch, cfg.BranchRef)
	params := sanitizeBranchParams(cfg.Parameters, cfg.BranchRef)
	warnMacroParams(logger, "plantuml", cfg.Macro, params, branch)
	return &transforms.RewritePlantUMLLinks{
		Macro:           cfg.Macro,
		Parameters:      params,
		Branch:          branch,
		SyncRootRelRepo: syncRel,
		Logger:          logger,
	}, nil
}

// sanitizeBranchParams returns a copy of params with a literal "branch"
// parameter value expanded via branch_ref. A value using the {branch}
// placeholder is left untouched — that path is expanded through the
// rewriter's already-expanded Branch field — so this only catches the
// common case of a hard-coded short branch name (branch: "feature/C2B")
// that branch_ref would otherwise miss.
func sanitizeBranchParams(params map[string]string, mode string) map[string]string {
	if len(params) == 0 {
		return params
	}
	out := make(map[string]string, len(params))
	for k, v := range params {
		if strings.EqualFold(strings.TrimSpace(k), "branch") && !strings.Contains(v, "{branch}") {
			v = transforms.ExpandBranchRef(v, mode)
		}
		out[k] = v
	}
	return out
}

// buildGitFilesRewriter is the catch-all companion to
// buildPlantUMLRewriter: it wraps non-md, non-puml file references
// (yaml, json, sql, ...) in the same view-git-file macro without the
// PlantUML renderpuml flag, so they show as source panels instead of
// turning into broken relative hrefs on the Confluence page.
//
// fsys is the sync source filesystem (rooted at --from) — only needed
// when inline mode is configured, but always passed so the rewriter can
// fall back gracefully when individual files trigger inline emission.
func buildGitFilesRewriter(profile *transforms.ImportProfile, absFrom string, fsys fs.FS, logger *log.Logger) (*transforms.RewriteGitFileLinks, error) {
	cfg := profile.GitFiles
	if cfg.Macro == "" || len(cfg.Parameters) == 0 {
		return nil, nil
	}
	branch, syncRel, err := resolveGitContext(absFrom, cfg.Branch, cfg.RepoRoot, "git_files")
	if err != nil {
		return nil, err
	}
	branch = transforms.ExpandBranchRef(branch, cfg.BranchRef)
	params := sanitizeBranchParams(cfg.Parameters, cfg.BranchRef)
	warnMacroParams(logger, "git_files", cfg.Macro, params, branch)
	return &transforms.RewriteGitFileLinks{
		Macro:           cfg.Macro,
		Parameters:      params,
		Branch:          branch,
		SyncRootRelRepo: syncRel,
		Extensions:      cfg.Extensions,
		Logger:          logger,
		Mode:            cfg.Mode,
		PerExtension:    cfg.PerExtension,
		InlineMaxBytes:  cfg.Inline.MaxBytes,
		FSys:            fsys,
	}, nil
}

// warnMacroParams logs non-fatal hints for macro parameter values that will
// almost certainly fail to render, so the user catches them before the page
// is uploaded rather than as an opaque Confluence error:
//
//   - any parameter value still containing "?" — an unfilled placeholder
//     (e.g. the repository-id "?" shipped in the example profile);
//   - a view-git-file branch that resolves to a bare short name — the plugin
//     throws "unexpected exception" unless the branch is a full ref.
//
// resolvedBranch is the post-expansion {branch} substitution value.
func warnMacroParams(logger *log.Logger, section, macro string, params map[string]string, resolvedBranch string) {
	if logger == nil {
		return
	}
	names := make([]string, 0, len(params))
	for n := range params {
		names = append(names, n)
	}
	sort.Strings(names)

	for _, n := range names {
		if strings.Contains(params[n], "?") {
			logger.Printf("sync: %s.parameters.%s = %q looks like an unfilled placeholder; the %q macro will not render until it is set to a real value", section, n, params[n], macro)
		}
	}

	if macro != "view-git-file" {
		return
	}

	// Work out the branch value the macro will actually emit: a "{branch}"
	// parameter takes the resolved (already branch_ref-expanded) value; a
	// literal value is emitted verbatim.
	var branchParam string
	for _, n := range names {
		if strings.EqualFold(n, "branch") {
			branchParam = params[n]
			break
		}
	}
	if branchParam == "" {
		return
	}
	effective := branchParam
	if strings.Contains(branchParam, "{branch}") {
		effective = strings.ReplaceAll(branchParam, "{branch}", resolvedBranch)
	}
	if effective == "" || strings.HasPrefix(effective, "refs/") {
		return
	}
	logger.Printf("sync: %s branch resolves to %q, a bare name; view-git-file needs a full ref like refs/remotes/origin/%s — set %s.branch_ref: remote with branch: \"{branch}\" to expand it automatically", section, effective, effective, section)
}

// resolveGitContext returns the branch and sync-root-relative-repo
// path that the plantuml / git_files rewriters need. branchOverride and
// rootOverride come from the corresponding profile section; when empty
// we walk up from absFrom to find .git and read HEAD + origin.
//
// configSection is "plantuml" or "git_files" purely for error message
// hints (so the user knows which section to set the override in).
func resolveGitContext(absFrom, branchOverride, rootOverride, configSection string) (branch, syncRel string, err error) {
	info, gitErr := sync.FindGitInfo(absFrom)

	branch = branchOverride
	if branch == "" {
		if info == nil {
			return "", "", fmt.Errorf("auto-detect branch failed (%v); set %s.branch in profile", gitErr, configSection)
		}
		branch = info.Branch
		if branch == "" {
			return "", "", fmt.Errorf("git HEAD is detached; set %s.branch in profile to pin a branch", configSection)
		}
	}

	repoRoot := rootOverride
	if repoRoot == "" {
		if info == nil {
			return "", "", fmt.Errorf("auto-detect repo root failed (%v); set %s.repo_root in profile", gitErr, configSection)
		}
		repoRoot = info.Root
	}
	if !filepath.IsAbs(repoRoot) {
		repoRoot = filepath.Join(absFrom, repoRoot)
	}

	rel, err := filepath.Rel(repoRoot, absFrom)
	if err != nil {
		return "", "", fmt.Errorf("compute --from path relative to repo root %q: %w", repoRoot, err)
	}
	if strings.HasPrefix(rel, "..") {
		return "", "", fmt.Errorf("--from %q is outside repo root %q", absFrom, repoRoot)
	}
	return branch, filepath.ToSlash(rel), nil
}

// resolveImageRepoFS returns a filesystem rooted at the git repository
// containing absFrom, plus the slash path from that root down to absFrom.
// It lets the sync engine resolve image/attachment references that point
// outside --from but still inside the repo. Best-effort: when absFrom is
// not inside a git repo (or the relative path can't be computed), it
// returns (nil, "") and image resolution falls back to the --from tree.
func resolveImageRepoFS(absFrom string, logger *log.Logger) (fs.FS, string) {
	info, err := sync.FindGitInfo(absFrom)
	if err != nil || info == nil || info.Root == "" {
		return nil, ""
	}
	rel, err := filepath.Rel(info.Root, absFrom)
	if err != nil || strings.HasPrefix(rel, "..") {
		if logger != nil {
			logger.Printf("sync: --from is outside detected repo root %q; image refs outside --from won't resolve", info.Root)
		}
		return nil, ""
	}
	return os.DirFS(info.Root), filepath.ToSlash(rel)
}

// outcomeFromStats maps planned counts into the Outcome shape so the
// dry-run report uses the same renderer as the post-apply report.
func outcomeFromStats(s sync.PlanStats) *sync.Outcome {
	return &sync.Outcome{
		Created:        s.Create,
		Updated:        s.Update,
		Skipped:        s.Skip,
		Orphaned:       s.Orphan,
		ImagesUploaded: s.Images,
	}
}

// printSyncReport renders the plan/outcome to w. In dry-run mode the
// per-action listing is always shown (the whole point of --dry-run is to
// preview what would happen). In apply mode the listing is gated behind
// --debug to keep the default output compact.
func printSyncReport(w io.Writer, plan *sync.Plan, outcome *sync.Outcome, dryRun, debug bool) {
	header := "Sync apply result"
	if dryRun {
		header = "Sync dry-run (no changes applied)"
	}
	fmt.Fprintln(w, header)
	fmt.Fprintln(w, "----")

	if dryRun || debug {
		for _, a := range plan.Actions {
			label := a.RelPath
			if label == "" {
				label = fmt.Sprintf("page %d (%s)", a.PageID, a.Title)
			}
			reason := a.Reason
			if reason != "" {
				reason = " — " + reason
			}
			fmt.Fprintf(w, "  %-7s %s%s\n", string(a.Kind), label, reason)
		}
		if len(plan.Actions) > 0 {
			fmt.Fprintln(w, "----")
		}
	}

	imageLabel := "images:"
	if dryRun {
		imageLabel = "images (to upload):"
	}
	fmt.Fprintf(w, "created:  %d\n", outcome.Created)
	fmt.Fprintf(w, "updated:  %d\n", outcome.Updated)
	fmt.Fprintf(w, "skipped:  %d\n", outcome.Skipped)
	fmt.Fprintf(w, "orphaned: %d\n", outcome.Orphaned)
	fmt.Fprintf(w, "%-9s %d\n", imageLabel, outcome.ImagesUploaded)
	fmt.Fprintf(w, "errors:   %d\n", len(outcome.Errors))

	for _, e := range outcome.Errors {
		fmt.Fprintf(w, "  error: %s\n", e.Error())
	}
}
