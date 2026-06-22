# Releasing

Releases are automated with [GoReleaser](https://goreleaser.com) via GitHub
Actions (`.github/workflows/release.yml`): pushing a `vX.Y.Z` tag builds binaries
for macOS/Linux (amd64/arm64), publishes a GitHub Release, and updates the
Homebrew tap.

## One-time setup

1. **Create the tap repository** (public) on GitHub:
   `triforge-ai/homebrew-tap` — empty is fine, GoReleaser populates `Casks/`.

2. **Create a Personal Access Token** with `repo` scope (classic) or a
   fine-grained token with **Contents: read/write** on `homebrew-tap`.

3. **Add it as a secret** on the `triforge-ai/aistack` repo:
   Settings → Secrets and variables → Actions → New repository secret
   - Name: `HOMEBREW_TAP_GITHUB_TOKEN`
   - Value: the PAT

   (The built-in `GITHUB_TOKEN` can publish the release but cannot push to the
   separate tap repo, hence the PAT.)

## Cutting a release

```bash
# make sure main is green and the working tree is clean
git tag v0.1.0
git push origin v0.1.0
```

GitHub Actions does the rest. After it finishes:

```bash
brew install triforge-ai/tap/ai
```

## Local dry run

No tag or publish, just build the artifacts into `dist/`:

```bash
make release-snapshot      # goreleaser release --snapshot --clean --skip=publish
goreleaser check           # validate .goreleaser.yaml
```

## Notes

- The binary version comes from the git tag, injected via
  `-ldflags "-X main.version=..."` and shown by `ai version`.
- Homebrew **casks** are macOS-only; Linux users install from the release
  tarball on the Releases page.
- `go install` works: the module path is `github.com/triforge-ai/aistack`, so
  `go install github.com/triforge-ai/aistack/cmd/ai@latest` installs the `ai`
  binary directly from source (pure-Go deps, `CGO_ENABLED=0`).
