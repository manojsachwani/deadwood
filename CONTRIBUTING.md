# Contributing

Bug reports and pull requests are welcome.

## Development

```sh
go build -o /tmp/deadwood ./cmd/deadwood
go test ./...
go vet ./...
gofmt -l .          # must print nothing
```

- `git/` wraps the git binary and has no UI code. Its tests create temporary repositories.
- `cmd/deadwood/` is the Bubble Tea program. Its tests drive `Update` and `View` with a fake model, so they need no repository.

Deadwood force-deletes branches, so try changes in a scratch repository, not one you care about.

## Pull requests

- Keep a change focused, and add or update a test for it.
- New keys go in four places: the handler in `update.go`, `keyAction` in `model.go` (so the key can be remapped), the hint line and the `?` overlay in `view.go`, and the key table in the README.

## Releasing

Versions follow [semantic versioning](https://semver.org): a patch for fixes, a minor for new features and keys, a major for changes that break existing keys, flags or the `--json` output.

```sh
git tag v1.2.3
git push origin v1.2.3
```

The tag starts `.github/workflows/release.yml`, which runs the tests and then [GoReleaser](https://goreleaser.com) (`.goreleaser.yaml`). It publishes archives for Linux, macOS and Windows with `checksums.txt` and release notes, and updates the Homebrew cask in `manojsachwani/homebrew-taps`. A tag with a suffix, such as `v1.3.0-rc.1`, is published as a pre-release.

To try the pipeline without publishing anything:

```sh
go run github.com/goreleaser/goreleaser/v2@latest release --snapshot --clean
```

The Homebrew step needs a `HOMEBREW_TAP_TOKEN` repository secret: a fine-grained personal access token with read and write access to the contents of `homebrew-taps`. Without it the release still succeeds and only the cask is skipped.
