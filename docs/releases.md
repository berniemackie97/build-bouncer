# Releasing BuildBouncer

BuildBouncer is a standalone Go CLI. Releases publish native binaries for macOS, Linux and Windows. Homebrew is the planned managed installation method for macOS; the first public tag and Homebrew publication are blocked until the prerequisites below are met. No Node.js runtime or npm registry is needed for installation.

## Artifacts and supported targets

GoReleaser produces `darwin`, `linux` and `windows` binaries on `amd64` and `arm64`. macOS and Linux archives are `tar.gz`, and Windows archives are ZIP files. Release archives use SHA-256 checksums; CLI build metadata includes the version and commit.

The Homebrew tap uses a cask for the precompiled binary, as supported by GoReleaser v2. Do not substitute the deprecated GoReleaser `brews` generator. Windows Winget/Scoop and Linux package repositories are follow-up distribution channels, not currently published or tested.

## Required before the first public release

1. **License:** The project is distributed under the [MIT License](../LICENSE), with a 2025–2026 copyright notice. The release workflow still refuses publication if the license file is missing or empty.
2. **Homebrew tap:** Create `berniemackie97/homebrew-tap` with a default branch. The GitHub CLI command `gh repo create berniemackie97/homebrew-tap --public --add-readme --description 'Homebrew tap for BuildBouncer'` can create it when authenticated as the owner. Provision a fine-grained token scoped only to that tap with **Contents: read/write** access and save it as `HOMEBREW_TAP_TOKEN` in the BuildBouncer `release` environment. Never put the token into source or chat. The default GitHub Actions token cannot write to another repository.
3. **Release authorization:** Configure the protected GitHub Actions `release` environment under **Settings → Environments**, with appropriate reviewer approvals and tag restrictions; protect `v*` tags using repository rulesets. Merely referencing an environment in YAML does not configure protection. Release tags must be approved, immutable SemVer tags and point to a commit reachable from `master`.
4. **macOS distribution:** Choose and validate an Apple signing/notarization strategy before promising that downloaded binaries work under standard Gatekeeper settings. Do not silently strip quarantine attributes.
5. **First release rehearsal:** Review the generated snapshot archives, cask, checksums, cross-platform installation results, and the release workflow before creating a tag.

## Validation and publication

The normal CI workflow tests formatting, dependencies, vet and Go tests on Linux, macOS and Windows, plus race tests on Linux.

The separate **Distribution** workflow runs on pull requests and `master`. It builds an unpublished GoReleaser snapshot and uploads the native archives. Clean Linux, macOS and Windows runners independently verify SHA-256 checksums for all six archives, extract the appropriate platform binary to an isolated temporary directory, execute it and verify version/commit metadata. A separate rehearsal job creates an unpushed temporary SemVer prerelease tag and builds versioned archives with `goreleaser release --skip=publish`, verifying tagged-release behavior before the first public tag. Neither workflow publishes anything.

When a `vMAJOR.MINOR.PATCH` (optionally prerelease) tag is pushed, **Release** invokes that same Distribution workflow in exact-tag mode (`goreleaser release --skip=publish --clean`), verifying the tagged archives before publication. The publication job checks the license, tag ancestry, tap token, repository access and an existing release, then publishes GitHub release assets and the Homebrew cask. The release uses `checksums.txt` and attaches signed GitHub artifact provenance attestations for published archives. A failure during Homebrew publication or subsequent attestation can leave a partially published release; investigate and recover explicitly rather than force-moving a tag.

Publishing is intentionally gated on the Homebrew tap and required release credentials. No GitHub release or Homebrew package has been published yet.

## First tagged release

After merging a green PR and configuring the above prerequisites, choose the first version (for example `v0.1.0`). From a clean, up-to-date `master`, create one annotated tag and push it:

```sh
git switch master
git pull --ff-only
git tag -a v0.1.0 -m 'BuildBouncer v0.1.0'
git push origin v0.1.0
```

Do not push a release tag before the Homebrew tap and release permissions are configured. Check the GitHub Release, `checksums.txt`, archive digests, the signed provenance and the resulting `Casks/build-bouncer.rb` in the tap after the release job. To verify a downloaded release archive's provenance, use `gh attestation verify FILE -R berniemackie97/build-bouncer` after verifying its SHA-256.

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
