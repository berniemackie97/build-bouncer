#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ne 1 ]; then
  echo "usage: publish-homebrew-formula.sh SOURCE_ARCHIVE" >&2
  exit 2
fi

source_archive="$1"
version="${GITHUB_REF_NAME#v}"
if [ "$version" = "$GITHUB_REF_NAME" ]; then
  echo "release tag must start with v" >&2
  exit 1
fi
if [ "$(basename "$source_archive")" != "build-bouncer_${version}_source.tar.gz" ]; then
  echo "unexpected source archive" >&2
  exit 1
fi
if [ -z "${GH_TOKEN:-}" ]; then
  echo "missing Homebrew publishing token" >&2
  exit 1
fi

tap_dir="$RUNNER_TEMP/build-bouncer-homebrew-tap"
gh auth setup-git
git clone --quiet --depth 1 --branch main \
  "https://github.com/berniemackie97/homebrew-tap.git" "$tap_dir"

source_url="https://github.com/berniemackie97/build-bouncer/releases/download/$GITHUB_REF_NAME/$(basename "$source_archive")"
./scripts/render-homebrew-formula.sh \
  "$version" "$GITHUB_SHA" "$source_archive" "$source_url" \
  "$tap_dir/Formula/build-bouncer.rb"

ruby -c "$tap_dir/Formula/build-bouncer.rb"
git -C "$tap_dir" rm --quiet --ignore-unmatch "Casks/build-bouncer.rb"
git -C "$tap_dir" add "Formula/build-bouncer.rb"
if git -C "$tap_dir" diff --cached --quiet; then
  echo "Homebrew formula already matches this release; refusing empty publication" >&2
  exit 1
fi

git -C "$tap_dir" -c user.name="BuildBouncer Releases" \
  -c user.email="berniemackie97@users.noreply.github.com" \
  commit --quiet -m "Publish BuildBouncer $GITHUB_REF_NAME source formula"
git -C "$tap_dir" push --quiet origin HEAD:main
