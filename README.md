# Deadwood

An interactive terminal app written in Go using Charm libraries. Use it to select, delete, and rename git branches in bulk or individually, with options for local and remote deletion.

## Features

- Select multiple branches with spacebar
- Delete branches (locally or locally + remotely)
- Rename a branch
- Batch operations
- TUI powered by [Bubble Tea](https://github.com/charmbracelet/bubbletea)

## Usage

Run the app in a git repository:

```sh
./deadwood
```

**Keybindings:**

- `Space`: Select/deselect branch
- `d`: Delete selected branches
- `r`: Rename selected branch (only one at a time)
- `q`: Quit

## Build

Build for your current OS/arch:

```sh
go build -o deadwood ./cmd/main.go
```

Build for another platform (example: macOS arm64):

```sh
GOOS=darwin GOARCH=arm64 go build -o deadwood ./cmd/main.go
```

## Distribute

Share the `deadwood` binary, or compress it:

```sh
zip deadwood.zip deadwood
```

## Automated Releases

Releases are created automatically on GitHub when you push a new tag starting with `v` (e.g., `v1.0.0`). Binaries for Linux, macOS, and Windows (amd64 and arm64) are attached to each release.

**To create a release:**

```sh
git tag v1.0.0
git push origin v1.0.0
```

See `.github/workflows/release.yml` for details.

## Install from Release

Download the binary for your OS/arch from the [GitHub Releases](https://github.com/yourusername/deadwood/releases) page, then:

```sh
chmod +x deadwood
./deadwood
```

## Requirements

- Go 1.21 or newer (for building)
- git (in your PATH)

## License

MIT
