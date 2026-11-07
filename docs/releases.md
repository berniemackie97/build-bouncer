# Releasing BuildBouncer

BuildBouncer publishes one versioned Go binary across GitHub Releases, a Homebrew tap and npm. Homebrew and npm upgrade the global executable; `build-bouncer configure` manages per-repository checks. Neither package manager regenerates a repository's configuration or changes its Git hooks.

## Supported binaries

GoReleaser produces macOS, Linux and Windows binaries on amd64 and arm64, bundled in per-platform tarballs or Windows ZIP archives with SHA-256 checksums. The npm package includes all six binaries and a Node launcher, with no installation-time download scripts. The Homebrew cask references the corresponding GitHub release archive.

Version metadata is injected from the release tag and commit. Source builds without that injection report `dev`; a version string on its own is not a substitute for checking an installed binary's origin.

## Prerequisites before the first public release

1. Choose and commit a `LICENSE` file. Set the matching SPDX expression in `npm/package.json` instead of `UNLICENSED`. The release workflow refuses to publish without both. This repository does not currently have an explicit license; do not assume one.
2. Create the public GitHub repository `berniemackie97/homebrew-tap` with a default branch. Configure the BuildBouncer repository secret `HOMEBREW_TAP_TOKEN` with **Contents: read/write** permission restricted to that tap. The default `GITHUB_TOKEN` cannot write to a different repository. Prefer a GitHub App installation token when available.
3. Confirm an npm account controls the `@berniemackie97` scope. The first publish of `@berniemackie97/build-bouncer` needs npm authorization through the repository secret `NPM_TOKEN` or a separately bootstrapped package; the repository cannot create a new npm account/scope. After the package exists, configure npm Trusted Publishing for GitHub repository `berniemackie97/build-bouncer` and workflow filename `release.yml`, authorize publish, and remove `NPM_TOKEN`. npm 11.5.1+ on GitHub-hosted runners supports OIDC; the workflow uses Node 24 and `id-token: write`.
4. Configure a protected GitHub Actions environment named `release`, with approvals/branch/tag restrictions matching your release policy. Protect `master`, allow trusted tag creation only, and review changes before tagging.
5. Decide whether to set up Apple code signing and notarization for Homebrew casks. Unsigned downloaded macOS executables may be blocked by Gatekeeper. Do not bypass quarantine or advertise unsigned cask installation as universally reliable.

## Validation before tagging

The `CI` workflow runs Go formatting, module verification, vet and tests across Linux, macOS and Windows plus a Linux race test. The `Distribution` workflow additionally runs a GoReleaser snapshot, stages the npm package from the exact archive bytes, verifies every checksum, invokes the native npm binary on Linux, and checks the npm package file manifest. Neither workflow publishes.

The release job repeats Go tests and the race test and verifies that the SemVer tag commit is reachable from `master`, the license is present, and the Homebrew tap can be accessed with the publishing token. An invalid or unsupported tag fails before publication.

## Publish

After merging and confirming the above prerequisites, create and push an approved release tag such as `v0.2.0`. Tags must be `vMAJOR.MINOR.PATCH` optionally followed by a SemVer prerelease. The tag invokes `.github/workflows/release.yml`. GoReleaser creates the GitHub release and publishes the Homebrew cask to the tap, then npm publishes the staged package from those same archives. If publishing to a downstream registry fails after GitHub release creation, treat it as a partial release; investigate and recover rather than reusing a version or force-moving the tag. The release workflow has a concurrency guard for the tag.

The npm bundle is larger than a platform-specific package, intentionally: installing or updating it never requires an opaque postinstall script to fetch binaries.

## Install and update after the first published release

### Homebrew

```sh
brew install --cask berniemackie97/tap/build-bouncer
brew upgrade --cask berniemackie97/tap/build-bouncer
```

### npm

```sh
npm install -g @berniemackie97/build-bouncer
npm update -g @berniemackie97/build-bouncer
```

### Direct GitHub release

Download the archive for the target OS/architecture from the [Releases](https://github.com/berniemackie97/build-bouncer/releases) page. Verify its SHA-256 against the matching checksums file before installing.

Do not install BuildBouncer simultaneously from multiple package managers into competing PATH locations. Inspect `command -v build-bouncer` (or `Get-Command build-bouncer` on Windows) if updates appear not to take effect. A project hook installed with `--no-copy` uses the currently resolved global executable. Copied hooks retain their own binary until reinstalled.

No self-update subcommand is part of this release slice. Homebrew and npm upgrades remain under their package managers' control.
