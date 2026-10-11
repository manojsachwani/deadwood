# Deadwood

A terminal UI for clearing out git branches and worktrees. Select branches in bulk, delete them locally or on the remote too, rename them, and create, lock, remove and prune worktrees — without leaving the keyboard.

Built in Go with [Bubble Tea](https://github.com/charmbracelet/bubbletea).

## Features

- **Branches** — one table with upstream, ahead/behind, last commit, author and subject. Branches already merged into the current branch are marked `merged`, branches whose upstream was deleted show `gone`, and branches checked out in a worktree show `worktree`.
- **Bulk delete** — select any number of branches and delete them locally, or locally and on their remote.
- **Rename** a branch in place.
- **Worktrees** — list every worktree with its branch and state (dirty, locked, missing), create one for an existing or new branch, lock/unlock, remove, and prune stale entries.
- Git operations run in the background, and git's own error messages are shown when something fails.
- Adapts to light and dark terminals and to the window size.

## Usage

Run it inside a git repository (any worktree of it):

```sh
deadwood
```

### Keys

| Key | Branches tab | Worktrees tab |
| --- | --- | --- |
| `↑`/`k` `↓`/`j` `g` `G` | Move | Move |
| `tab`, `1`, `2` | Switch tab | Switch tab |
| `space` | Select / deselect | |
| `ctrl+a` | Select all / none | |
| `esc` | Clear selection | |
| `d` | Delete selected branches | Remove worktree |
| `r` | Rename branch | |
| `w` | Create a worktree for the branch | |
| `n` | | New worktree |
| `l` | | Lock / unlock |
| `p` | | Prune stale entries |
| `ctrl+r` | Refresh | Refresh |
| `q` | Quit | Quit |

`d` and `r` act on the branch under the cursor when nothing is selected. In prompts, `enter` confirms and `esc` cancels.

### Reading the table

| Column | Values |
| --- | --- |
| State | `current`, `worktree` (checked out in a worktree), `merged` (fully merged into the current branch) |
| Sync | `synced`, `↑2 ↓1` (ahead / behind the upstream), `gone` (upstream was deleted), `local` (no upstream) |

Columns are hidden as the window gets narrower; the branch name, state and sync stay visible longest.

### What to expect

- **Deleting is forced** (`git branch -D`), so unmerged branches are deleted too. You are asked where to delete (`l` local only, `r` local + remote) and then to confirm; the confirmation warns when any of the branches is not merged into the current branch.
- Remote deletion targets the branch's own upstream. Branches without an upstream, or whose upstream is already gone, are only deleted locally.
- The current branch and branches checked out in a worktree are skipped.
- New worktrees default to a sibling of the main checkout, e.g. `../myrepo-feature-login`. If the branch name doesn't exist yet, it is created from `HEAD`.
- Removing a worktree with uncommitted changes, or a locked one, warns first and then forces the removal. The main worktree, and the one deadwood is running in, cannot be removed.

## Build

```sh
go build -o deadwood ./cmd/
```

Cross-compile (example: macOS arm64):

```sh
GOOS=darwin GOARCH=arm64 go build -o deadwood ./cmd/
```

Run the tests:

```sh
go test ./...
```

## Install from a release

Download the binary for your OS/arch from the [GitHub Releases](https://github.com/manojsachwani/deadwood/releases) page, then:

```sh
chmod +x deadwood
./deadwood
```

## Releases

Pushing a tag that starts with `v` builds binaries for Linux, macOS and Windows (amd64 and arm64) and attaches them to a GitHub release. See `.github/workflows/release.yml`.

```sh
git tag v1.0.0
git push origin v1.0.0
```

## Requirements

- git 2.31 or newer in your `PATH`
- Go 1.24 or newer to build

## Roadmap

See [TODO.md](TODO.md).

## License

MIT
