# Releasing BuildBouncer

BuildBouncer is a standalone Go CLI. Releases publish native binaries for macOS, Linux and Windows. Homebrew is the planned managed installation method for macOS; the first public tag and Homebrew publication are blocked until the prerequisites below are met. No Node.js runtime or npm registry is needed for installation.

## Artifacts and supported targets

GoReleaser produces `darwin`, `linux` and `windows` binaries on `amd64` and `arm64`. macOS and Linux archives are `tar.gz`, and Windows archives are ZIP files. Release archives use SHA-256 checksums; CLI build metadata includes the version and commit.

The Homebrew tap uses a cask for the precompiled binary, as supported by GoReleaser v2. Do not substitute the deprecated GoReleaser `brews` generator. Windows Winget/Scoop and Linux package repositories are follow-up distribution channels, not currently published or tested.

## Required before the first public release

1. **License:** Select and commit explicit distribution terms in `LICENSE`. There is no license declaration in the repository yet. Do not infer the owner's permission to redistribute from public source availability.
2. **Homebrew tap:** Create `berniemackie97/homebrew-tap` with a default branch and grant the release workflow a narrowly scoped `HOMEBREW_TAP_TOKEN` with contents write access to that repository only. The default GitHub Actions token cannot publish into another repository.
3. **Release authorization:** Configure the protected GitHub Actions `release` environment, tag protection and release approval policy. Release tags must be approved, immutable SemVer tags and point to a commit reachable from `master`.
4. **macOS distribution:** Choose and validate an Apple signing/notarization strategy before promising that downloaded binaries work under standard Gatekeeper settings. Do not silently strip quarantine attributes.
5. **First release rehearsal:** Review the generated snapshot archives, cask, checksums, cross-platform installation results, and the release workflow before creating a tag.

## Validation and publication

The normal CI workflow tests formatting, dependencies, vet and Go tests on Linux, macOS and Windows, plus race tests on Linux.

The separate **Distribution** workflow runs on pull requests and `master`. It builds an unpublished GoReleaser snapshot and uploads the native archives. Clean Linux, macOS and Windows runners then download that exact artifact, verify their platform archive against the generated SHA-256 checksums, extract it to an isolated temporary directory, execute the installed binary and verify version/commit metadata. Neither workflow publishes anything.

When a `vMAJOR.MINOR.PATCH` (optionally prerelease) tag is pushed, **Release** invokes that same Distribution workflow as a required reusable-workflow gate. The publication job additionally checks the license, tag ancestry, tap token and repository access, then publishes GitHub release assets and the Homebrew cask. A failure after external publication can leave a partially published release; investigate and recover explicitly rather than force-moving a tag.

Publishing is intentionally impossible until the license and tap requirements are met. No GitHub release or Homebrew package has been published yet.

## Installation and updates after the first published release

### macOS Homebrew

```sh
brew install --cask berniemackie97/tap/build-bouncer
brew upgrade --cask berniemackie97/tap/build-bouncer
```

### Native archives

Download the matching archive and checksum manifest from [GitHub Releases](https://github.com/berniemackie97/build-bouncer/releases). Verify SHA-256 before extracting the executable into a directory on PATH. Platform-specific package-manager integration is not required for native binaries. On macOS, follow the published signing/notarization guidance; do not bypass Gatekeeper by disabling quarantine.

### Go toolchain (source installation)

Developers with a compatible Go toolchain can install a tagged version with:

```sh
go install github.com/berniemackie97/build-bouncer/cmd/build-bouncer@vX.Y.Z
```

Use an actual published tag in place of `vX.Y.Z`. For an unreleased development build, pin a reviewed commit.

BuildBouncer is installed once globally. Its Git hooks and configurations remain per repository. A hook installed with `--no-copy` uses the resolved global binary; copied hooks must be refreshed separately. Package-manager upgrades should not regenerate project configs or mutate existing hooks.

There is no standalone self-updater in this slice. Package managers own their installations, and GitHub archive installations require manual replacement until a separately reviewed update mechanism is available.
