# Changelog

All notable changes to confcli will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [v0.0.14] - 2026-06-19

### Added

- `confcli page get` can rewrite Confluence internal page links to local files, driven through the transform-profile `--set` mechanism (no named `--transform` profile required). New keys: `page.refs_dir` / `page.refs_dirs` (comma-separated folders of previously-exported pages to resolve links against), `page.rewrite_links`, and `page.refs_link_style` (`relative` default, or `absolute`). A referenced page found in a reference folder is rewritten to its local file; the page ID → file map is built from `{id}_*` filenames and `*.meta.json` sidecars, so it works with both default and `--clean-names` hierarchy exports. Pages not present keep their original Confluence URL
- `--set page.clear_macros=<names>` and `--set page.expand_macros=<names>` convenience overrides that append `remove_macro` transforms: `clear` drops the macro and its content, `expand` unwraps it (preserves the inner content). Comma-separated macro names
- `converters.BuildPageMapFromDirs` — scans exported page folders to build a page-ID → local-file map for link rewriting

### Changed

- `--set` overrides now apply to `confcli page get` even without a `--transform` profile (an empty profile is synthesized), so standalone flags like `--set page.refs_dir=…` work on their own

### Fixed

- `confcli page get` now runs the transform pipeline both before and after format conversion (matching `hierarchy space`). Previously content transforms (e.g. `remove_macro`) ran only after conversion, so macro stripping/expansion on a single page was silently a no-op

## [v0.0.13] - 2026-06-08

### Fixed

- `confcli sync` now reparents pages under `--root` on update, so `--root` is authoritative. Previously a page that already existed (created before `--root` was used, created by hand, or moved) was updated in place and never pulled under the sync root, making it look like `--root` was ignored. Reparenting rides along on the same update request (no extra call); if the target parent failed to create earlier in the run, the content update still happens and reparenting resolves on the next sync

### Added

- `confcli sync` now uploads local images. Markdown image references (`![alt](path.png)`) to on-disk files are rewritten into Confluence `<ac:image><ri:attachment ri:filename="…"/></ac:image>` macros and the image is uploaded as a page attachment after the page is created/updated. Always-on; supports png/jpg/jpeg/gif/svg/webp/bmp. Remote images (`http(s)://`, protocol-relative, `data:`) and unreadable/unsupported references are left untouched. Attachments are keyed by basename and upserted (re-upload replaces existing data)
- Image bytes are folded into the page content hash, so editing an image (without changing the markdown) triggers a re-sync. The sync report gains an `images` line (shown as `images (to upload)` under `--dry-run`)
- `api.Client.UploadAttachment` — multipart attachment upload (`POST /content/{id}/child/attachment`, with `X-Atlassian-Token: no-check`), upserting by filename

## [v0.0.12] - 2026-05-27

### Added

- `confcli sync` command: single-run uploader from a local markdown tree to a Confluence page tree, anchored by `confcli-id-<sha>` labels. Builds a plan (create / update / skip / orphan) printable via `--dry-run`, then applies it (create/update pages, reparent under folder markers, update content-hash labels)
- Import profile (`kind: import`): `tree.folder_page` (per-folder marker file), `tree.skip` / `tree.flatten` doublestar globs, per-path `overrides` for skipping a path or pinning it to a fixed `page_id`
- `tree.title.rewrites` regex pipeline + `tree.title.trim` for deriving Confluence page titles from filenames (e.g. `getting_started.md` → `Getting Started`). Rewrites apply only to the basename stem so the identity label and forward-link resolution stay stable
- `plantuml` and `git_files` profile sections: rewrite markdown links to `.puml` files and to non-md/non-puml repo files (yaml, json, sql, sh, …) into Confluence macros (typically `view-git-file`) so links resolve at Confluence render time instead of becoming broken relative hrefs
- `git_files.mode: link | inline` (default `link`), `git_files.per_extension` per-extension overrides, and `git_files.inline.max_bytes` size cap. Inline mode reads the source file from `--from` at sync time and embeds its content in a Confluence `code` structured macro on the page

### Changed

- Forward-link rewriter and identity hashing now share `profile.TitleFor`, so any title transforms configured under `tree.title` propagate to both the rendered page title and `ac:link` targets without drift

## [v0.0.11] - 2026-05-08

### Added

- `embed_plantuml_links` transform: rewrites markdown links targeting `.puml` / `.plantuml` files into image embeds so renderers with PlantUML preview can show the diagram inline. Strips surrounding `**` bold markers (a PlantUML link should not be bold) and skips links inside fenced code blocks
- `skip_root` per-page override in transform profile YAML, reinstated alongside the `--skip-root` CLI flag

### Changed

- Verified and documented deep-flatten via per-page `flatten: true` profile override; README and `transform init` starter template now contrast the global `flat_leaves` (leaf-only) and per-page `flatten` (cascade to all descendants) modes

### Fixed

- Scroll Versions probe no longer prints a 403/404 warning when the user lacks plugin admin permission; a clear error is still shown when `--scroll-version` is requested but the plugin is unavailable
- Cross-platform isolation in `pkg/config/config` tests: now sets both `HOME` (POSIX) and `USERPROFILE` (Windows) and resets viper between tests so test runs no longer leak into the user's real `~/.confcli/config.yaml`

## [v0.0.10] - 2026-04-16

### Added

- HTTP client retry with exponential backoff: transient failures (timeouts, 5xx, connection reset) are retried up to 3 times with 1s/2s/4s backoff
- Graceful degradation in batch page fetching: a single page failure no longer aborts the entire batch — failed pages are warned and skipped
- Structured export failure report: lists each failed page ID, title, and error reason with success/fail/skip counts and ready-to-run retry commands
- `preserve_content` parameter for `remove_macro` transform: preserves the content inside removed macros (e.g., expand macro content) instead of discarding it entirely
- Language identifier fallback for code blocks: when no explicit language is set, macro name is used as a hint (e.g., `plantuml` macro → `plantuml` language identifier)

### Changed

- `skip_root` (briefly) removed from transform profile YAML in this release in favour of the `--skip-root` CLI flag on `hierarchy space`; reinstated as a per-page override in v0.0.11
- TOC stripping now uses HTML parser-based approach instead of regex for more reliable handling of deeply nested TOC macros
- Empty folders are no longer created when pages are skipped via transform profile `skip: true`

### Fixed

- `<!--THE-END-->` HTML comments are now stripped after image removal during space export
- TOC stripping handles nested TOC macros inside layout cells, panels, expand macros, and other container elements
- Expand macro removal now preserves inner content when `preserve_content: true` is set

## [v0.0.9] - 2026-04-07

### Added

- Scroll Versions plugin support: `--scroll-version` flag exports a specific named version from spaces using the Scroll Versions plugin
- Generic macro body handling: unknown macros with content bodies render as fenced code blocks; macros with file/URL references render as markdown links for link rewriting
- Tiny URL expansion: `expand_tiny_urls` transform resolves Confluence `/x/AbCd` short links to canonical page URLs before link rewriting
- Per-page flatten override: `flatten: true/false` in transform profile `pages:` section overrides the global `--flat-leaves` setting per page
- Skip cascading for child pages: `skip: true` and `skip_transforms: true` per-page overrides in transform profiles

### Fixed

- TOC stripping now operates at the HTML/storage macro level with nesting-aware regex, correctly removing TOC macros nested inside layout cells, panels, and expand macros
- Hierarchy download is now resilient to per-page errors: failed pages are logged to stderr and skipped instead of aborting the entire export

## [v0.0.8] - 2026-03-25

### Added

- Transform profiles: YAML-based content transformation pipelines for export
  - 6 built-in transform types: `remove_macro`, `remove_element`, `modify_links`, `modify_content`, `rewrite_tfs_links`, `rewrite_internal_links`
  - Profile resolution: file path or named profile from `~/.confcli/transformations/`
  - Per-page overrides with ID and path glob matching
  - `--set` flag for inline parameter overrides without editing the profile
- `--transform` flag on `hierarchy space` and `page get` commands
- `confcli transform` subcommand with `list`, `show`, and `init` actions
- Starter template generation for new transform profiles

### Changed

- `hierarchy space` export flags (`--named-folders`, `--flat-leaves`, `--skip-root`, etc.) can now be driven by a transform profile, with explicit flags taking priority

## [v0.0.7] - 2026-03-25

### Added

- Self-update command for in-place binary updates

### Fixed

- `hierarchy --page-id` now uses the same fetch path as export, fixing a type assertion panic

## [v0.0.6] - 2026-03-23

### Added

- `--date` flag to hierarchy command for date-based version retrieval
- Progress bar for hierarchy space downloads
- Windows PATH elevation: attempt elevated PATH addition before falling back to manual instructions

### Changed

- `hierarchy --page-id` shows subtree only, no ancestors
- Consolidated ai-agent commands into regular page commands

### Removed

- Duplicate ai-agent command

## [v0.0.5] - 2026-03-13

### Added

- Install/uninstall commands for PATH management
- Auto-generate agent skills and commands from cobra tree
- `--with-descendants`, `--depth`, `--skip-content` flags for ai-agent slash get-page
- Enriched CLI flag metadata in help-json output

## [v0.0.4] - 2026-03-11

### Changed

- Named all platform binaries `confcli` without platform postfix

## [v0.0.3] - 2026-03-10

### Added

- CI/CD pipelines and version command
- `--page-id` flag to the hierarchy space command for exporting specific pages and descendants
- `--skip-root` flag to the hierarchy space command
- Link rewriting for hierarchy space exports (`--rewrite-links`)
- HTML list flattening and Markdown table cleanup functions
- `clean-names` and `no-length-limit` commands
- Confluence link stripping for unresolvable links
- Code-block-aware Markdown unescaping
- Space hierarchy and space download features
- Page export with Markdown conversion
- Diff and simple converter
- Initial CLI with page get, edit, and format commands

### Fixed

- Link rewriting: internal links resolved to relative paths, external links preserved
- Markdown escaping made code-block-aware
- License text fix
