# Deadwood

A terminal UI for clearing out git branches and worktrees.

Deadwood shows every local branch with what you need to decide its fate — is it merged (including squash and rebase merges), is its remote gone, how old is it, is a worktree holding it — and lets you delete in bulk, undo, rename, switch, and manage worktrees from the same screen. One binary, no configuration, no GitHub token.

```
 deadwood  my-project  ·  base origin/main                              Branches 9   Worktrees 2

     Branch                    State      Sync      Upstream               Commit   Updated
 ──────────────────────────────────────────────────────────────────────────────────────────────
     chore/old-experiment      stale      local                            ab14a3b  1y 9mo ago
  ●  fix/typo                  merged     synced    origin/fix/typo        825a394  3w ago
  ●  feature/export            squashed   gone      origin/feature/export  29f3319  5d ago
     feature/login                        ↑1        origin/feature/login   6f67d42  2h ago
     feature/payments          worktree   ↑3 ↓1     origin/feature/paym…   1bc1a93  16m ago
     main                      current    synced    origin/main            6307dfe  just now
  ●  refactor/parser           rebased    local                            2a183cc  2d ago
     release/1.0               protected  local                            6307dfe  4mo ago

 3 selected  ·  2 of 9
 space select  ·  d delete  ·  M select merged  ·  / filter  ·  ? all keys  ·  q quit  ·  u undo
```

## Why

- **It knows what is really merged.** `git branch --merged` misses branches that landed through GitHub's *Squash and merge* or *Rebase and merge*. Deadwood compares patches, so those show up as `squashed` and `rebased`, and `M` selects every branch that is safe to drop.
- **Deleting is reversible.** `u` restores the branches you just deleted. When you quit, deadwood prints a `git branch <name> <sha>` line for everything it removed in the session.
- **It is careful.** `main`, `master`, `develop`, the base branch, your own patterns, the current branch and branches checked out in a worktree are never deleted. A branch that moved since it was listed is left alone. Remote branches are deleted with a lease, so a branch someone pushed to after your last fetch survives.
- **Branches and worktrees live together.** Create a worktree for a branch, remove a worktree together with its branch, lock, prune, or jump into one from your shell.

## Install

**Homebrew** (macOS and Linux):

```sh
brew install manojsachwani/taps/deadwood
```

**Go** 1.24 or newer:

```sh
go install github.com/manojsachwani/deadwood/cmd/deadwood@latest
```

**Prebuilt binary:** download the archive for your system from the [releases page](https://github.com/manojsachwani/deadwood/releases), unpack it and put `deadwood` on your `PATH`:

```sh
tar -xzf deadwood_*_darwin_arm64.tar.gz
sudo mv deadwood /usr/local/bin/
```

The binaries are not notarized. If macOS refuses to open a downloaded one, run `xattr -d com.apple.quarantine deadwood` once; the Homebrew install does this for you. `checksums.txt` on each release lets you verify a download with `shasum -a 256 -c`.

Deadwood needs git 2.31 or newer in your `PATH`.

## Usage

Run it inside a repository (any worktree of it):

```sh
deadwood
```

### Keys

Press `?` in the app for this list.

**Branches**

| Key | Action |
| --- | --- |
| `↑`/`k` `↓`/`j` `g` `G` | Move |
| `space` | Select / deselect |
| `ctrl+a` | Select all / none |
| `M` | Select every merged, squashed or rebased branch |
| `S` | Select every stale branch |
| `d` | Delete the selection, locally or locally + remote |
| `u` | Undo the last delete |
| `r` | Rename |
| `s` | Switch to the branch |
| `w` | Create a worktree for the branch |
| `enter` | Show the commits that are not in the base branch |
| `/` | Filter by name |
| `o` | Cycle sort order: name, newest, oldest |

**Worktrees**

| Key | Action |
| --- | --- |
| `n` | New worktree, for an existing or a new branch |
| `d` | Remove the worktree, optionally with its branch |
| `enter` | Quit and print the worktree's path (see [shell integration](#jump-into-a-worktree)) |
| `l` | Lock / unlock |
| `p` | Prune entries whose directory is gone |

**Everywhere**

| Key | Action |
| --- | --- |
| `tab`, `1`, `2` | Switch tab |
| `F` | Fetch all remotes and prune |
| `ctrl+r` | Refresh |
| `esc` | Clear the selection, then the filter |
| `?` | Show all keys |
| `q` | Quit |

`d`, `r` and `enter` act on the branch under the cursor when nothing is selected.

### Reading the table

| Column | Values |
| --- | --- |
| State | `current` · `worktree` (checked out in a worktree) · `protected` · `merged` · `squashed` · `rebased` · `stale` |
| Sync | `synced` · `↑2 ↓1` (ahead / behind the upstream) · `gone` (the upstream branch was deleted) · `local` (no upstream) |

`merged`, `squashed` and `rebased` are relative to the **base branch** shown in the header: the remote's default branch when there is one, otherwise `main` or `master`. Columns disappear as the window narrows; branch, state and sync stay longest.

### Deleting

1. Select branches (`space`, `M`, `S`, `ctrl+a`) or just put the cursor on one, and press `d`.
2. Choose `l` for local only, or `r` to delete the upstream branch too.
3. Confirm with `y`. The prompt tells you how many of the branches are **not** merged into the base.

What happens then:

- Local deletion is forced (`git branch -D`), because squash-merged branches cannot be deleted any other way. That is why the unmerged warning and undo exist.
- Protected branches, the current branch and branches held by a worktree are skipped and reported.
- Remote deletion targets the branch's own upstream, whatever the remote is called, and uses `--force-with-lease`. Branches without an upstream, or whose upstream is already gone, are only deleted locally.
- `u` brings the last batch back locally and re-attaches the upstream if it still exists. A deleted remote branch is **not** restored; push the branch to recreate it.

### Worktrees

- New worktrees default to a sibling of the main checkout, e.g. `../my-project-feature-login`. A branch name that does not exist yet is created from `HEAD`.
- Removing a worktree with uncommitted changes, or a locked one, warns first and then forces the removal. Press `b` instead of `y` to delete its branch in the same step.
- The main worktree, and the one deadwood is running in, cannot be removed.

#### Jump into a worktree

`enter` on the Worktrees tab quits and prints the path. Add this to your shell profile to land there:

```sh
dw() { local dir; dir="$(deadwood "$@")" && [ -d "$dir" ] && cd "$dir"; }
```

## Command line

```
deadwood [flags]

  --base <ref>        compare branches against this ref
  --stale-days <n>    mark branches without commits for n days as stale (default 90, 0 disables)
  --list              print the branches and their state, then exit
  --json              like --list, as JSON including worktrees
  --version           print the version
```

`--list` and `--json` never change anything, so they are safe in scripts:

```sh
deadwood --json | jq -r '.branches[] | select(.merged and (.protected|not) and (.current|not)) | .name'
```

## Configuration

Deadwood reads three optional git config keys, per repository or `--global`:

```sh
git config deadwood.base origin/develop        # default for --base
git config deadwood.staleDays 30               # default for --stale-days
git config --add deadwood.protect 'release/*'  # never delete these; repeat for more patterns
```

## How it compares

Other tools do parts of this well. This is how they looked in October 2026, going by their own documentation:

| | Deadwood | [git-barber](https://github.com/rekurt/git-barber) | [gh-poi](https://github.com/seachicken/gh-poi) | [git-trim](https://github.com/foriequal0/git-trim) | [lazyworktree](https://github.com/chmouel/lazyworktree) |
| --- | --- | --- | --- | --- | --- |
| Interactive UI | yes | yes | no | no | yes |
| Squash / rebase merge detection | yes | yes | yes, from pull requests | yes | — |
| Works without GitHub or a token | yes | yes | no | yes | yes |
| Undo inside the app | yes | prints restore commands | no | no | — |
| Rename and switch branches | yes | no | no | no | — |
| Worktree management | yes | protects their branches | cleans up merged ones | no | yes |
| Unattended cleanup (`--yes`) | no | yes | yes | yes | — |
| Pull request and CI status | no | no | yes | no | yes |

If you want cleanup to run unattended in a script, or pull request status next to each worktree, those tools are the better pick today. Deadwood's bet is one interactive screen for both branches and worktrees, with deletes you can take back.

## Limits

- Squash detection compares patches. A squash merge whose conflicts were resolved by hand produces a different patch and is not recognised; such a branch simply stays unmarked.
- Merge detection runs a few git commands per unmerged branch, so the first load takes longer in repositories with hundreds of them.
- `gone` and the remote lease are only as fresh as your last fetch. Press `F` first.

## Build from source

```sh
git clone https://github.com/manojsachwani/deadwood
cd deadwood
go build -o deadwood ./cmd/deadwood
go test ./...
```

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

[MIT](LICENSE)
