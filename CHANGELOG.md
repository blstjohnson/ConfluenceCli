# Changelog

All notable changes to confcli will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

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
