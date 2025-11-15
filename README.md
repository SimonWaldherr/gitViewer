# gitViewer

[![Go Version](https://img.shields.io/badge/go-1.25.4-blue.svg)](https://golang.org/doc/go1.25)
[![License](https://img.shields.io/badge/license-see%20repository-lightgrey.svg)](LICENSE)

A minimal, lightweight web-based Git repository viewer written in Go. Similar in spirit to `python -m http.server`, but specifically designed for exploring Git repositories through a clean, modern web interface.

**Perfect for:** Quick local repository exploration, code reviews, documentation browsing, and teaching Git concepts visually.

## Features

- **Repository Overview**: View repository information and current HEAD
- **File Browser**: Navigate through the repository file tree at any branch or commit
- **File Viewer**: View file contents with syntax highlighting support
- **Commit History**: Browse the commit log with dates and messages
- **Diff Viewer**: Compare changes between commits or branches
- **GitHub Actions**: View GitHub Actions workflow files
- **Pages Viewer**: Serve any branch as a static site (not just gh-pages!)
- **Branch Switching**: Easily switch between different branches
- **Raw File Access**: Download raw file contents
- **Responsive UI**: Clean, minimal web interface

## Installation

### From Source

Requires Go 1.25.4 or later:

```bash
git clone https://github.com/SimonWaldherr/gitViewer.git
cd gitViewer
go build .
```

This will create a `gitViewer` executable in the current directory.

### Using go install

```bash
go install github.com/SimonWaldherr/gitViewer@latest
```

## Usage

### Basic Usage

Start the server in the current Git repository:

```bash
gitViewer
```

By default, the server listens on `:8080`. Open your browser and navigate to:

```
http://localhost:8080
```

### Specify a Repository

Serve a specific Git repository:

```bash
gitViewer /path/to/your/repo
```

### Custom Port

Change the listen address:

```bash
gitViewer -addr :3000
```

Or bind to a specific interface:

```bash
gitViewer -addr 127.0.0.1:8080
```

### Complete Example

```bash
gitViewer -addr :9090 ~/projects/my-repo
```

## Command-Line Options

```
-addr string
    HTTP listen address (default ":8080")
```

## Serving Branches as Static Sites

One of gitViewer's unique features is the ability to serve **any branch** as a static website, not just `gh-pages`. This is perfect for:

- **Preview branches**: View documentation or static site builds from feature branches
- **Multi-version docs**: Serve different versions from different branches
- **Testing**: Preview static builds before merging to production
- **Development**: View builds from any branch without switching your working directory

### Examples

```bash
# View the main branch as a static site
http://localhost:8080/pages/main/

# View a feature branch (with slashes in the name)
http://localhost:8080/pages/feature/new-docs/

# View a specific file in a branch
http://localhost:8080/pages/develop/assets/style.css

# Default: gh-pages (for backward compatibility)
http://localhost:8080/pages/
```

The pages viewer automatically:
- Detects valid branch names (even with slashes)
- Serves `index.html` for directory paths
- Sets correct MIME types for all file types
- Provides a dropdown menu in the UI to easily switch between branches

## Features in Detail

### Repository Overview (/)
The home page shows:
- Repository name
- Current branch or HEAD reference
- Short commit hash

### File Browser (/tree)
Navigate through directories and files:
- View directory contents
- See file sizes and types
- Navigate back to parent directories
- Switch between branches

### File Viewer (/blob)
View file contents:
- Syntax-highlighted code display
- Line numbers
- Raw file download option
- Large files are truncated (200 KiB preview limit)

### Commit History (/commits)
Browse recent commits:
- Short commit hashes
- Commit dates
- Commit messages
- Limited to 50 most recent commits

### Diff Viewer (/diff)
Compare changes:
- View unified diffs between any two commits or branches
- See file statistics and patch content

### GitHub Actions (/workflows)
List workflow files from `.github/workflows` directory

### Pages Viewer (/pages/)
Serve any branch as a static site, not just gh-pages:
- Access any branch: `/pages/{branch-name}/` (e.g., `/pages/main/`, `/pages/docs/`, `/pages/feature/new-ui/`)
- Supports branch names with slashes (e.g., `/pages/feature/ui-redesign/`)
- Serves `index.html` by default for directory paths
- Proper MIME types for web assets (HTML, CSS, JS, images, etc.)
- Navigate through nested paths: `/pages/{branch}/path/to/file.html`
- Backward compatible: `/pages/` defaults to gh-pages branch
- Dropdown menu in navigation bar shows all available branches

## Technical Details

- **Language**: Go
- **Dependencies**: Standard library only (no external dependencies)
- **Templates**: Embedded HTML templates
- **Static Assets**: Embedded CSS and JavaScript
- **Git Integration**: Uses the `git` command-line tool

## Requirements

- Go 1.25.4 or later (for building from source)
- Git installed and available in PATH (for runtime)

## Development

The project structure:
```
gitViewer/
├── main.go           # Main server implementation
├── go.mod            # Go module file
├── templates/        # HTML templates
│   ├── layout.html
│   ├── index.html
│   ├── tree.html
│   ├── blob.html
│   ├── commits.html
│   ├── diff.html
│   └── workflows.html
└── static/           # CSS and JavaScript
    ├── app.css
    └── app.js
```

### Building

```bash
go build .
```

This creates a `gitViewer` executable.

### Testing Locally

```bash
go run . -addr :8080
```

## Use Cases

- **Quick Repository Exploration**: Browse any Git repository without cloning to a special location
- **Code Review**: View diffs and file contents in a web browser
- **Documentation**: Serve documentation sites from gh-pages
- **Teaching**: Demonstrate Git concepts visually
- **CI/CD Inspection**: View workflow files easily
- **Lightweight Alternative**: Use instead of heavier Git hosting platforms for local browsing

## Security Considerations

- This tool is intended for **local development use only**
- Do not expose it to untrusted networks without proper authentication
- It provides read-only access to Git repositories
- No authentication or authorization mechanisms are included

## License

See the repository for license information.

## Author

Simon Waldherr

## Contributing

Contributions are welcome! Please feel free to submit issues or pull requests.
