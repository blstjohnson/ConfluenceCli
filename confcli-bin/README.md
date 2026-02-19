# confcli - Confluence CLI Tool

`confcli` is a command-line interface for Atlassian Confluence that allows you to retrieve, search, and manage Confluence pages from the terminal. It's designed to be a lightweight, cross-platform alternative to the MCP server with enhanced integration capabilities for LLM agents.

## Features

- Retrieve Confluence pages by ID, space/title, or path
- Export page hierarchies and descendants
- Support for multiple output formats (text, JSON, YAML, markdown, export)
- Disk-based caching with configurable TTL
- Cross-platform support (Linux, Windows, macOS)
- Shell autocompletion
- Read-only mode for safe operations
- Machine-readable help output
- Full CRUD operations for pages, comments, and labels
- **AI Agent Integration**: Built-in slash commands and skills for AI agents (Qwen Code, Claude Code)

### AI Agent Integration

The CLI includes dedicated commands for AI agent integration:

- **Slash Commands** (Read operations):
  - `/get-page` - Get page by ID in export view (JSON output)
  - `/get-page-diff` - Get diff between two versions of a page

- **Skills** (Write operations):
  - `edit-page` - Edit a page with AI assistance (two-step workflow)

To initialize AI agent configuration files:

```bash
confcli ai-agent init --agent all --output-dir ~/.confcli/ai-agents
```

This creates configuration files for Qwen Code and Claude Code in the specified directory.

### Content Formats

The CLI supports multiple content formats for retrieving pages:

- **markdown** (default): Converts Confluence storage format to clean Markdown using advanced HTML-to-Markdown conversion with support for:
  - Tables with proper markdown formatting (including colspan/rowspan handling)
  - Info/Warning/Note/Tip panels
  - Status macros
  - Expand/collapse sections
  - Code blocks with syntax highlighting
  - User mentions
- **storage**: Raw Confluence storage format (XML/HTML-based) - useful for updating pages while preserving formatting
- **edit** (editor): Confluence editor format - optimized for editing in Confluence's editor
- **export**: Export view format converted to Markdown - cleaner HTML representation meant for exporting, with improved table formatting and fewer Confluence-specific macros
- **html**: Raw HTML output
- **plain**: Plain text with all HTML tags stripped

Use `--format export` when you want clean Markdown output optimized for reading and documentation, especially for pages with complex tables.
Use `--format storage` or `--format edit` when you plan to update the page later and want to preserve Confluence formatting.

## Installation

### From Source

```bash
git clone https://github.com/your-org/confcli.git
cd confcli
make build
```

### Using Go

```bash
go install confcli@latest
```

## Configuration

Create a configuration file at `~/.confcli/config.yaml`:

```yaml
current_profile: default
profiles:
  default:
    url: "https://your-domain.atlassian.net/wiki"
    token: "your-api-token"
    username: "your-email@example.com"
    auth_type: "bearer"
    read_only: false
  prod:
    url: "https://production-domain.atlassian.net/wiki"
    token: "production-api-token"
    username: "admin@example.com"
    auth_type: "bearer"
    read_only: true
```

Or use the config command:

```bash
# Initialize config
confcli config init

# Set configuration values
confcli config set url https://your-domain.atlassian.net/wiki
confcli config set token your-api-token
confcli config set username your-email@example.com
```

## Usage

### Basic Commands

```bash
# Get a page by ID
confcli page get --id 123456

# Get a page by space and title
confcli page get --space DEV --title "Project Documentation"

# Get a page by path
confcli page get --path "DEV/Project/Documentation"

# Export a space hierarchy
confcli hierarchy space --space DEV --output-dir ./export

# Get descendants of a page
confcli descendants get --id 123456 --depth 3

# Search for pages
confcli search "my query"
```

### Output Formats

```bash
# Human-readable text (default)
confcli page get --id 123456

# JSON output (for machine processing)
confcli page get --id 123456 --format json

# YAML output
confcli page get --id 123456 --format yaml
```

### Read-Only Mode

To prevent any modifying operations:

```bash
# Enable read-only mode globally
confcli --read-only page get --id 123456

# Or set in config
confcli config set read_only true
```


## Commands

### Page Commands

- `confcli page get` - Get a page by ID, space/title, or path
- `confcli page comments <id>` - Get comments for a page
- `confcli page labels <id>` - Get labels for a page
- `confcli page create` - Create a new page
- `confcli page update <id>` - Update an existing page
- `confcli page delete <id>` - Delete a page
- `confcli page comment add <id>` - Add a comment to a page
- `confcli page label add <id>` - Add a label to a page

### Hierarchy Commands

- `confcli hierarchy space` - Get hierarchy of a space

### Descendants Commands

- `confcli descendants get` - Get descendants of a page

### Other Commands

- `confcli search [query]` - Search for pages
- `confcli config` - Manage configuration profiles
- `confcli completion` - Generate shell completion scripts
- `confcli help-json` - Output command structure in JSON format
- `confcli ai-agent` - AI agent integration commands

### AI Agent Commands

- `confcli ai-agent init` - Initialize AI agent configuration
- `confcli ai-agent slash get-page <id>` - Get page by ID in export view (JSON output)
- `confcli ai-agent slash get-page-diff <page-id>` - Get diff between two versions
- `confcli ai-agent skill edit-page <id>` - Edit a page with AI assistance

## Detailed Command Examples

### Getting Pages

```bash
# Get page by ID with markdown content (default)
confcli page get --id 123456

# Get page by ID with export format (clean markdown for reading)
confcli page get --id 123456 --format export

# Get page by space and title
confcli page get --space DEV --title "Project Overview"

# Get page by path (resolves through ancestors)
confcli page get --path "DEV/Projects/Overview"

# Get page with specific version
confcli page get --id 123456 --version 3

# Get page content in storage format (for editing later)
confcli page get --id 123456 --format storage

# Get page content in editor format (for Confluence editor)
confcli page get --id 123456 --format edit

# Get page with comments and labels
confcli page get --id 123456 --with-comments --with-labels

# Full export of page with all metadata
confcli page get --id 123456 --full --output-dir ./exports
```

### Working with Hierarchies

```bash
# Show space hierarchy as tree
confcli hierarchy space --space DEV --tree

# Export space to directory structure
confcli hierarchy space --space DEV --output-dir ./export --format both

# Get flat list of all pages in space
confcli hierarchy space --space DEV --flat --format json

# Limit depth of hierarchy
confcli hierarchy space --space DEV --depth 2 --tree
```

### Working with Descendants

```bash
# Get descendants of a page
confcli descendants get --id 123456

# Get descendants with limited depth
confcli descendants get --id 123456 --depth 2 --tree

# Export descendants to directory
confcli descendants get --id 123456 --output-dir ./descendants --format markdown

# Include source page in output
confcli descendants get --id 123456 --include-self --tree
```

### Managing Pages

```bash
# Create a new page
confcli page create --space DEV --title "New Page" --content-file content.md --confirm

# Update an existing page
confcli page update 123456 --content-file updated-content.md --version-comment "Updated content" --confirm

# Delete a page
confcli page delete 123456 --confirm

# Add a comment to a page
confcli page comment add 123456 --text "This is a great page!" --confirm

# Add a label to a page
confcli page label add 123456 --label "important" --confirm
```

### Searching

```bash
# Simple search
confcli search "project documentation"

# Advanced search with CQL
confcli search --cql 'space = "DEV" AND text ~ "performance"' --limit 10

# Search in specific space
confcli search "error handling" --space DEV
```

## Shell Completion

Generate completion scripts:

```bash
# Bash
confcli completion bash > /etc/bash_completion.d/confcli

# Zsh
confcli completion zsh > "${fpath[1]}/_confcli"

# Fish
confcli completion fish > ~/.config/fish/completions/confcli.fish

# PowerShell
confcli completion powershell > confcli.ps1
```

## Development

### Prerequisites

- Go 1.25+
- Make
- Docker (for cross-platform builds)

### Building

```bash
# Build for current platform
go build -o confcli ./cmd/confcli

# Build for all platforms using Docker (requires Docker)
make release

# Run tests
make test

# Clean build artifacts
make clean
```

### Cross-Platform Docker Build

To build binaries for Windows, Linux, and macOS using Docker:

```bash
# Build all binaries using Docker
make build-docker

# Create release packages
make release
```

Alternatively, use the build script:

```bash
./build.sh
```

This will create binaries for:
- Linux (AMD64 and ARM64)
- Windows (AMD64 and ARM64) 
- macOS (AMD64 and ARM64)

The binaries will be placed in the `dist/releases/` directory.

## Architecture

The confcli tool follows a modular architecture:

- `cmd/` - Main entry points
- `internal/` - Internal packages
  - `client/` - Confluence API client
  - `config/` - Configuration management
  - `commands/` - CLI command implementations
  - `formatter/` - Output formatting
  - `converter/` - Content format conversion
  - `utils/` - Utility functions

## API Integration

The tool supports both Confluence Cloud (REST API v2) and Confluence Server/DC (v1) with:

- Proper authentication (Bearer token, Basic auth)
- Rate limiting and retries
- Pagination for large datasets
- Context-aware cancellation
- Comprehensive error handling

## Security

- All modifying operations require explicit confirmation
- Read-only mode prevents accidental changes
- Authentication tokens are not logged
- Secure credential handling

## AI Agent Integration

The CLI provides dedicated commands for AI agent integration, designed to work seamlessly with AI coding assistants like Qwen Code and Claude Code.

### Initialization

To set up AI agent configuration files:

```bash
# Initialize for all supported agents
confcli ai-agent init --agent all

# Initialize for a specific agent
confcli ai-agent init --agent qwen
confcli ai-agent init --agent claude

# Specify custom output directory
confcli ai-agent init --output-dir /path/to/config
```

This creates the following files:
- `qwen-commands.json` - Command definitions for Qwen Code
- `qwen-instructions.md` - Usage instructions for Qwen Code
- `claude-commands.json` - Command definitions for Claude Code
- `claude-instructions.md` - Usage instructions for Claude Code

### Slash Commands (Read Operations)

#### Get Page by ID

Retrieves a Confluence page in export view format with JSON output:

```bash
# Basic usage
confcli ai-agent slash get-page --id 123456

# Include labels and comments
confcli ai-agent slash get-page --id 123456 --with-labels --with-comments

# Save output to file
confcli ai-agent slash get-page --id 123456 --output page.json
```

Output format:
```json
{
  "page_id": "123456",
  "title": "Page Title",
  "space": "DEV",
  "space_name": "Development",
  "version": 5,
  "content": "# Page Content...",
  "web_url": "/spaces/DEV/pages/123456",
  "edit_url": "/pages/edit.action?pageId=123456",
  "updated_at": "2024-01-01T00:00:00Z",
  "labels": [...],
  "comments": [...]
}
```

#### Get Page Diff

Shows differences between two versions of a page:

```bash
# Compare versions 1 and 2
confcli ai-agent slash get-page-diff --page-id 123456 --old-version 1 --new-version 2

# Save diff to file
confcli ai-agent slash get-page-diff --page-id 123456 --old-version 1 --new-version 2 --output diff.json
```

Output format:
```json
{
  "page_id": 123456,
  "old_version": 1,
  "new_version": 2,
  "format": "unified",
  "diff": "--- version 1\n+++ version 2\n...",
  "changes_summary": {
    "old_line_count": 100,
    "new_line_count": 105,
    "lines_added": 10,
    "lines_removed": 5,
    "net_change": 5
  }
}
```

### Skills (Write Operations)

#### Edit Page

Edits a Confluence page with AI assistance using a two-step workflow:

**Step 1: Get page content for editing**

```bash
confcli ai-agent skill edit-page --id 123456
```

This retrieves the page in edit view format and saves it to a temporary file. The command outputs:
```json
{
  "status": "temp_file_created",
  "page_id": 123456,
  "title": "Page Title",
  "version": 5,
  "temp_file": "/tmp/confcli-edit-123456/page_123456.md",
  "content": "...",
  "next_step": "Modify the content and run: confcli ai-agent skill edit-page <id> --content-file <modified_file> --version-comment '<comment>' --confirm"
}
```

**Step 2: Update page with modified content**

After the AI agent modifies the content:

```bash
confcli ai-agent skill edit-page --id 123456 --content-file modified.md --version-comment "Updated by AI" --confirm
```

Optional flags:
- `--temp-dir`: Specify custom temporary directory
- `--keep-temp`: Keep temp file after update
- `--version-comment`: Add a comment to the version update

Output format:
```json
{
  "status": "page_updated",
  "page_id": 123456,
  "title": "Page Title",
  "old_version": 5,
  "new_version": 6,
  "version_comment": "Updated by AI",
  "web_url": "/spaces/DEV/pages/123456",
  "edit_url": "/pages/edit.action?pageId=123456",
  "temp_file_removed": true
}
```

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

MIT License