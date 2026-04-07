# Changelog

All notable changes to confcli will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

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
